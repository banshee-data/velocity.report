import test from "node:test";
import assert from "node:assert/strict";
import { followingLabel, sceneTrackLabel } from "../scene-following.js";

const following = {
  bucket: "following_eligible",
  leader_id: "t2",
  gap_m: 12.34,
  net_time_gap_s: 1.234,
};

test("scene shows estimated distance and net time gap together", () => {
  assert.equal(followingLabel({ following }), "estimated gap 12.3 m · 1.23 s");
});

test("scene omits absent, excluded and malformed relationships", () => {
  for (const value of [
    undefined,
    { ...following, bucket: "record_gap" },
    { ...following, leader_id: "" },
    { ...following, gap_m: NaN },
    { ...following, gap_m: -1 },
    { ...following, net_time_gap_s: Infinity },
    { ...following, net_time_gap_s: -1 },
  ]) {
    assert.equal(followingLabel({ following: value }), "");
  }
});

test("a stopped or excluded next frame replaces the previous relationship label", () => {
  assert.match(sceneTrackLabel({ c: "car", following }, 10), /estimated gap/);
  assert.equal(
    sceneTrackLabel(
      { c: "car", following: { ...following, bucket: "record_gap" } },
      10,
    ),
    "10 mph",
  );
  assert.equal(sceneTrackLabel({}, 0), "");
  assert.equal(sceneTrackLabel({ c: "car" }, Infinity), "");
});

test("recorded dynamic classes do not hide exact following estimates or become vehicles", () => {
  const track = { c: "dynamic", following };
  assert.equal(sceneTrackLabel(track, 10), "estimated gap 12.3 m · 1.23 s");
  assert.equal(track.c, "dynamic");
  assert.equal(sceneTrackLabel({ c: "pedestrian" }, 10), "");
});
