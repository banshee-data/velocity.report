/**
 * Tests for the live-observation adapter behind the tracks page's scene view.
 *
 * The point of the adapter is that the operator tools and the public scenes
 * share one player, so these tests check the shared reader actually accepts
 * what the adapter builds: seeking, frame lookup and trail reconstruction are
 * exercised through the real PartReader/SceneSession code, not a stand-in.
 */

import { SceneSession } from '$scene/scene-reader.js';
import { LivePart, buildLiveFrames, createLiveSceneSession } from '$lib/scene/liveSceneSource';
import type { RunTrack, TrackObservation } from '$lib/types/lidar';

function observation(
	trackId: string,
	isoTimestamp: string,
	overrides: Partial<TrackObservation> = {}
): TrackObservation {
	return {
		track_id: trackId,
		timestamp: isoTimestamp,
		position: { x: 1, y: 2, z: 3 },
		velocity: { vx: 0.5, vy: -0.25 },
		speed_mps: 4,
		heading_rad: 0.75,
		bounding_box: { length: 4.5, width: 1.8, height: 1.5 },
		...overrides
	} as TrackObservation;
}

describe('buildLiveFrames', () => {
	it('groups observations sharing a frame timestamp into one frame', () => {
		// The pipeline writes every track in one rotation with that rotation's
		// frame time, so an exact timestamp match is the real frame boundary.
		const { frames } = buildLiveFrames([
			observation('a', '2026-09-16T10:00:00.000Z'),
			observation('b', '2026-09-16T10:00:00.000Z'),
			observation('a', '2026-09-16T10:00:00.100Z')
		]);

		expect(frames).toHaveLength(2);
		expect(frames[0].tr?.map((t) => t.id).sort()).toEqual(['a', 'b']);
		expect(frames[1].tr?.map((t) => t.id)).toEqual(['a']);
	});

	it('expresses frame time as a microsecond offset from the first frame', () => {
		// Absolute nanosecond capture times exceed Number.MAX_SAFE_INTEGER, so
		// the format carries offsets and keeps the absolute start separately.
		const { frames, startNs } = buildLiveFrames([
			observation('a', '2026-09-16T10:00:00.000Z'),
			observation('a', '2026-09-16T10:00:00.250Z')
		]);

		expect(frames[0].t).toBe(0);
		expect(frames[1].t).toBe(250_000);
		expect(startNs).toBe(`${Date.parse('2026-09-16T10:00:00.000Z')}000000`);
		// A string, because the value is past float precision.
		expect(typeof startNs).toBe('string');
	});

	it('orders frames by time whatever order the rows arrived in', () => {
		const { frames } = buildLiveFrames([
			observation('a', '2026-09-16T10:00:00.200Z'),
			observation('a', '2026-09-16T10:00:00.000Z'),
			observation('a', '2026-09-16T10:00:00.100Z')
		]);

		expect(frames.map((f) => f.t)).toEqual([0, 100_000, 200_000]);
		expect(frames.map((f) => f.f)).toEqual([0, 1, 2]);
	});

	it('maps observation geometry onto the scene track fields', () => {
		const { frames } = buildLiveFrames([
			observation('a', '2026-09-16T10:00:00.000Z', {
				position: { x: 10, y: -4, z: 1.2 },
				velocity: { vx: 3, vy: 4 },
				speed_mps: 5,
				heading_rad: 1.5,
				bounding_box: { length: 4.2, width: 1.9, height: 1.6 }
			})
		]);

		const track = frames[0].tr![0];
		expect(track).toMatchObject({
			id: 'a',
			x: 10,
			y: -4,
			z: 1.2,
			vx: 3,
			vy: 4,
			spd: 5,
			hdg: 1.5,
			l: 4.2,
			w: 1.9,
			h: 1.6
		});
		// No separate per-frame OBB yaw exists on this endpoint, so the box
		// heading follows the recorded heading rather than being invented.
		expect(track.bh).toBe(1.5);
	});

	it('carries each track lifetime peak speed on every one of its frames', () => {
		const { frames } = buildLiveFrames([
			observation('a', '2026-09-16T10:00:00.000Z', { speed_mps: 3 }),
			observation('a', '2026-09-16T10:00:00.100Z', { speed_mps: 9 }),
			observation('b', '2026-09-16T10:00:00.100Z', { speed_mps: 2 })
		]);

		expect(frames[0].tr![0].mspd).toBe(9);
		expect(frames[1].tr!.find((t) => t.id === 'a')!.mspd).toBe(9);
		expect(frames[1].tr!.find((t) => t.id === 'b')!.mspd).toBe(2);
	});

	it('prefers an operator label over the classifier', () => {
		const runTracks = [
			{
				track_id: 'a',
				object_class: 'car',
				object_confidence: 0.4,
				user_label: 'van',
				label_confidence: 0.95
			} as RunTrack,
			{ track_id: 'b', object_class: 'bus', object_confidence: 0.8 } as RunTrack
		];
		const { frames } = buildLiveFrames(
			[observation('a', '2026-09-16T10:00:00.000Z'), observation('b', '2026-09-16T10:00:00.000Z')],
			runTracks
		);

		const byId = new Map(frames[0].tr!.map((t) => [t.id, t]));
		// The labelling workflow exists because a person's judgement is the
		// better answer.
		expect(byId.get('a')).toMatchObject({ c: 'van', cf: 0.95 });
		expect(byId.get('b')).toMatchObject({ c: 'bus', cf: 0.8 });
	});

	it('leaves the class unset for a track with no label at all', () => {
		const { frames } = buildLiveFrames([observation('a', '2026-09-16T10:00:00.000Z')]);
		expect(frames[0].tr![0].c).toBeUndefined();
	});

	it('drops rows with an unusable timestamp and counts them', () => {
		// A row we cannot place in time cannot be drawn in time either, and
		// silently dropping it would make the count look complete.
		const { frames, skipped } = buildLiveFrames([
			observation('a', 'not-a-timestamp'),
			observation('a', '2026-09-16T10:00:00.000Z')
		]);

		expect(skipped).toBe(1);
		expect(frames).toHaveLength(1);
	});

	it('returns an empty result for no observations', () => {
		const { frames, startNs, skipped } = buildLiveFrames([]);
		expect(frames).toEqual([]);
		expect(startNs).toBe('0');
		expect(skipped).toBe(0);
	});
});

describe('LivePart', () => {
	const frames = [
		{ f: 0, t: 0, tr: [] },
		{ f: 1, t: 100_000, tr: [] },
		{ f: 2, t: 200_000, tr: [] }
	];

	function part() {
		return new LivePart(frames, { sensor_id: 's', start_ns: '0' });
	}

	it('reports its span from the frames it holds', () => {
		const p = part();
		expect(p.startUs).toBe(0);
		expect(p.endUs).toBe(200_000);
		expect(p.durationSec).toBeCloseTo(0.2);
		expect(p.frameCount).toBe(3);
	});

	it('serves the shared reader frame lookup from memory', async () => {
		// frameAtOffset is inherited: the adapter overrides only loadChunk, so
		// the binary search and clamping behaviour are the shared ones.
		const p = part();
		expect((await p.frameAtOffset(0))?.f).toBe(0);
		expect((await p.frameAtOffset(150_000))?.f).toBe(1);
		expect((await p.frameAtOffset(999_000))?.f).toBe(2);
	});

	it('reconstructs a range through the inherited framesInRange', async () => {
		const p = part();
		const inRange = await p.framesInRange(50_000, 200_000);
		expect(inRange.map((f) => f.f)).toEqual([1, 2]);
	});

	it('gives an empty run one empty chunk rather than none', async () => {
		// The reader indexes chunks[0] unconditionally, and a quiet street is
		// a real answer rather than a malformed part.
		const empty = new LivePart([], { sensor_id: 's' });
		expect(empty.chunks).toHaveLength(1);
		expect(empty.frameCount).toBe(0);
		expect(await empty.frameAtOffset(0)).toBeNull();
	});

	it('does not fetch: an unknown chunk is empty, not a network call', async () => {
		const p = part();
		expect(await p.loadChunk(7)).toEqual([]);
		// The base URL is deliberately not http, so an accidental fetch would
		// fail loudly rather than hitting the page's own origin.
		expect(p.baseURL.startsWith('live:')).toBe(true);
	});
});

describe('createLiveSceneSession', () => {
	const observations = [
		observation('a', '2026-09-16T10:00:00.000Z'),
		observation('a', '2026-09-16T10:00:00.100Z'),
		observation('b', '2026-09-16T10:00:00.100Z'),
		observation('a', '2026-09-16T10:00:00.200Z')
	];

	it('produces a session the shared player can drive', async () => {
		const { session, frameCount } = createLiveSceneSession({
			observations,
			sensorId: 'hesai-pandar40p',
			title: 'Run 42'
		});

		expect(session).toBeInstanceOf(SceneSession);
		expect(frameCount).toBe(3);
		expect(session.title).toBe('Run 42');
		expect(session.duration).toBeCloseTo(0.2);

		// Seeking goes through the shared session logic.
		const atStart = await session.frameAt(0);
		expect(atStart.frame?.f).toBe(0);
		const midway = await session.frameAt(0.15);
		expect(midway.frame?.f).toBe(1);
	});

	it('reconstructs trail history through the shared session', async () => {
		const { session } = createLiveSceneSession({
			observations,
			sensorId: 's'
		});

		// A trail built from recorded samples, so a direct seek and sequential
		// playback arrive at the same trail.
		const history = await session.trailHistory(0.2, 0.15);
		expect(history.frames.map((f) => f.f)).toEqual([1, 2]);
		expect(history.fromUs).toBe(50_000);
		expect(history.toUs).toBe(200_000);
	});

	it('declares the ENU frame the renderer requires', () => {
		// These coordinates are already in the frame the exporter declares for
		// a published scene, so the renderer Z-up to Y-up mapping applies
		// identically.
		const { session } = createLiveSceneSession({
			observations,
			sensorId: 'hesai-pandar40p',
			buildVersion: '0.5.2-test'
		});

		const header = session.parts[0].header!;
		expect(header.coordinate_frame?.reference_frame).toBe('ENU');
		expect(header.sensor_id).toBe('hesai-pandar40p');
		expect(header.build_version).toBe('0.5.2-test');
		expect(header.frame_count).toBe(3);
	});

	it('handles an empty run without throwing', async () => {
		const { session, frameCount } = createLiveSceneSession({
			observations: [],
			sensorId: 's'
		});

		expect(frameCount).toBe(0);
		expect(session.duration).toBe(0);
		expect((await session.frameAt(0)).frame).toBeNull();
	});

	it('reports skipped observations so a partial load is visible', () => {
		const { skippedObservations } = createLiveSceneSession({
			observations: [observation('a', 'rubbish'), observation('a', '2026-09-16T10:00:00.000Z')],
			sensorId: 's'
		});

		expect(skippedObservations).toBe(1);
	});
});
