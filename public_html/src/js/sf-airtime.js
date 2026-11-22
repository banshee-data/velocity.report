import { MAX_REPLAY_TICKS } from "./sf-replay.js";

const minimumClearance = 0.18;
const holdMs = 2000;

const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y);

function worldLine(line, origin) {
  return Array.from({ length: line.length / 2 }, (_, i) => ({
    x: origin[0] + line[i * 2] / 10,
    y: origin[1] + line[i * 2 + 1] / 10,
  }));
}

// Continue a named centreline across section and tile ends when the join is
// unambiguous. The terrain preview can cover the rest of the takeoff axis.
function extendLine(path, lines, reach) {
  let length = 0;
  while (length < reach) {
    const end = path.at(-1);
    const prior = path.at(-2);
    const heading = Math.atan2(end.y - prior.y, end.x - prior.x);
    const matches = [];
    for (const [index, line] of lines.entries()) {
      const forward = distance(end, line[0]) < 0.8;
      const reverse = distance(end, line.at(-1)) < 0.8;
      if (forward === reverse) continue;
      const points = forward ? line : [...line].reverse();
      const next = points[1];
      const bearing = Math.atan2(next.y - end.y, next.x - end.x);
      if (distance(end, next) > 0.01 && Math.cos(bearing - heading) > 0.85)
        matches.push({ index, points });
    }
    if (matches.length !== 1) break;
    const { index, points } = matches[0];
    lines.splice(index, 1);
    for (const point of points.slice(1)) {
      length += distance(path.at(-1), point);
      path.push(point);
    }
  }
}

export function sampleFlightProfile(car, road, heightAt, deckAt, tiles = []) {
  const heading = car.heading + (car.v < 0 ? Math.PI : 0);
  const direction = road && Math.cos(heading - road.heading) < 0 ? -1 : 1;
  const hx = Math.cos(heading),
    hy = Math.sin(heading);
  const ahead = Math.max(80, Math.abs(car.v) * 7);
  let stations, entry;
  if (road) {
    const path = worldLine(road.centreline, road.origin);
    const segmentStart = path[road.segment / 2];
    const segmentEnd = path[road.segment / 2 + 1];
    const lines = road.name
      ? tiles.flatMap((tile) =>
          tile.r.flatMap((row) =>
            row[3] !== road.centreline &&
            row[3].length >= 4 &&
            tile.n[row[0]] === road.name
              ? [worldLine(row[3], tile.o)]
              : [],
          ),
        )
      : [];
    extendLine(path, lines, ahead);
    path.reverse();
    extendLine(path, lines, ahead);
    path.reverse();
    let s = 0;
    stations = path.map((point, i) => {
      if (i) s += distance(point, path[i - 1]);
      if (point === segmentStart)
        entry = s + road.fraction * distance(segmentStart, segmentEnd);
      return { ...point, s };
    });
  }
  const profile = [];
  for (let s = -24; s <= ahead; s += 2) {
    let x = car.x + hx * s,
      y = car.y + hy * s;
    let onRoad = false;
    if (stations) {
      const target = entry + s * direction;
      const first = stations[0];
      const last = stations.at(-1);
      onRoad = target >= 0 && target <= last.s;
      const next = stations.findIndex(
        (point, index) => index > 0 && point.s >= target,
      );
      const i = target > last.s || next < 0 ? stations.length - 1 : next;
      const a = target < 0 ? first : stations[i - 1];
      const b = target < 0 ? stations[1] : stations[i];
      const t = (target - a.s) / (b.s - a.s || 1);
      x = a.x + (b.x - a.x) * t;
      y = a.y + (b.y - a.y) * t;
    }
    const height = heightAt(x, y);
    profile.push({
      s,
      x,
      y,
      onRoad,
      z: onRoad ? height : (deckAt?.(x, y) ?? null),
      ground: onRoad ? null : height,
    });
  }
  return { profile, basis: road ? "Road centreline" : "Takeoff heading" };
}

export function createFlightTimeline(onLanding) {
  let flight;
  let cards = [];
  function reset(keepCompleted = false) {
    flight = undefined;
    if (!keepCompleted) cards = [];
  }
  function finish(now, label = "Touchdown") {
    if (!flight) return;
    if (flight.peak >= minimumClearance) {
      flight.label = label;
      flight.until = now + holdMs;
      cards.push(flight);
      if (label === "Touchdown")
        onLanding?.(
          { seconds: flight.seconds, peak: flight.peak },
          flight.poses,
        );
      delete flight.poses;
    }
    flight = undefined;
  }
  function step(before, car, dt, now, environment) {
    if (car.grounded !== false && !flight) return;
    const { heightAt } = environment;
    if (!flight) {
      const { road, deckAt, tiles } = environment;
      flight = {
        ...sampleFlightProfile(before, road, heightAt, deckAt, tiles),
        trace: [{ s: 0, z: before.z }],
        poses: environment.pose ? [environment.pose(before)] : undefined,
        seconds: 0,
        peak: 0,
        label: "Airborne",
        right: Math.max(55, Math.abs(before.v) * 2),
        refreshedAt: now,
      };
    }
    const ground = heightAt(car.x, car.y);
    if (ground !== null) flight.peak = Math.max(flight.peak, car.z - ground);
    else if (car.grounded === false)
      flight.peak = Math.max(flight.peak, Math.abs(car.z - flight.trace[0].z));
    const s = flight.trace.at(-1).s + distance(before, car);
    flight.trace.push({ s, z: car.z });
    flight.seconds += dt;
    if (flight.poses?.length < MAX_REPLAY_TICKS)
      flight.poses.push(environment.pose(car));
    else flight.poses = undefined;
    flight.right = Math.max(flight.right, s + 12);
    if (now - flight.refreshedAt >= 120 || car.grounded !== false) {
      for (const point of flight.profile) {
        const layer = point.onRoad ? "z" : "ground";
        if (point[layer] === null) point[layer] = heightAt(point.x, point.y);
      }
      flight.refreshedAt = now;
    }
    if (environment.impact) finish(now, "Impact");
    else if (car.grounded !== false) finish(now);
  }
  function visible(now) {
    cards = cards.filter((card) => now < card.until);
    return flight?.peak >= minimumClearance ? [...cards, flight] : cards;
  }
  return { step, finish, reset, visible };
}

export function createAirtimeHud(panel, template, onLanding) {
  const timeline = createFlightTimeline(onLanding);
  const elements = new Map();
  function draw(now) {
    const visible = timeline.visible(now);
    for (const [flight, card] of elements) {
      if (visible.includes(flight)) continue;
      card.element.remove();
      elements.delete(flight);
    }
    for (const flight of visible) {
      let card = elements.get(flight);
      if (!card) {
        const element = template.content.firstElementChild.cloneNode(true);
        const canvas = element.querySelector("canvas");
        card = {
          element,
          readout: element.querySelector("strong"),
          canvas,
          ctx: canvas.getContext("2d"),
          lo: Infinity,
          hi: -Infinity,
        };
        elements.set(flight, card);
        panel.append(element);
      }
      if (card.label !== flight.label || card.samples !== flight.trace.length) {
        drawCard(card, flight);
        card.label = flight.label;
        card.samples = flight.trace.length;
      }
    }
    panel.hidden = !visible.length;
  }
  return { ...timeline, draw };
}

function drawCard(card, flight) {
  const { canvas, ctx } = card;
  const last = flight.trace.at(-1);
  card.readout.textContent = `${flight.label} · ${flight.seconds.toFixed(1)} s · ${last.z.toFixed(1)} m elevation`;
  const left = -24,
    right = flight.right;
  const profile = flight.profile.filter(({ s }) => s <= right);
  for (const { z, ground } of [...profile, ...flight.trace])
    for (const height of [z, ground]) {
      if (!Number.isFinite(height)) continue;
      card.lo = Math.min(card.lo, height - 1);
      card.hi = Math.max(card.hi, height + 1);
    }
  const { width: w, height: h } = canvas;
  const xAt = (s) => 14 + ((w - 28) * (s - left)) / (right - left);
  const yAt = (z) => 16 + ((h - 54) * (card.hi - z)) / (card.hi - card.lo);
  ctx.clearRect(0, 0, w, h);
  const colour = getComputedStyle(card.element);
  const plot = (points, key, ink, dashed = false) => {
    ctx.beginPath();
    let pen = false;
    for (const point of points) {
      const z = point[key];
      if (!Number.isFinite(z)) {
        pen = false;
        continue;
      }
      const x = xAt(point.s),
        y = yAt(z);
      if (pen) ctx.lineTo(x, y);
      else {
        ctx.moveTo(x, y);
        ctx.lineTo(x + 0.01, y);
      }
      pen = true;
    }
    ctx.setLineDash(dashed ? [7, 7] : []);
    ctx.strokeStyle = ink;
    ctx.lineWidth = 4;
    ctx.lineCap = "round";
    ctx.stroke();
  };
  const road = colour.getPropertyValue("--scene-cycle").trim();
  const trajectory = colour.getPropertyValue("--dash").trim();
  plot(profile, "ground", road, true);
  plot(profile, "z", road);
  plot(flight.trace, "z", trajectory);
  ctx.beginPath();
  ctx.arc(xAt(last.s), yAt(last.z), 6, 0, Math.PI * 2);
  ctx.fillStyle = trajectory;
  ctx.fill();
  ctx.fillStyle = colour.color;
  ctx.font = "20px system-ui";
  ctx.fillText(`${flight.basis} · metres from takeoff`, 14, h - 10);
  ctx.fillText(`${left}`, 14, h - 32);
  ctx.textAlign = "right";
  ctx.fillText(`${Math.round(right)}`, w - 14, h - 32);
  ctx.textAlign = "left";
}
