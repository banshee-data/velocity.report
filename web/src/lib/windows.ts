// Presentation helpers for the Windows page's held-out view. A held-out
// window's state and progress come from two server reads: the ranking (GET
// /api/lidar/segments), which says whether a clip and a pack were made from
// the window, and the labelling workspace (GET /api/lidar/workspace), which
// reads review and freeze state from the label stores. These helpers combine
// what those reads said and decide nothing: in particular, whether a window
// may be frozen is the server's verdict, made on the Labelling page.

import type { PlanProgress, WorkspaceRow, WorkspaceSegment } from './labellingWorkspace';
import type { SegmentSelector } from './segments';
import { statusText } from './segments';

/** The colour a state chip takes, by DESIGN.md §3.2. */
export type StateTone = 'neutral' | 'progress' | 'success' | 'sealed';

export type WindowState = { state: string; tone: StateTone };

// Workspace stages in the words the Windows page uses. A pack whose every
// object is reviewed is ready to freeze, which this page calls reviewed.
const STAGE_STATE: Record<string, WindowState> = {
	frozen: { state: 'frozen', tone: 'sealed' },
	ready_to_freeze: { state: 'reviewed', tone: 'success' },
	in_review: { state: 'in review', tone: 'progress' },
	labelling: { state: 'labelling', tone: 'progress' },
	protected: { state: 'protected', tone: 'neutral' },
	unavailable: { state: 'unavailable', tone: 'neutral' }
};

/**
 * A held-out window's state: the workspace's stage once the label stores
 * have something to say about it, else the ranking's status (candidate,
 * clip made, packing, packed). An unlabelled stage says no more than the
 * ranking does, so the ranking's word is kept.
 */
export function windowState(status: string, row?: WorkspaceRow): WindowState {
	const staged = row ? STAGE_STATE[row.stage] : undefined;
	if (staged) return staged;
	if (status === 'candidate') return { state: 'candidate', tone: 'neutral' };
	return { state: statusText(status), tone: 'progress' };
}

/** Tailwind classes for a state chip. */
export function toneClass(tone: StateTone): string {
	switch (tone) {
		case 'progress':
			return 'bg-amber-100 text-amber-900';
		case 'success':
			return 'bg-green-100 text-green-800';
		case 'sealed':
			return 'bg-green-700 text-green-50';
		default:
			return 'border border-surface-content/30 text-surface-content/70';
	}
}

/**
 * What a held-out window still needs, in words. A window with no workspace
 * row has no pack the label stores know about yet.
 */
export function progressText(status: string, row?: WorkspaceRow): string {
	if (row?.stage === 'frozen') {
		const split = row.splits[0];
		return split ? `sealed in ${split.name}` : 'sealed';
	}
	const p = row?.progress;
	if (p && p.objects > 0) {
		return `${p.reviewed_objects} of ${p.objects} objects reviewed`;
	}
	if (p && p.proposed_objects > 0) return `${p.proposed_objects} proposals ready`;
	if (row?.packs.length) return 'pack cut, no objects yet';
	if (status === 'candidate') return 'not selected';
	if (status === 'case') return 'selected, pack not cut';
	if (status === 'clipping') return 'cutting the pack';
	return 'pack cut';
}

/**
 * The selectors the Windows page offers for a held-out pack: the random draw
 * only, so the pack is not shaped by what the finders look for. The server
 * also accepts the following and exposure selectors for held-out windows;
 * the page does not offer them.
 */
export function heldOutSelectors(selectors: SegmentSelector[]): SegmentSelector[] {
	return selectors.filter((selector) => selector.held_out && selector.finder === 'random');
}

/** The locked selector's read-out: "Random · 4 windows · seed 1". */
export function drawText(selector: SegmentSelector | undefined, count: number): string {
	if (!selector) return 'No random selector is configured';
	const seed = selector.parameters.random_seed;
	const windows = `${count} ${count === 1 ? 'window' : 'windows'}`;
	return seed === undefined ? `Random · ${windows}` : `Random · ${windows} · seed ${seed}`;
}

/** The workspace row of each held-out window, by window id. */
export function rowsBySegment(items: WorkspaceRow[]): Map<string, WorkspaceRow> {
	const out = new Map<string, WorkspaceRow>();
	for (const row of items) {
		for (const segment of row.segments) {
			// A segment row is the fallback; a pack or label set row has the
			// review state, so it wins when both name the window.
			if (!out.has(segment.segment_id) || row.kind !== 'segment') out.set(segment.segment_id, row);
		}
	}
	return out;
}

/**
 * Held-out windows already selected on this run that the random draw does
 * not list: windows chosen by an earlier traffic selector, or by another
 * seed. They belong to the pack, so the page lists and counts them too.
 */
export function otherHeldOut(
	items: WorkspaceRow[],
	runID: string,
	drawn: Set<string>
): WorkspaceSegment[] {
	const seen = new Set<string>();
	const out: WorkspaceSegment[] = [];
	for (const row of items) {
		for (const segment of row.segments) {
			if (segment.role !== 'held_out' || segment.run_id !== runID) continue;
			if (drawn.has(segment.segment_id) || seen.has(segment.segment_id)) continue;
			seen.add(segment.segment_id);
			out.push(segment);
		}
	}
	return out;
}

/** "1 of 4 frozen", over the windows the page lists. */
export function frozenText(states: WindowState[]): string {
	const frozen = states.filter((entry) => entry.state === 'frozen').length;
	return `${frozen} of ${states.length} frozen`;
}

/** The plan bound to a row, if any. */
export function boundPlan(plans: PlanProgress[], key: string): PlanProgress | undefined {
	return plans.find((plan) => plan.binding?.item_key === key);
}
