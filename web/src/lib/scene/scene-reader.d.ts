/**
 * Types for the shared scene reader in public_html/src/js.
 *
 * The scene modules are the same files the public scenes run on, imported
 * here through the `$scene` alias rather than copied. Only the surface the
 * operator tools use is declared.
 */
declare module '$scene/scene-reader.js' {
	/** One frame as the scene format carries it. */
	export interface SceneFrame {
		/** Frame ordinal in the source recording. */
		f: number;
		/** Microsecond offset from the part's first frame. */
		t: number;
		/** Tracks present in this frame. */
		tr?: SceneTrack[];
		/** Foreground points as [x, y, z, intensity]. */
		p?: [number, number, number, number][];
	}

	export interface SceneTrack {
		id: string;
		x: number;
		y: number;
		z: number;
		vx: number;
		vy: number;
		spd: number;
		mspd?: number;
		hdg: number;
		l: number;
		w: number;
		h: number;
		bh: number;
		c?: string;
		cf?: number;
	}

	export interface SceneChunk {
		/** Chunk id. */
		c: number;
		/** Frames in the chunk. */
		n: number;
		/** First frame offset, microseconds. */
		t0: number;
		/** Last frame offset, microseconds. */
		t1: number;
	}

	export interface SceneHeader {
		version?: number;
		sensor_id?: string;
		frame_count?: number;
		start_ns?: string;
		duration_sec?: number;
		build_version?: string;
		coordinate_frame?: { frame_id?: string; reference_frame?: string };
	}

	export class SceneError extends Error {
		constructor(message: string, cause?: unknown);
	}

	export class PartReader {
		constructor(baseURL: string);
		baseURL: string;
		header: SceneHeader | null;
		chunks: SceneChunk[];
		readonly startUs: number;
		readonly endUs: number;
		readonly startNs: string;
		readonly durationSec: number;
		readonly frameCount: number;
		open(): Promise<this>;
		chunkIndexForOffset(us: number): number;
		loadChunk(id: number): Promise<SceneFrame[]>;
		framesInRange(fromUs: number, toUs: number): Promise<SceneFrame[]>;
		frameAtOffsetWithIndex(us: number): Promise<{ frame: SceneFrame; frameIndex: number } | null>;
		frameAtOffset(us: number): Promise<SceneFrame | null>;
		frameAtIndex(n: number): Promise<SceneFrame>;
		prefetchAfter(us: number): void;
	}

	export class SceneSession {
		constructor(manifestURL: string);
		parts: PartReader[];
		duration: number;
		readonly title: string;
		static fromParts(parts: PartReader[], options?: { title?: string }): SceneSession;
		open(): Promise<this>;
		locate(seconds: number): { partIndex: number; part: PartReader; us: number };
		frameAt(
			seconds: number
		): Promise<{ partIndex: number; frame: SceneFrame | null; sceneSeconds: number }>;
		trailHistory(
			seconds: number,
			historySeconds: number
		): Promise<{ partIndex: number; frames: SceneFrame[]; fromUs: number; toUs: number }>;
		trailHistoryForPart(
			partIndex: number,
			us: number,
			historySeconds: number
		): Promise<{ partIndex: number; frames: SceneFrame[]; fromUs: number; toUs: number }>;
	}
}

declare module '$scene/scene-player.js' {
	import type { SceneSession } from '$scene/scene-reader.js';

	export function mountScenePlayer(options: {
		canvas: HTMLCanvasElement;
		manifestURL?: string;
		pointCloudManifestURL?: string;
		ui: Record<string, unknown>;
		capture?: boolean;
		session?: SceneSession | null;
	}): Promise<unknown>;
}
