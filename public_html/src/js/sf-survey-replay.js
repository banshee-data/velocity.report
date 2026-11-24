import * as THREE from "../vendor/three/three.module.js";
import { roadUserKind, SCENE_COLOURS } from "./scene-colours.js";
import { sceneRangeOpacity } from "./scene-fade.js";
import { followingScopeAt } from "./scene-following.js";
import { advanceSceneClock, sceneSkipTarget } from "./scene-playback.js";
import { ChunkUnavailableError, SceneSession } from "./scene-reader.js";
import {
  createTrackBoxGeometry,
  createTrackBoxMaterials,
  placeTrackBox,
  trackBoxAnchor,
  trackBoxSize,
} from "./scene-track-box.js";
import { ghostCarFactory } from "./sf-car-model.js";
import { histogramSvg } from "./survey-charts.js";

const NOTICE_M = 250;
const APPROACH_M = 115;
const LEAVE_M = 145;
const SWITCH_MARGIN_M = 15;

// Interpolate between consecutive observations of the same track and class.
export function surveyTracksBetween(frame, next, us) {
  if (!next || us <= frame.t || next.t <= frame.t || next.t - frame.t > 250000)
    return frame.tr;
  const f = Math.min(1, (us - frame.t) / (next.t - frame.t));
  const targets = new Map(next.tr.map((t) => [t.id, t]));
  return frame.tr.map((t) => {
    const n = targets.get(t.id);
    if (t.id == null || !n || n.c !== t.c) return t;
    const [x, y] = trackBoxAnchor(t),
      [u, v] = trackBoxAnchor(n);
    const yaw = t.bh ?? t.hdg ?? 0,
      turn = (n.bh ?? n.hdg ?? yaw) - yaw;
    return {
      ...t,
      bx: x + (u - x) * f,
      by: y + (v - y) * f,
      z: t.z + (n.z - t.z) * f,
      bh: yaw + Math.atan2(Math.sin(turn), Math.cos(turn)) * f,
    };
  });
}

/** Keep the current recording near overlapping approaches and at the boundary. */
export function nearbySurvey(sites, car, current) {
  let nearest = null;
  let distance = APPROACH_M;
  for (const site of sites) {
    const d = Math.hypot(site.x - car.x, site.y - car.y);
    if (d <= distance) {
      nearest = site;
      distance = d;
    }
  }
  if (current) {
    const currentDistance = Math.hypot(current.x - car.x, current.y - car.y);
    if (
      currentDistance <= LEAVE_M &&
      (!nearest || currentDistance <= distance + SWITCH_MARGIN_M)
    )
      return current;
  }
  return nearest;
}

/** The closest site in front of the car, using straight-line distance. */
function surveyAhead(sites, car) {
  const east = Math.cos(car.heading);
  const north = Math.sin(car.heading);
  let result = null;
  let distance = NOTICE_M;
  for (const site of sites) {
    const dx = site.x - car.x;
    const dy = site.y - car.y;
    if (dx * east + dy * north <= 0) continue;
    const metres = Math.hypot(dx, dy);
    if (metres > distance) continue;
    result = { site, distance: metres };
    distance = metres;
  }
  return result;
}

export function projectSurveyTrack(transform, track) {
  const [x, y] = trackBoxAnchor(track);
  const z = track.z;
  if (
    ![x, y, z].every(Number.isFinite) ||
    !sceneRangeOpacity(Math.hypot(x, y, z))
  )
    return null;
  const anchor = transform.map(([a, b, c, d]) => a * x + b * y + c * z + d);
  const yaw = track.bh ?? track.hdg ?? 0;
  const east = Math.cos(yaw);
  const north = Math.sin(yaw);
  const [dx, dy] = transform.map(([a, b]) => a * east + b * north);
  return { anchor, yaw: Math.atan2(dy, dx) };
}

/**
 * One local replay clock. New approaches start at the beginning; buffering
 * pauses the clock. Requests from an old approach can finish without updating
 * the current intersection's card or scene.
 */
export function createSurveyReplay({
  sites,
  open,
  onSite,
  onFrame,
  onStatus,
  onApproach,
}) {
  let active = null;

  async function sample(replay, seconds) {
    replay.busy = true;
    try {
      const { session } = replay;
      const { part, us } = session.locate(seconds);
      let pair = replay.pair;
      if (
        !pair ||
        pair.part !== part ||
        us < pair.frame.t ||
        us >= pair.until
      ) {
        const located = await part.frameAtOffsetWithIndex(us);
        if (!located) throw new Error("Survey replay contains no frame");
        let next = null,
          until = Infinity;
        if (located.frameIndex + 1 < part.frameCount) {
          try {
            next = await part.frameAtIndex(located.frameIndex + 1);
            until = next.t;
          } catch (error) {
            // The current frame is still playable when only lookahead fails.
            // At the chunk boundary the ordinary missing-chunk recovery runs.
            if (!(error instanceof ChunkUnavailableError)) throw error;
            until = part.chunks[part.chunkIndexForOffset(us) + 1]?.t0 ?? us;
          }
        }
        pair = { part, frame: located.frame, next, until };
        replay.pair = pair;
      }
      if (replay !== active) return;
      replay.hasFrame = true;
      if (
        followingScopeAt(part.header, pair.frame.t).available &&
        followingScopeAt(part.header, us).available
      ) {
        onStatus("Recorded traffic · looping");
        const next =
          pair.next && followingScopeAt(part.header, pair.next.t).available
            ? pair.next
            : null;
        onFrame(replay.site, surveyTracksBetween(pair.frame, next, us));
      } else {
        onStatus("Outside placed replay coverage");
        onFrame(replay.site, []);
      }
    } catch (error) {
      if (replay !== active) return;
      const skip =
        error instanceof ChunkUnavailableError &&
        sceneSkipTarget(replay.session, seconds);
      onFrame(replay.site, []);
      if (skip) {
        replay.seconds = skip.seconds;
        replay.pair = null;
      } else {
        replay.session = null;
        onStatus("Replay unavailable");
      }
    } finally {
      replay.busy = false;
    }
  }

  async function start(replay) {
    try {
      const session = await open(replay.site);
      if (active !== replay) return;
      if (!(session.duration > 0))
        throw new Error("Survey replay has no duration");
      replay.session = session;
      await sample(replay, 0);
    } catch {
      if (active === replay) onStatus("Replay unavailable");
    }
  }

  return {
    update(car, dt) {
      const site = car ? nearbySurvey(sites, car, active?.site) : null;
      if (site !== (active?.site ?? null)) {
        active = site ? { site, seconds: 0, busy: true } : null;
        onSite(site);
        onFrame(site, []);
        if (active) {
          onStatus("Loading recorded traffic…");
          void start(active);
        }
      }
      onApproach?.(
        car && !active ? surveyAhead(sites, car) : null,
        car,
        active?.site,
      );
      if (!active?.session || active.busy) return;
      const { seconds, wrapped } = advanceSceneClock(
        active.seconds,
        active.hasFrame ? dt : 0,
        1,
        active.session.duration,
      );
      active.seconds = seconds;
      if (wrapped) {
        active.pair = null;
        onFrame(active.site, []);
      }
      void sample(active, seconds);
    },
  };
}

export function mountSurveyReplay({ sites, root, scene, hud, groundAt }) {
  const layer = new THREE.Group();
  scene.add(layer);
  const geometry = createTrackBoxGeometry();
  const materials = Object.fromEntries(
    ["vehicle", "walking", "cycle"].map((kind) => [
      kind,
      createTrackBoxMaterials(SCENE_COLOURS[kind]),
    ]),
  );
  const boxes = [];
  const ghosts = [];
  const makeGhost = ghostCarFactory();
  function showTracks(site, tracks) {
    let used = 0,
      ghostCount = 0;
    for (const track of tracks) {
      const kind = roadUserKind(track.c);
      if (!kind) continue;
      const pose = projectSurveyTrack(site.transform, track);
      if (!pose) continue;
      const { anchor, yaw } = pose;
      const ground = groundAt(anchor[0], anchor[1]);
      if (ground === null) continue;
      // Keep box bases visible over the game's coarser terrain. Horizontal
      // positions come directly from the accepted pose.
      anchor[2] = Math.max(anchor[2], ground + 0.06);
      if (kind === "vehicle") {
        let ghost = ghosts[ghostCount++];
        if (!ghost) {
          ghost = makeGhost();
          ghosts.push(ghost);
          layer.add(ghost);
        }
        ghost.visible = true;
        ghost.position.set(anchor[0], anchor[2], -anchor[1]);
        ghost.rotation.y = yaw;
        continue;
      }
      let box = boxes[used];
      if (!box) {
        box = new THREE.Group();
        box.add(
          new THREE.Mesh(geometry.fill, materials[kind].fill),
          new THREE.LineSegments(geometry.edges, materials[kind].edges),
        );
        layer.add(box);
        boxes.push(box);
      }
      box.children[0].material = materials[kind].fill;
      box.children[1].material = materials[kind].edges;
      box.visible = true;
      placeTrackBox(box, anchor, trackBoxSize(track), yaw);
      used++;
    }
    for (let i = ghostCount; i < ghosts.length; i++) ghosts[i].visible = false;
    for (let i = used; i < boxes.length; i++) boxes[i].visible = false;
  }

  let approachSite = null;
  return createSurveyReplay({
    sites,
    onApproach(approach, car, activeSite) {
      const target = activeSite || approach?.site;
      if (target) {
        const dx = target.x - car.x,
          dy = target.y - car.y;
        const direction = car.heading - Math.atan2(dy, dx);
        const cue = activeSite ? hud.cardCue : hud.approachCue;
        cue.querySelector("i").style.transform = `rotate(${direction}rad)`;
        const distance = Math.round(Math.hypot(dx, dy));
        cue.querySelector("span").textContent = `${distance} m`;
        cue.setAttribute(
          "aria-label",
          `Survey ${distance} metres, ${["ahead", "ahead right", "right", "behind right", "behind", "behind left", "left", "ahead left"][(Math.round(direction / (Math.PI / 4)) + 16) % 8]}`,
        );
      }
      const site = approach?.site ?? null;
      if (site !== approachSite) {
        approachSite = site;
        hud.approach.hidden = !site;
        if (site) hud.approachTitle.textContent = site.title;
      }
      if (!site) return;
    },
    open: (site) => new SceneSession(new URL(site.manifest, root).href).open(),
    onFrame: showTracks,
    onStatus(text) {
      if (hud.status.textContent !== text) hud.status.textContent = text;
    },
    onSite(site) {
      hud.card.hidden = !site;
      if (!site) return;
      hud.title.textContent = site.title;
      hud.p85.textContent = Number.isFinite(site.speed.p85)
        ? `P85 ${site.speed.p85.toFixed(1)} mph`
        : "";
      hud.chart.innerHTML = histogramSvg(site.speed, {
        width: 188,
        height: 62,
        compact: true,
        showMph: true,
        limit: site.limit,
        label: "Peak speed per vehicle track",
      });
      hud.link.href = new URL(site.page, root).href;
    },
  });
}
