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
import { histogramSvg } from "./survey-charts.js";

const NOTICE_M = 250;
const APPROACH_M = 115;
const LEAVE_M = 145;
const SWITCH_MARGIN_M = 15;
const SAMPLE_INTERVAL_S = 1 / 15;

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
      const { frame, partIndex } = await replay.session.frameAt(seconds);
      if (replay !== active) return;
      if (!frame) throw new Error("Survey replay contains no frame");
      replay.hasFrame = true;
      replay.lastSample = seconds;
      if (frame === replay.frame) return;
      replay.frame = frame;
      const header = replay.session.parts[partIndex].header;
      if (followingScopeAt(header, frame.t).available) {
        onStatus("Recorded traffic · looping");
        onFrame(replay.site, frame.tr);
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
        replay.lastSample = -Infinity;
        replay.frame = null;
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
        active = site
          ? { site, seconds: 0, lastSample: -Infinity, busy: true }
          : null;
        onSite(site);
        onFrame(site, []);
        if (active) {
          onStatus("Loading recorded traffic…");
          void start(active);
        }
      }
      onApproach?.(car && !active ? surveyAhead(sites, car) : null);
      if (!active?.session || active.busy) return;
      const { seconds, wrapped } = advanceSceneClock(
        active.seconds,
        active.hasFrame ? dt : 0,
        1,
        active.session.duration,
      );
      active.seconds = seconds;
      if (wrapped) onFrame(active.site, []);
      if (wrapped || seconds - active.lastSample >= SAMPLE_INTERVAL_S)
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
  function showTracks(site, tracks) {
    let used = 0;
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
    for (let i = used; i < boxes.length; i++) boxes[i].visible = false;
  }

  let approachSite = null;
  return createSurveyReplay({
    sites,
    onApproach(approach) {
      const site = approach?.site ?? null;
      if (site !== approachSite) {
        approachSite = site;
        hud.approach.hidden = !site;
        if (site) hud.approachTitle.textContent = site.title;
      }
      if (!site) return;
      // Round to 10 m so the readout is steady while driving.
      const distance = `${Math.round(approach.distance / 10) * 10} m`;
      if (hud.approachDistance.textContent !== distance)
        hud.approachDistance.textContent = distance;
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
