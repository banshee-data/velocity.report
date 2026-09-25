/**
 * Adapts live database track observations to the shared scene format.
 *
 * The operator tools and the public scenes now render through the same
 * three.js player. They differ only in where frames come from: a published
 * scene reads static gzipped chunks, while the tracks page reads a run out of
 * the database. Rather than write a second player against a second clock,
 * this builds a `PartReader` whose chunk happens to be in memory, so the
 * shared code's seeking, playback and trail reconstruction apply unchanged.
 */

import type { SceneChunk, SceneFrame, SceneHeader, SceneTrack } from '$scene/scene-reader.js';
import { PartReader, SceneSession } from '$scene/scene-reader.js';
import type { RunTrack, TrackObservation } from '$lib/types/lidar';

/** Microseconds in one millisecond, named so the conversions read plainly. */
const US_PER_MS = 1000;

export interface LiveFrameBuildResult {
	frames: SceneFrame[];
	/** Absolute capture time of the first frame, nanoseconds as a string. */
	startNs: string;
	/** Observations that carried an unusable timestamp and were dropped. */
	skipped: number;
}

/**
 * Groups observations into frames.
 *
 * Observations are grouped by their exact recorded timestamp: the pipeline
 * writes every track in one sensor rotation with that rotation's frame time,
 * so an exact match is the frame boundary the recording actually had. Nothing
 * is bucketed or rounded into a synthetic frame rate, because a tolerance
 * wide enough to join two tracks is also wide enough to merge two rotations.
 *
 * Frame `t` is a microsecond offset from the first frame, matching the export
 * format: absolute nanosecond capture times exceed `Number.MAX_SAFE_INTEGER`
 * and would lose precision as JSON numbers.
 */
export function buildLiveFrames(
	observations: TrackObservation[],
	runTracks: RunTrack[] = []
): LiveFrameBuildResult {
	// An operator's own label wins over the classifier's: the point of the
	// labelling workflow is that a person's judgement is the better answer.
	const classByTrack = new Map<string, { label?: string; confidence?: number }>();
	for (const track of runTracks) {
		classByTrack.set(track.track_id, {
			label: track.user_label || track.object_class || undefined,
			confidence: track.user_label ? track.label_confidence : track.object_confidence
		});
	}

	// Peak speed is a track-lifetime property, but the renderer reads it per
	// frame, so it is resolved once here rather than recomputed per frame.
	const peakSpeed = new Map<string, number>();
	for (const observation of observations) {
		const current = peakSpeed.get(observation.track_id) ?? 0;
		if (observation.speed_mps > current) {
			peakSpeed.set(observation.track_id, observation.speed_mps);
		}
	}

	let skipped = 0;
	const byTimestamp = new Map<number, TrackObservation[]>();
	for (const observation of observations) {
		const ms = Date.parse(observation.timestamp);
		if (!Number.isFinite(ms)) {
			// A row we cannot place in time cannot be drawn in time either.
			skipped += 1;
			continue;
		}
		const bucket = byTimestamp.get(ms);
		if (bucket) bucket.push(observation);
		else byTimestamp.set(ms, [observation]);
	}

	const timestamps = [...byTimestamp.keys()].sort((a, b) => a - b);
	if (timestamps.length === 0) {
		return { frames: [], startNs: '0', skipped };
	}

	const firstMs = timestamps[0];
	const frames: SceneFrame[] = timestamps.map((ms, index) => ({
		f: index,
		t: Math.round((ms - firstMs) * US_PER_MS),
		tr: (byTimestamp.get(ms) ?? []).map((observation) =>
			toSceneTrack(observation, classByTrack, peakSpeed)
		)
	}));

	return {
		frames,
		// Milliseconds to nanoseconds, kept as a string for the same reason
		// the export does: the value is past float precision.
		startNs: `${firstMs}000000`,
		skipped
	};
}

function toSceneTrack(
	observation: TrackObservation,
	classByTrack: Map<string, { label?: string; confidence?: number }>,
	peakSpeed: Map<string, number>
): SceneTrack {
	const classification = classByTrack.get(observation.track_id);
	return {
		id: observation.track_id,
		x: observation.position.x,
		y: observation.position.y,
		z: observation.position.z,
		vx: observation.velocity.vx,
		vy: observation.velocity.vy,
		spd: observation.speed_mps,
		mspd: peakSpeed.get(observation.track_id) ?? observation.speed_mps,
		hdg: observation.heading_rad,
		l: observation.bounding_box.length,
		w: observation.bounding_box.width,
		h: observation.bounding_box.height,
		// The recorded heading is the box heading for a live observation:
		// there is no separate per-frame OBB yaw on this endpoint, and
		// inventing one would rotate boxes on no evidence.
		bh: observation.heading_rad,
		c: classification?.label,
		cf: classification?.confidence
	};
}

/**
 * A part whose single chunk is already in memory.
 *
 * Everything the shared reader does with a part — the chunk binary search,
 * `frameAtOffset`, `framesInRange`, trail reconstruction — derives from
 * `chunks` and `loadChunk`, so overriding just those two gives the live
 * source all of it.
 */
export class LivePart extends PartReader {
	private readonly frames: SceneFrame[];

	constructor(frames: SceneFrame[], header: SceneHeader) {
		// The base class only uses the URL to fetch chunks, which this part
		// never does. The scheme is deliberately not http so an accidental
		// fetch fails loudly rather than hitting the page's own origin.
		super('live:///');
		this.frames = frames;
		this.header = header;
		this.chunks = LivePart.chunkFor(frames);
	}

	private static chunkFor(frames: SceneFrame[]): SceneChunk[] {
		if (frames.length === 0) {
			// One empty chunk rather than none: the reader indexes chunks[0]
			// unconditionally, and an empty run is a real answer — the street
			// was quiet — not a malformed part.
			return [{ c: 0, n: 0, t0: 0, t1: 0 }];
		}
		return [
			{
				c: 0,
				n: frames.length,
				t0: frames[0].t,
				t1: frames[frames.length - 1].t
			}
		];
	}

	/** Resolves immediately: the frames are already here. */
	override async loadChunk(id: number): Promise<SceneFrame[]> {
		return id === 0 ? this.frames : [];
	}

	/** Nothing to warm. */
	override prefetchAfter(): void {}
}

export interface LiveSceneSessionOptions {
	observations: TrackObservation[];
	runTracks?: RunTrack[];
	sensorId: string;
	title?: string;
	/** Build version to show as the scene's generator, when known. */
	buildVersion?: string;
}

export interface LiveSceneSessionResult {
	session: SceneSession;
	frameCount: number;
	skippedObservations: number;
}

/**
 * Builds a scene session over live observations.
 *
 * The header declares ENU because that is the frame these coordinates are
 * already in — the same frame the exporter declares for a published scene —
 * so the renderer's Z-up to Y-up mapping applies identically.
 */
export function createLiveSceneSession(options: LiveSceneSessionOptions): LiveSceneSessionResult {
	const { frames, startNs, skipped } = buildLiveFrames(
		options.observations,
		options.runTracks ?? []
	);

	const header: SceneHeader = {
		version: 1,
		sensor_id: options.sensorId,
		frame_count: frames.length,
		start_ns: startNs,
		duration_sec: frames.length ? (frames[frames.length - 1].t - frames[0].t) / 1e6 : 0,
		build_version: options.buildVersion,
		coordinate_frame: { frame_id: options.sensorId, reference_frame: 'ENU' }
	};

	const part = new LivePart(frames, header);
	return {
		session: SceneSession.fromParts([part], { title: options.title ?? 'Live run' }),
		frameCount: frames.length,
		skippedObservations: skipped
	};
}
