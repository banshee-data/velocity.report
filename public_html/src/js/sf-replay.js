import { Quaternion } from "../vendor/three/three.core.js";

export const REPLAY_HZ = 120;
export const CRASH_LEAD_TICKS = 2 * REPLAY_HZ;
export const CRASH_HOLD_TICKS = 2 * REPLAY_HZ;
export const MAX_REPLAY_TICKS = 30 * REPLAY_HZ;
const KEY = "sf-replays-v1";
export const REPLAY_POSITION_SCALE = 100; // centimetres
const ROT = 10000;
const LIMIT = 8;
export const FAVOURITE_LIMIT = 8;
const modes = ["city", "survey", "soma", "bay-bridge", "practice"];
const before = new Quaternion(),
  after = new Quaternion();
const angleOf = (a, b) =>
  Math.abs(Math.atan2(Math.sin(a - b), Math.cos(a - b)));

export function crashHistory() {
  const poses = new Array(CRASH_LEAD_TICKS);
  let end = 0,
    count = 0;
  return {
    push(pose) {
      poses[end] = pose;
      end = (end + 1) % poses.length;
      count = Math.min(count + 1, poses.length);
    },
    clear() {
      end = count = 0;
    },
    read() {
      return Array.from(
        { length: count },
        (_, i) => poses[(end - count + i + poses.length) % poses.length],
      );
    },
  };
}

// A pose carries the car origin, scene quaternion and camera heading. Keep
// the worst key whenever a chord misses a simulated position by 3 cm or
// orientation by 1°. Also check camera heading and split at the crash cut.
export function encodeReplay(poses, impact = null, hold = 0) {
  const frames = [];
  function encode(tick) {
    const { x, y, z, q, heading } = poses[tick];
    frames.push([
      tick,
      Math.round(x * REPLAY_POSITION_SCALE),
      Math.round(y * REPLAY_POSITION_SCALE),
      Math.round(z * REPLAY_POSITION_SCALE),
      ...q.map((value) => Math.round(value * ROT)),
      Math.round(Math.atan2(Math.sin(heading), Math.cos(heading)) * ROT),
    ]);
  }
  function section(start, end) {
    if (start > end) return;
    encode(start);
    for (let i = start; i < end; i += 24) keep(i, Math.min(i + 24, end));
  }
  function keep(start, end) {
    const a = poses[start],
      b = poses[end];
    before.fromArray(a.q);
    let worst = 1,
      split = -1;
    for (let i = start + 1; i < end; i++) {
      const p = poses[i],
        t = (i - start) / (end - start);
      after.fromArray(b.q);
      const q = after.slerp(before, 1 - t);
      const dot = Math.abs(
        q.x * p.q[0] + q.y * p.q[1] + q.z * p.q[2] + q.w * p.q[3],
      );
      const h =
        a.heading +
        Math.atan2(
          Math.sin(b.heading - a.heading),
          Math.cos(b.heading - a.heading),
        ) *
          t;
      const error = Math.max(
        Math.hypot(
          p.x - a.x - (b.x - a.x) * t,
          p.y - a.y - (b.y - a.y) * t,
          p.z - a.z - (b.z - a.z) * t,
        ) / 0.03,
        (2 * Math.acos(Math.min(1, dot))) / (Math.PI / 180),
        angleOf(p.heading, h) / (Math.PI / 180),
      );
      if (error > worst) {
        worst = error;
        split = i;
      }
    }
    if (split < 0) encode(end);
    else {
      keep(start, split);
      keep(split, end);
    }
  }
  if (impact === null) section(0, poses.length - 1);
  else {
    section(0, impact - 1);
    section(impact, poses.length - 1);
  }
  return { frames, impact, end: poses.length - 1 + hold };
}

export function replayPose(clip, tick) {
  const { frames, impact } = clip;
  let lo = 0,
    hi = frames.length - 1;
  while (lo + 1 < hi) {
    const i = (lo + hi) >> 1;
    if (frames[i][0] <= tick) lo = i;
    else hi = i;
  }
  const a = frames[lo],
    b = frames[hi];
  const t =
    a === b || (b[0] === impact && tick < b[0])
      ? 0
      : Math.max(0, Math.min(1, (tick - a[0]) / (b[0] - a[0])));
  const lerp = (index) =>
    (a[index] + (b[index] - a[index]) * t) / REPLAY_POSITION_SCALE;
  before.set(a[4] / ROT, a[5] / ROT, a[6] / ROT, a[7] / ROT).normalize();
  after.set(b[4] / ROT, b[5] / ROT, b[6] / ROT, b[7] / ROT).normalize();
  before.slerp(after, t);
  const delta = (b[8] - a[8]) / ROT;
  return {
    x: lerp(1),
    y: lerp(2),
    z: lerp(3),
    q: before.toArray(),
    heading: a[8] / ROT + Math.atan2(Math.sin(delta), Math.cos(delta)) * t,
  };
}

function validReplay(record, crash) {
  if (
    !record ||
    typeof record.id !== "string" ||
    !modes.includes(record.mode) ||
    !Number.isFinite(record.created) ||
    Math.abs(record.created) > 8.64e15 ||
    !Number.isInteger(record.end) ||
    record.end < 1 ||
    record.end > MAX_REPLAY_TICKS ||
    !Array.isArray(record.frames) ||
    !record.frames.length ||
    record.frames.length > record.end + 1
  )
    return false;
  if (
    crash
      ? !Number.isInteger(record.impact) ||
        record.impact < 0 ||
        record.impact > CRASH_LEAD_TICKS ||
        record.impact > record.end ||
        typeof record.kind !== "string"
      : record.impact !== null ||
        !Number.isFinite(record.seconds) ||
        record.seconds <= 0 ||
        !Number.isFinite(record.peak) ||
        record.peak < 0
  )
    return false;
  if (
    !crash &&
    (record.takeoff !== undefined || record.touchdown !== undefined) &&
    (!Number.isInteger(record.takeoff) ||
      record.takeoff < 0 ||
      record.takeoff > CRASH_LEAD_TICKS ||
      !Number.isInteger(record.touchdown) ||
      record.touchdown <= record.takeoff ||
      record.touchdown + CRASH_HOLD_TICKS !== record.end)
  )
    return false;
  let last = -1;
  for (const frame of record.frames) {
    if (
      !Array.isArray(frame) ||
      frame.length !== 9 ||
      !frame.every(Number.isSafeInteger) ||
      frame[0] <= last ||
      frame[0] > record.end ||
      frame.slice(1, 4).some((n) => Math.abs(n) > 100000000) ||
      Math.abs(Math.hypot(...frame.slice(4, 8)) - ROT) > 2 ||
      Math.abs(frame[8]) > 31416
    )
      return false;
    last = frame[0];
  }
  return record.frames[0][0] === 0;
}

function keepRecords(records, crash) {
  const valid = Array.isArray(records)
    ? records.filter((row) => validReplay(row, crash))
    : [];
  const unique = [...new Map(valid.map((row) => [row.id, row])).values()];
  const ranked = unique.sort((a, b) =>
    crash
      ? b.created - a.created
      : b.seconds - a.seconds || b.peak - a.peak || b.created - a.created,
  );
  if (!crash) return ranked.slice(0, LIMIT);
  const favourites = ranked
    .filter((record) => record.favourite === true)
    .slice(0, FAVOURITE_LIMIT);
  return [
    ...new Map(
      [...favourites, ...ranked.slice(0, LIMIT)].map((record) => [
        record.id,
        record,
      ]),
    ).values(),
  ];
}

export function replayLibrary(storage) {
  let crashes = [],
    airtimes = [];
  let sessionOnly = false;
  const store = () => storage ?? globalThis.localStorage;
  function read() {
    // Disk is authoritative between successful writes. An open tab should
    // not reinsert a replay that another tab deleted.
    if (!sessionOnly) {
      try {
        const data = JSON.parse(store().getItem(KEY));
        crashes = keepRecords(data?.crashes, true);
        airtimes = keepRecords(data?.airtimes, false);
      } catch {
        // Corrupt or inaccessible storage leaves the last valid view intact.
      }
    }
    return { crashes, airtimes };
  }

  function write() {
    try {
      store().setItem(KEY, JSON.stringify({ crashes, airtimes }));
      sessionOnly = false;
      return true;
    } catch {
      sessionOnly = true;
      return false;
    }
  }

  function save(record) {
    read();
    if (record.impact === null)
      airtimes = keepRecords([record, ...airtimes], false);
    else crashes = keepRecords([record, ...crashes], true);
    return write();
  }

  function setFavourite(id, favourite) {
    read();
    const record = crashes.find((row) => row.id === id);
    if (
      !record ||
      (favourite &&
        !record.favourite &&
        crashes.filter((row) => row.favourite === true).length >=
          FAVOURITE_LIMIT)
    )
      return null;
    crashes = keepRecords(
      crashes.map((row) => (row.id === id ? { ...row, favourite } : row)),
      true,
    );
    return write();
  }

  function remove(id) {
    read();
    crashes = crashes.filter((row) => row.id !== id);
    airtimes = airtimes.filter((row) => row.id !== id);
    return write();
  }

  return { read, save, setFavourite, remove };
}
