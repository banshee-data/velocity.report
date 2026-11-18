import test from "node:test";
import assert from "node:assert/strict";
import {
  followingLabel,
  sceneTrackLabel,
  gapBand,
  followingRelationships,
} from "../scene-following.js";

test("minimum colours have lower-inclusive boundaries and unknown stays unknown", () => {
  for (const [value, label] of [
    [0, "<0.5 s"],
    [0.5, "0.5–1 s"],
    [1, "1–1.5 s"],
    [1.5, "1.5–2 s"],
    [2, "2+ s"],
  ])
    assert.equal(gapBand(value)?.label, label);
  for (const v of [null, undefined, NaN, Infinity, -1])
    assert.equal(gapBand(v), null);
});

test("only present, exact eligible pairs draw their projected measurement", () => {
  const f = {
    id: "f",
    following: {
      bucket: "following_eligible",
      leader_id: "l",
      gap_m: 10,
      net_time_gap_s: 1,
      track_minimum_time_gap_s: 0.5,
      projected_gap_xy: [
        [0, 0],
        [10, 0],
      ],
      experimental_drac_mps2: 0.2,
    },
  };
  assert.equal(followingRelationships({ tr: [f] }).length, 0);
  const rows = followingRelationships({ tr: [f, { id: "l" }] });
  assert.equal(rows.length, 1);
  assert.equal(rows[0].colour, gapBand(0.5).colour);
  assert.match(followingLabel(f), /DRAC 0.20 m\/s²/);
  assert.match(followingLabel(f), /track min 0.50 s/);
  for (const patch of [
    { bucket: "record_gap" },
    {
      projected_gap_xy: [
        [NaN, 0],
        [1, 0],
      ],
    },
    { projected_gap_xy: [[0, 0]] },
  ])
    assert.equal(
      followingRelationships({
        tr: [{ ...f, following: { ...f.following, ...patch } }, { id: "l" }],
      }).length,
      0,
    );
});

const following = {
  bucket: "following_eligible",
  leader_id: "t2",
  gap_m: 12.34,
  net_time_gap_s: 1.234,
};

test("review labels name the pair and separate distance, time and DRAC", () => {
  const track = { id: "t1", c: "car", following };
  assert.equal(
    sceneTrackLabel(track, 10, true),
    "t1 → t2\n12.3 m · 1.23 s\nDRAC unavailable",
  );
  assert.equal(
    sceneTrackLabel({ id: "t2", c: "car" }, 10, true),
    "t2 · 10 mph",
  );
  assert.equal(sceneTrackLabel({ id: "t3", c: "dynamic" }, 0, true), "");
  track.following = {
    ...following,
    bucket: "following_excluded",
    net_time_gap_s: null,
    drac_status: "supported",
    experimental_drac_mps2: 0.03,
  };
  assert.match(
    sceneTrackLabel(track, 0, true),
    /time gap unavailable\nDRAC 0.03/,
  );
});

test("supported DRAC survives an unavailable low-speed headway without inventing zero", () => {
  const track = {
    following: {
      ...following,
      bucket: "following_excluded",
      net_time_gap_s: null,
      drac_status: "supported",
      experimental_drac_mps2: 0.03,
    },
  };
  assert.match(sceneTrackLabel(track, 0), /DRAC 0.03/);
  assert.match(sceneTrackLabel(track, 0), /net time gap unavailable/);
  track.following.drac_status = "non_closing";
  track.following.experimental_drac_mps2 = null;
  assert.equal(sceneTrackLabel(track, 0), "");
});

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
