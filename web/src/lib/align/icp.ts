/**
 * Two-dimensional ICP of a scan against map outlines, in the browser.
 *
 * The scan is the settled background's points seen from above; the target is
 * line segments (building outlines, walls, kerbs) in the same local east/north
 * frame. Each iteration pairs every scan point with the nearest point on a
 * segment within the gate, solves the rigid motion that best moves the pairs
 * onto each other (Arun/Horn in 2D, rotation about the sensor), and shrinks the
 * gate. It refines a pose an operator has already placed roughly; it does not
 * search for one.
 */
import type { EN } from './pose';

export interface Segment {
	a: EN;
	b: EN;
}

export interface IcpOptions {
	/** Pairs further apart than this, in metres, are ignored. Starts here. */
	maxDistance?: number;
	/** The gate never shrinks below this. */
	minDistance?: number;
	maxIterations?: number;
	/** Stop when an iteration moves less than this many metres and degrees. */
	tolerance?: number;
	/** Grid cell for the nearest-segment search, metres. */
	cellSize?: number;
}

export interface IcpResult {
	/** Translation of the sensor, metres east and north. */
	dEast: number;
	dNorth: number;
	/** Rotation about the sensor, degrees clockwise (a heading change). */
	dHeadingDeg: number;
	/** Root-mean-square distance of the final pairs, metres. */
	rms: number;
	/** How many scan points were paired at the end, and of how many. */
	inliers: number;
	total: number;
	iterations: number;
}

/** The point on segment s nearest p. */
export function nearestOnSegment(p: EN, s: Segment): EN {
	const dx = s.b.east - s.a.east;
	const dy = s.b.north - s.a.north;
	const len2 = dx * dx + dy * dy;
	if (len2 === 0) return s.a;
	let t = ((p.east - s.a.east) * dx + (p.north - s.a.north) * dy) / len2;
	t = Math.max(0, Math.min(1, t));
	return { east: s.a.east + t * dx, north: s.a.north + t * dy };
}

/** A uniform grid over segments, so a nearest-segment query looks at a few. */
export class SegmentIndex {
	private cells = new Map<string, Segment[]>();

	constructor(
		segments: Segment[],
		private cellSize: number
	) {
		for (const s of segments) {
			const x0 = Math.floor(Math.min(s.a.east, s.b.east) / cellSize);
			const x1 = Math.floor(Math.max(s.a.east, s.b.east) / cellSize);
			const y0 = Math.floor(Math.min(s.a.north, s.b.north) / cellSize);
			const y1 = Math.floor(Math.max(s.a.north, s.b.north) / cellSize);
			for (let x = x0; x <= x1; x++) {
				for (let y = y0; y <= y1; y++) {
					const key = `${x},${y}`;
					const list = this.cells.get(key);
					if (list) list.push(s);
					else this.cells.set(key, [s]);
				}
			}
		}
	}

	/** The nearest point on any segment within maxDistance of p, or null. */
	nearest(p: EN, maxDistance: number): { point: EN; distance: number } | null {
		const r = Math.ceil(maxDistance / this.cellSize);
		const cx = Math.floor(p.east / this.cellSize);
		const cy = Math.floor(p.north / this.cellSize);
		let best: { point: EN; distance: number } | null = null;
		const seen = new Set<Segment>();
		for (let x = cx - r; x <= cx + r; x++) {
			for (let y = cy - r; y <= cy + r; y++) {
				for (const s of this.cells.get(`${x},${y}`) ?? []) {
					if (seen.has(s)) continue;
					seen.add(s);
					const q = nearestOnSegment(p, s);
					const d = Math.hypot(q.east - p.east, q.north - p.north);
					if (d <= maxDistance && (!best || d < best.distance)) best = { point: q, distance: d };
				}
			}
		}
		return best;
	}
}

/**
 * The rigid motion (rotation about pivot, then translation) that best maps
 * from onto to, in the least-squares sense. Returns radians counter-clockwise.
 */
export function solveRigid(
	from: EN[],
	to: EN[],
	pivot: EN
): { theta: number; tEast: number; tNorth: number } {
	const n = from.length;
	let fx = 0,
		fy = 0,
		tx = 0,
		ty = 0;
	for (let i = 0; i < n; i++) {
		fx += from[i].east - pivot.east;
		fy += from[i].north - pivot.north;
		tx += to[i].east - pivot.east;
		ty += to[i].north - pivot.north;
	}
	fx /= n;
	fy /= n;
	tx /= n;
	ty /= n;
	let sxx = 0,
		sxy = 0;
	for (let i = 0; i < n; i++) {
		const ax = from[i].east - pivot.east - fx;
		const ay = from[i].north - pivot.north - fy;
		const bx = to[i].east - pivot.east - tx;
		const by = to[i].north - pivot.north - ty;
		sxx += ax * bx + ay * by;
		sxy += ax * by - ay * bx;
	}
	const theta = Math.atan2(sxy, sxx);
	const c = Math.cos(theta);
	const s = Math.sin(theta);
	// Rotate the source centroid about the pivot, then translate onto the target's.
	return { theta, tEast: tx - (c * fx - s * fy), tNorth: ty - (s * fx + c * fy) };
}

/**
 * Refines a placed scan against segments. scan is in the local frame already;
 * pivot is the sensor's position there. The result is the move to apply to the
 * pose: rotate by dHeadingDeg about the sensor, then shift by dEast, dNorth.
 */
export function icp2d(
	scan: EN[],
	segments: Segment[],
	pivot: EN,
	opts: IcpOptions = {}
): IcpResult {
	const maxDistance = opts.maxDistance ?? 4;
	const minDistance = opts.minDistance ?? 0.5;
	const maxIterations = opts.maxIterations ?? 40;
	const tolerance = opts.tolerance ?? 1e-3;
	const index = new SegmentIndex(segments, opts.cellSize ?? 2);

	let theta = 0;
	let tEast = 0;
	let tNorth = 0;
	let gate = maxDistance;
	let rms = Infinity;
	let inliers = 0;
	let iterations = 0;

	for (; iterations < maxIterations; iterations++) {
		const c = Math.cos(theta);
		const s = Math.sin(theta);
		const from: EN[] = [];
		const to: EN[] = [];
		let sum = 0;
		for (const p of scan) {
			const rx = p.east - pivot.east;
			const ry = p.north - pivot.north;
			const moved = {
				east: pivot.east + c * rx - s * ry + tEast,
				north: pivot.north + s * rx + c * ry + tNorth
			};
			const hit = index.nearest(moved, gate);
			if (!hit) continue;
			from.push(p);
			to.push(hit.point);
			sum += hit.distance * hit.distance;
		}
		inliers = from.length;
		if (inliers < 3) break;
		rms = Math.sqrt(sum / inliers);
		const step = solveRigid(from, to, pivot);
		const moved =
			Math.hypot(step.tEast - tEast, step.tNorth - tNorth) +
			(Math.abs(step.theta - theta) * 180) / Math.PI;
		theta = step.theta;
		tEast = step.tEast;
		tNorth = step.tNorth;
		gate = Math.max(minDistance, Math.min(gate, 3 * rms));
		if (moved < tolerance) {
			iterations++;
			break;
		}
	}
	return {
		dEast: tEast,
		dNorth: tNorth,
		// Counter-clockwise in east/north is a heading decrease.
		dHeadingDeg: (-theta * 180) / Math.PI,
		rms: inliers >= 3 ? rms : Infinity,
		inliers,
		total: scan.length,
		iterations
	};
}

/** Keeps one point per cell of a square grid: an even, lighter scan for ICP. */
export function thin(points: EN[], cell: number): EN[] {
	const seen = new Set<string>();
	const out: EN[] = [];
	for (const p of points) {
		const key = `${Math.floor(p.east / cell)},${Math.floor(p.north / cell)}`;
		if (seen.has(key)) continue;
		seen.add(key);
		out.push(p);
	}
	return out;
}
