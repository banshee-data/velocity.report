import {
  APPROACH,
  createApproach,
  stepApproach,
  openReplay,
  following,
  signalAt,
} from "./sf-following.js";
import { updateFollowingKey } from "./sf-following-input.js";
import { bindDrivingPad } from "./sf-touch-controls.js";
import { SCENE_CSS_COLOURS as colours } from "./scene-colours.js";
const $ = (id) => document.getElementById(id);
const canvas = $("approach"),
  ctx = canvas.getContext("2d");
let run = createApproach(),
  started = false,
  accumulator = 0,
  last = 0,
  playback = 0;
let touch = {},
  keys = new Set();
const pad = bindDrivingPad(
  document.querySelectorAll("[data-act]"),
  (value) => (touch = value),
);
const clear = () => {
  keys.clear();
  pad.clear();
  accumulator = 0;
  last = 0;
};
window.addEventListener("blur", clear);
document.addEventListener("visibilitychange", clear);
for (const type of ["keydown", "keyup"])
  window.addEventListener(type, (e) => updateFollowingKey(keys, e));
$("start").onclick = () => {
  clear();
  run = createApproach();
  started = true;
  playback = 0;
  $("start").blur();
};
$("replay").onclick = () => {
  if (openReplay(run)) playback = 0;
  $("replay").blur();
};
$("closeReplay").onclick = () => (run.replay = false);
const number = (v, unit) =>
  v == null ? "unavailable" : `${v.toFixed(1)} ${unit}`;
function draw(frame) {
  const width = canvas.clientWidth;
  canvas.width = width * devicePixelRatio;
  canvas.height = 260 * devicePixelRatio;
  ctx.scale(devicePixelRatio, devicePixelRatio);
  const scale = (width - 32) / 190,
    x = (metres) => 16 + metres * scale;
  ctx.fillStyle = colours.asphalt;
  ctx.fillRect(16, 65, width - 32, 110);
  ctx.fillStyle = colours.text;
  ctx.fillRect(x(APPROACH.line), 65, 2, 110);
  for (let y = 65; y < 170; y += 18)
    ctx.fillRect(x(APPROACH.crossing), y, 6 * scale, 10);
  ctx.font = "12px monospace";
  ctx.fillText("LIMIT", x(APPROACH.line) - 25, 195);
  ctx.fillText("CROSSWALK", Math.min(width - 80, x(APPROACH.crossing)), 215);
  for (const [actor, label] of [
    [frame.player, "YOU"],
    [frame.lead, "LEAD"],
  ]) {
    ctx.fillStyle = colours.vehicleShade;
    ctx.strokeStyle = colours.vehicle;
    ctx.fillRect(x(actor.x - 2.3), 105, 4.6 * scale, 28);
    ctx.strokeRect(x(actor.x - 2.3), 105, 4.6 * scale, 28);
    ctx.fillStyle = colours.text;
    ctx.fillText(label, x(actor.x) - 12, label === "YOU" ? 153 : 95);
  }
  ctx.fillText(
    `${frame.signal.toUpperCase()} · ${frame.t.toFixed(1)}s${run.replay ? " · RECORDED REPLAY" : ""}`,
    16,
    35,
  );
}
function animate(now) {
  const elapsed = last ? Math.min((now - last) / 1000, 0.1) : 0;
  last = now;
  if (started && !document.hidden) {
    accumulator += elapsed;
    while (accumulator >= APPROACH.dt) {
      stepApproach(run, {
        throttle: Number(keys.has("ArrowUp") || touch.throttle > 0),
        brake: Number(
          keys.has("Space") || keys.has("ArrowDown") || touch.brake,
        ),
      });
      accumulator -= APPROACH.dt;
    }
  }
  const gap = following(run.player, run.lead),
    event = run.event;
  $("metrics").textContent =
    `${run.contact ? "Contact · attempt ended" : run.finished ? "Attempt ended" : started ? "Live" : "Ready"} · ${signalAt(run.tick * APPROACH.dt)} · Speed ${number(run.player.v, "m/s")}\n${run.player.v === 0 ? "Stopped gap" : "Gap"} ${number(gap.gap, "m")} · Headway ${number(gap.headway, "s")}\nClosing ${number(gap.closing, "m/s")} · ${gap.status}\nExperimental DRAC ${number(gap.drac, "m/s²")} (constant lead speed assumption)`;
  $("replay").hidden = !(
    event?.end != null &&
    run.player.v === 0 &&
    !run.contact &&
    !run.replay
  );
  $("closeReplay").hidden = !run.replay;
  let frame = {
    player: run.player,
    lead: run.lead,
    signal: signalAt(run.tick * APPROACH.dt),
    t: run.tick * APPROACH.dt,
  };
  if (run.replay) {
    playback += elapsed;
    const frames = event.frames;
    frame = frames[Math.min(frames.length - 1, Math.floor(playback * 20))];
  }
  $("annotation").textContent = event
    ? `First brake: ${event.start.toFixed(2)}s · ${number(event.speed, "m/s")}\n${event.end == null ? "Braking event incomplete" : `${number(event.distance, "m")} to stop in ${number(event.end - event.start, "s")} · ${event.outcome}\nLine crossing: ${event.signal}`}\nPeak deceleration ${number(event.peakDeceleration, "m/s²")} · peak jerk ${number(event.peakJerk, "m/s³")}${run.replay ? `\nRecorded ${frame.t.toFixed(2)}s · gap ${number(frame.gap.gap, "m")} · brake ${frame.brake ? "on" : "off"}` : ""}`
    : "First braking event will appear here. Brake firmly when needed; comfort is not scored.";
  draw(frame);
  requestAnimationFrame(animate);
}
requestAnimationFrame(animate);
