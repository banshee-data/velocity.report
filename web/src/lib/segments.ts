// Presentation helpers for the Segments page. The selectors, their rules and
// every ranking come from the server (GET /api/lidar/segments/selectors and
// GET /api/lidar/segments), which reads config/segment-selectors.defaults.json.
// These helpers group and label what the server served and decide nothing:
// in particular, whether a selector may choose held-out windows is the
// server's verdict, shown as it arrives.

export type SegmentScore = { measure: string; order: 'descending' | 'ascending' };

export type SegmentRequirement = { measure: string; min: number | null; max: number | null };

export type SegmentSelector = {
	id: string;
	label: string;
	category: string;
	description: string;
	finder: string;
	parameters: Record<string, number>;
	score: SegmentScore;
	require: SegmentRequirement[];
	digest: string;
	held_out: boolean;
};

export type SelectorGroup = { category: string; selectors: SegmentSelector[] };

/** The measures of a following pass, shown as the table's detail column. */
const FOLLOWING_FINDERS = new Set(['following', 'leader_changes']);

/**
 * The selectors a role may use, grouped by category. Categories keep the
 * order in which the file first uses them, and selectors the file's order.
 */
export function selectorGroups(selectors: SegmentSelector[], role: string): SelectorGroup[] {
	const groups: SelectorGroup[] = [];
	for (const selector of selectors) {
		if (role === 'held_out' && !selector.held_out) continue;
		let group = groups.find((entry) => entry.category === selector.category);
		if (!group) {
			group = { category: selector.category, selectors: [] };
			groups.push(group);
		}
		group.selectors.push(selector);
	}
	return groups;
}

/** The selector to keep after the role changes: the same one if the role allows it. */
export function keepSelector(current: string, groups: SelectorGroup[]): string {
	const ids = groups.flatMap((group) => group.selectors.map((selector) => selector.id));
	return ids.includes(current) ? current : (ids[0] ?? '');
}

/** The query that ranks a run's windows with a selector for a role. */
export function segmentQuery(runID: string, selector: string, role: string): string {
	return new URLSearchParams({ run_id: runID, selector, role }).toString();
}

/** What a selector requires, in words: "pair_seconds ≥ 2". Empty when nothing. */
export function requirementText(selector: SegmentSelector): string {
	return selector.require
		.map((bound) => {
			if (bound.min !== null && bound.max !== null)
				return `${bound.min} ≤ ${bound.measure} ≤ ${bound.max}`;
			if (bound.min !== null) return `${bound.measure} ≥ ${bound.min}`;
			return `${bound.measure} ≤ ${bound.max}`;
		})
		.join(', ');
}

/** How a selector ranks, in words: "pair_seconds, largest first". */
export function scoreText(selector: SegmentSelector): string {
	const order = selector.score.order === 'ascending' ? 'smallest first' : 'largest first';
	return `${selector.score.measure}, ${order}`;
}

export type SegmentMeasures = {
	pair_seconds?: number;
	pairs?: number;
	leaders?: number;
	leader_changes?: number;
	closest_gap_m?: number;
	events?: number;
};

/** The table's detail column for a window, by the finder that ranked it. */
export function segmentDetail(segment: SegmentMeasures, finder: string): string {
	if (FOLLOWING_FINDERS.has(finder)) {
		return (
			`${segment.pair_seconds ?? 0} pair-s · ${segment.pairs ?? 0} pairs · ` +
			`${segment.leaders ?? 0} leaders · ${segment.leader_changes ?? 0} changes · ` +
			`${segment.closest_gap_m ?? 0} m`
		);
	}
	return `${segment.events ?? 0} observations`;
}

// The server's window states predate the platform vocabulary: a window with a
// clip made from it is "case", and one whose pack job is queued or running is
// "clipping", from when the pack was called a clip. Shown in today's words;
// pack review states (packed, proposed, reviewed) already are.
const STATUS_TEXT: Record<string, string> = {
	case: 'clip made',
	clipping: 'packing'
};

/** The table's status column for a window or its pack. */
export function statusText(status: string): string {
	return STATUS_TEXT[status] ?? status;
}
