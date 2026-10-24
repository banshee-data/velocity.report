/**
 * The street editor's marks and their unsaved changes.
 *
 * A mark is one driveway, parked-car space or travel lane as the operator
 * edits it: its vertices in WGS 84 (heights kept where Gerardus gave them; a
 * new vertex has none, and Gerardus drapes it), and its tags. The marks
 * loaded from Gerardus are the base; the batch Save sends is the difference
 * between the base and the marks now, so a mark moved and moved back sends
 * nothing. Every change keeps the previous marks for Undo.
 */
import {
	STREET_USE_KINDS,
	type GeoJSONFeature,
	type Position,
	type SiteFeature,
	type StreetUseEdit,
	type StreetUseKind
} from './gerardus';
import { openRing } from './geometry';

export interface Mark {
	/** Stable within the page: the server id, or a local key for a mark not saved yet. */
	key: string;
	/** Gerardus's id; empty for a new mark. */
	id: string;
	kind: StreetUseKind;
	/** Vertices, longitude first. A space's ring is open: its last vertex is not its first. */
	points: Position[];
	tags: Record<string, string>;
	author?: string;
	editedAt?: string;
}

export interface EditState {
	base: Mark[];
	marks: Mark[];
	/** Earlier marks, most recent last, for Undo. */
	past: Mark[][];
}

/** How many changes Undo can step back through. */
export const UNDO_DEPTH = 200;

/** The tag values each kind offers (Gerardus's proto comments list them). */
export const SPACE_TYPES = ['metered', 'permit', 'loading', 'disabled', 'free'] as const;
export const LANE_TYPES = ['general', 'bus', 'bike', 'turn'] as const;

/** The fewest vertices a kind needs, and the most (driveways are one span). */
export const VERTEX_LIMITS: Record<StreetUseKind, { min: number; max: number }> = {
	driveway: { min: 2, max: 2 },
	parking_space: { min: 3, max: Infinity },
	travel_lane: { min: 2, max: Infinity }
};

export function isStreetUseKind(kind: string): kind is StreetUseKind {
	return (STREET_USE_KINDS as readonly string[]).includes(kind);
}

let localCounter = 0;

/** A key for a mark that has no id yet. */
export function newKey(): string {
	localCounter += 1;
	return `new-${localCounter}`;
}

/**
 * The street-use features as marks. Features of another kind, or whose
 * geometry does not fit their kind, are left out and counted.
 */
export function marksFromFeatures(features: SiteFeature[]): { marks: Mark[]; skipped: number } {
	const marks: Mark[] = [];
	let skipped = 0;
	for (const f of features) {
		if (!isStreetUseKind(f.kind)) continue;
		let points: Position[] | null = null;
		if (f.kind === 'parking_space' && f.geometry.type === 'Polygon') {
			points = openRing(f.geometry.coordinates[0] ?? []);
		} else if (f.kind !== 'parking_space' && f.geometry.type === 'LineString') {
			points = f.geometry.coordinates;
		}
		if (!points || points.length < VERTEX_LIMITS[f.kind].min) {
			skipped++;
			continue;
		}
		const mark: Mark = {
			key: f.id || newKey(),
			id: f.id,
			kind: f.kind,
			points: points.map((p) => [...p] as Position),
			tags: { ...f.tags }
		};
		if (f.author) mark.author = f.author;
		if (f.editedAt) mark.editedAt = f.editedAt;
		marks.push(mark);
	}
	return { marks, skipped };
}

/**
 * A mark as the GeoJSON Feature an upsert carries: `kind`, `id` (empty for a
 * new mark) and its non-empty tags as properties. A space is a closed
 * Polygon; a driveway and a lane are LineStrings, a lane in its direction of
 * travel.
 */
export function markToFeature(mark: Mark): GeoJSONFeature {
	const properties: Record<string, unknown> = { kind: mark.kind, id: mark.id };
	for (const [key, value] of Object.entries(mark.tags)) {
		const v = value.trim();
		if (v !== '' && key !== 'kind' && key !== 'id') properties[key] = v;
	}
	const points = mark.points.map((p) => [...p] as Position);
	const geometry: GeoJSONFeature['geometry'] =
		mark.kind === 'parking_space'
			? { type: 'Polygon', coordinates: [[...points, [...points[0]] as Position]] }
			: { type: 'LineString', coordinates: points };
	return { type: 'Feature', geometry, properties };
}

function cleanTags(tags: Record<string, string>): Record<string, string> {
	const out: Record<string, string> = {};
	for (const key of Object.keys(tags).sort()) {
		const v = tags[key].trim();
		if (v !== '') out[key] = v;
	}
	return out;
}

/** Whether two marks would send the same feature: same kind, vertices and tags. */
export function sameMark(a: Mark, b: Mark): boolean {
	if (a.kind !== b.kind || a.points.length !== b.points.length) return false;
	for (let i = 0; i < a.points.length; i++) {
		if (a.points[i][0] !== b.points[i][0] || a.points[i][1] !== b.points[i][1]) return false;
	}
	return JSON.stringify(cleanTags(a.tags)) === JSON.stringify(cleanTags(b.tags));
}

/**
 * The batch that turns base into marks: an upsert for each new or changed
 * mark, a remove for each saved mark no longer there. Removes come first.
 */
export function diffEdits(base: Mark[], marks: Mark[]): StreetUseEdit[] {
	const byId = new Map(base.filter((m) => m.id).map((m) => [m.id, m]));
	const kept = new Set(marks.filter((m) => m.id).map((m) => m.id));
	const edits: StreetUseEdit[] = [];
	for (const m of base) if (m.id && !kept.has(m.id)) edits.push({ remove: m.id });
	for (const m of marks) {
		const before = m.id ? byId.get(m.id) : undefined;
		if (!before || !sameMark(before, m)) edits.push({ upsert: markToFeature(m) });
	}
	return edits;
}

// ---- Edit state -----------------------------------------------------------------

export function startEditing(base: Mark[]): EditState {
	return { base, marks: base, past: [] };
}

/** Replaces the marks, keeping the previous ones for Undo. */
export function change(state: EditState, marks: Mark[]): EditState {
	const past = [...state.past, state.marks];
	if (past.length > UNDO_DEPTH) past.splice(0, past.length - UNDO_DEPTH);
	return { ...state, marks, past };
}

export function undo(state: EditState): EditState {
	if (state.past.length === 0) return state;
	return { ...state, marks: state.past[state.past.length - 1], past: state.past.slice(0, -1) };
}

export function canUndo(state: EditState): boolean {
	return state.past.length > 0;
}

/** The batch Save would send now. */
export function pendingEdits(state: EditState): StreetUseEdit[] {
	return diffEdits(state.base, state.marks);
}

export function addMark(state: EditState, mark: Mark): EditState {
	return change(state, [...state.marks, mark]);
}

export function removeMark(state: EditState, key: string): EditState {
	if (!state.marks.some((m) => m.key === key)) return state;
	return change(
		state,
		state.marks.filter((m) => m.key !== key)
	);
}

/** Applies fn to the mark with key, as one undoable change. */
export function updateMark(state: EditState, key: string, fn: (m: Mark) => Mark): EditState {
	let found = false;
	const marks = state.marks.map((m) => {
		if (m.key !== key) return m;
		found = true;
		return fn(m);
	});
	return found ? change(state, marks) : state;
}

/** Moves one vertex. A new position has no height: Gerardus drapes it. */
export function moveVertex(mark: Mark, index: number, to: [number, number]): Mark {
	if (index < 0 || index >= mark.points.length) return mark;
	const points = mark.points.slice();
	points[index] = [to[0], to[1]];
	return { ...mark, points };
}

/** Removes one vertex, unless the mark would have fewer than its kind needs. */
export function removeVertex(mark: Mark, index: number): Mark {
	if (index < 0 || index >= mark.points.length) return mark;
	if (mark.points.length - 1 < VERTEX_LIMITS[mark.kind].min) return mark;
	return { ...mark, points: mark.points.filter((_, i) => i !== index) };
}

/** A lane the other way round: its direction of travel is the order of its vertices. */
export function reverseMark(mark: Mark): Mark {
	return { ...mark, points: mark.points.slice().reverse() };
}

export function setTag(mark: Mark, key: string, value: string): Mark {
	return { ...mark, tags: { ...mark.tags, [key]: value } };
}

/** The tags a new mark of each kind starts with, from the tool's fields. */
export function tagsFor(
	kind: StreetUseKind,
	fields: { spaceType: string; count: number; timeLimit: string; laneType: string; widthM: number }
): Record<string, string> {
	if (kind === 'parking_space') {
		const tags: Record<string, string> = {
			space_type: fields.spaceType,
			count: String(Math.max(1, Math.round(fields.count) || 1))
		};
		if (fields.timeLimit.trim()) tags.time_limit = fields.timeLimit.trim();
		return tags;
	}
	if (kind === 'travel_lane') {
		const tags: Record<string, string> = { lane_type: fields.laneType };
		if (Number.isFinite(fields.widthM) && fields.widthM > 0) tags.width_m = String(fields.widthM);
		return tags;
	}
	return {};
}
