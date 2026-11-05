/**
 * The labelling workspace's client: types for GET /api/lidar/workspace and
 * its detail, the requests that change workflow state or review an object,
 * and the words the page shows. Every count, stage, role and eligibility
 * verdict is the server's; these helpers only say them.
 *
 * Nanosecond times and identities arrive as decimal strings and stay
 * strings, or BigInt for arithmetic: a JavaScript number holds 53 bits, and
 * a 2026 Unix time in nanoseconds needs 61.
 */

export type Count = { name: string; done: number; total: number | null; unit: string };

export type LayerProgress = {
	layer: string;
	title: string;
	status: string;
	where: string;
	counts: Count[];
	note?: string;
};

export type Task = {
	layer: string;
	action: string;
	reason: string;
	depends_on?: string;
	blocked: boolean;
	tbd?: string;
	needs: string[];
	where: string;
	handoff?: string;
	source_revision?: string;
	plan?: string;
};

export type WorkflowItem = {
	item_key: string;
	layer: string;
	state: string;
	assignee: string;
	reviewer: string;
	blocker: string;
	tbd_reason: string;
	note: string;
	source_revision: string;
	revision: number;
	updated_by: string;
	updated_at_ns: string;
	created_at_ns: string;
};

export type FrozenRef = {
	name: string;
	split_digest: string;
	revision: number;
	pack_digest: string;
	pinned_revision: number;
	head_revision: number;
	roles: string[];
	head_moved: boolean;
};

export type PhysicalProgress = {
	exists: boolean;
	revision: number;
	digest?: string;
	stale: number;
	proposed_records: { kind: string; object_id?: string; record_id: string }[];
	layers: LayerProgress[];
};

export type WorkspacePack = {
	handle: string;
	pack_digest: string;
	dataset_id: string;
	run: string;
	segment_id?: string;
	selection_role?: string;
	pcap_basename?: string;
	samples?: number;
	abs_dir?: string;
	vrlog?: string;
	physical?: PhysicalProgress;
	error?: string;
};

export type WorkspaceSegment = {
	segment_id: string;
	run_id?: string;
	clip_id: string;
	role: string;
	finder: string;
	selector?: string;
	held_out_eligible: boolean;
	capture: string;
	window_start_ns: string;
	window_end_ns: string;
	pack_dir?: string;
	pack_missing?: boolean;
};

export type WorkspaceRow = {
	key: string;
	kind: 'label_set' | 'pack' | 'segment';
	title: string;
	stage: string;
	flags: string[];
	protected: boolean;
	cohort: string;
	site_id?: string;
	roles: string[];
	captures: { basename: string; sha256?: string; verified: boolean }[];
	runs: string[];
	segments: WorkspaceSegment[];
	packs: WorkspacePack[];
	label_set?: { id: string; name: string; revision: number; state_digest: string; dir: string };
	splits: FrozenRef[];
	progress?: {
		objects: number;
		reviewed_objects: number;
		proposed_objects: number;
		masks: number;
		reviewed_masks: number;
		proposed_masks: number;
		freeze_blockers: string[];
		layers: LayerProgress[];
	};
	evaluation?: {
		purpose: string;
		held_out_score: boolean;
		split_digest?: string;
		allowed: boolean;
		reasons: string[];
	};
	workflow: WorkflowItem[];
	tasks: Task[];
	error?: string;
};

export type SourceState = { available: boolean; error?: string };

export type EligibilityStatus = {
	manifest: {
		configured: boolean;
		state: string;
		error?: string;
		digest?: string;
		revision?: number;
		cohorts: Record<string, { sites: number; captures: number }>;
	};
	registry: {
		splits: number;
		held_out_spans: number;
		tuned_spans: number;
		weak_identity: number;
		unreadable: string[];
		error?: string;
	};
	access: { qualification_storage_enforced: boolean; custodians_tbd: boolean };
	ledger_available: boolean;
	protection_complete: boolean;
	gaps: string[];
	purposes: { purpose: string; available: boolean; reason?: string }[];
	policy_digest: string;
};

export type Plan = {
	id: string;
	title: string;
	status: string;
	depends_on: string[];
	path: string;
	steps: { id: string; kind: string; human_estimate?: string; note?: string }[];
	error?: string;
};

export type StepProgress = {
	id: string;
	kind: string;
	status: 'done' | 'in_progress' | 'not_started' | 'blocked' | 'untracked';
	detail: string;
	next?: string;
	where: string;
	counts: Count[];
	note?: string;
};

export type PlanBinding = {
	plan_id: string;
	item_key: string;
	note: string;
	revision: number;
	bound_by: string;
	bound_at_ns: string;
};

export type PlanProgress = Plan & {
	binding?: PlanBinding;
	bound_title?: string;
	bound_stage?: string;
	bound_error?: string;
	waiting_on: string[];
	progress: StepProgress[];
	steps_done: number;
	next: string;
	where: string;
};

export type PlanCandidate = { key: string; title: string; stage: string; why: string };

export type WorkspaceResponse = {
	generated_at_ns: string;
	sources: Record<string, SourceState>;
	eligibility: EligibilityStatus;
	totals: {
		rows: number;
		label_sets: number;
		packs: number;
		captures: number;
		segments: number;
		runs: number;
		splits: number;
		layers: LayerProgress[];
		unreadable: number;
		outstanding_tasks: number;
		blocked_tasks: number;
		tbd_tasks: number;
		stages: Record<string, number>;
	};
	queue: (Task & { key: string; title: string })[];
	items: WorkspaceRow[];
	total: number;
	limit: number;
	offset: number;
	plans: { available: boolean; error?: string; plans: PlanProgress[] };
	candidates: PlanCandidate[];
	facets: Record<string, string[]>;
};

export type ObjectSpan = {
	object_id: string;
	class: string;
	status: string;
	first_window: number;
	last_window: number;
	first_ns: string;
	last_ns: string;
	masks: number;
	reviewed_masks: number;
	proposed_masks: number;
	span_windows: number;
	has_span: boolean;
};

export type WorkspaceDetail = {
	plans: PlanProgress[];
	row: WorkspaceRow;
	restricted: boolean;
	objects: ObjectSpan[];
	journal: { revision: number; author: string; operation: string; created_utc: string }[];
	journal_error?: string;
	workflow_history: WorkflowItem[];
	layers: string[];
	states: string[];
	tbd_reasons: string[];
};

export type WorkspaceFilter = {
	kind?: string;
	q?: string;
	capture?: string;
	run?: string;
	cohort?: string;
	role?: string;
	eligible?: string;
	stage?: string;
	assignee?: string;
	flag?: string;
	limit?: number;
	offset?: number;
};

/** The query string for a filter, leaving out what is unset. */
export function workspaceQuery(filter: WorkspaceFilter): string {
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(filter)) {
		if (value !== undefined && value !== null && value !== '') params.set(key, String(value));
	}
	return params.toString();
}

/** A count in words: "42 of 80 masks reviewed", or its total unknown. */
export function countText(count: Count): string {
	const done = count.done.toLocaleString('en-GB');
	if (count.total === null) return `${done} ${count.unit}: ${count.name} (total unknown)`;
	return `${done} of ${count.total.toLocaleString('en-GB')} ${count.unit}: ${count.name}`;
}

/** A fraction for a bar, or null when the total is unknown or zero. */
export function countFraction(count: Count): number | null {
	if (count.total === null || count.total === 0) return null;
	return Math.min(1, count.done / count.total);
}

const STAGES: Record<string, string> = {
	unlabelled: 'Unlabelled',
	labelling: 'Labelling',
	in_review: 'In review',
	ready_to_freeze: 'Ready to freeze',
	frozen: 'Frozen',
	unavailable: 'Unavailable',
	protected: 'Protected (counts only)'
};

export function stageText(stage: string): string {
	return STAGES[stage] ?? stage;
}

const FLAGS: Record<string, string> = {
	blocked: 'Blocked',
	tbd: 'TBD',
	ready_for_review: 'Ready for review',
	ready_for_freeze: 'Ready to freeze',
	missing_files: 'Missing files',
	unavailable: 'Unavailable',
	stale_revision: 'Stale revision'
};

export function flagText(flag: string): string {
	return FLAGS[flag] ?? flag;
}

const WHERE: Record<string, string> = {
	browser: 'Here, in the browser',
	native: 'macOS annotation window',
	cli: 'Command line',
	unsupported: 'Not available yet'
};

export function whereText(where: string): string {
	return WHERE[where] ?? where;
}

const TBD: Record<string, string> = {
	unassigned_purpose: 'Purpose unassigned',
	missing_reference: 'Missing reference',
	undecided_criterion: 'Criterion undecided',
	unavailable_source: 'Source unavailable',
	missing_required_case: 'Required case missing',
	other: 'Other (see note)'
};

export function tbdText(reason: string): string {
	return TBD[reason] ?? reason;
}

const COHORTS: Record<string, string> = {
	development: 'Development',
	validation: 'Validation',
	qualification: 'Blind qualification',
	unassigned: 'Unassigned (TBD)',
	conflicting: 'Conflicting'
};

export function cohortText(cohort: string): string {
	return COHORTS[cohort] ?? cohort;
}

/** Split and selection roles in words, kept apart from the cohort. */
export function roleText(role: string): string {
	const [source, name] = role.split(':');
	const what = name === 'held_out' ? 'held out' : name;
	return source === 'split' ? `split: ${what}` : `selected for ${what}`;
}

/** Whether a row's evaluation verdict is a usable one: never true for TBD. */
export function evaluationText(row: WorkspaceRow): string {
	const ev = row.evaluation;
	if (!ev) return 'Unknown';
	const what = ev.held_out_score ? 'Held-out score' : 'Development evaluation';
	return ev.allowed ? `${what}: allowed` : `${what}: refused`;
}

/** The first eight hex digits of a digest. */
export function shortDigest(digest?: string): string {
	return (digest ?? '').replace(/^sha256:/, '').slice(0, 8);
}

const NS_PER_MS = 1_000_000n;

/** An ISO time from a nanosecond string, exact to the millisecond. */
export function nsToISO(ns: string | undefined): string {
	if (!ns || ns === '0') return '';
	try {
		const value = BigInt(ns);
		return new Date(Number(value / NS_PER_MS)).toISOString();
	} catch {
		return '';
	}
}

/** The latest workflow item for a layer of a row, if any. */
export function workflowFor(row: WorkspaceRow, layer: string): WorkflowItem | undefined {
	return row.workflow.find((item) => item.layer === layer);
}

/** A stage's place in the overview's order. */
export const STAGE_ORDER = [
	'in_review',
	'ready_to_freeze',
	'labelling',
	'unlabelled',
	'frozen',
	'unavailable',
	'protected'
];

/** An API failure, with the server's code and body when it sent one. */
export class WorkspaceError extends Error {
	constructor(
		message: string,
		readonly status: number,
		readonly body: Record<string, unknown> = {}
	) {
		super(message);
	}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(path, init);
	const body = await response.json().catch(() => ({}));
	if (!response.ok) {
		throw new WorkspaceError(
			(body as { error?: string }).error || `Request failed (${response.status})`,
			response.status,
			body as Record<string, unknown>
		);
	}
	return body as T;
}

export function loadWorkspace(filter: WorkspaceFilter): Promise<WorkspaceResponse> {
	return request(`/api/lidar/workspace?${workspaceQuery(filter)}`);
}

export function loadItem(key: string): Promise<WorkspaceDetail> {
	return request(`/api/lidar/workspace/item?${new URLSearchParams({ key })}`);
}

/** The headers every write carries: who the operator says they are. */
function writeHeaders(actor: string): HeadersInit {
	return { 'Content-Type': 'application/json', 'X-Velocity-Actor': actor };
}

/**
 * Save a workflow item against the revision the page loaded. The server
 * refuses with 409 when someone saved first; the page then reloads rather
 * than overwrite them.
 */
export function saveWorkflow(
	item: Partial<WorkflowItem> & { item_key: string; layer: string; state: string },
	baseRevision: number,
	actor: string
): Promise<WorkflowItem> {
	const body = {
		item_key: item.item_key,
		layer: item.layer,
		state: item.state,
		assignee: item.assignee ?? '',
		reviewer: item.reviewer ?? '',
		blocker: item.blocker ?? '',
		tbd_reason: item.state === 'tbd' ? (item.tbd_reason ?? '') : '',
		note: item.note ?? '',
		source_revision: item.source_revision ?? '',
		updated_by: actor,
		base_revision: baseRevision
	};
	return request('/api/lidar/workspace/workflow', {
		method: 'PUT',
		headers: writeHeaders(actor),
		body: JSON.stringify(body)
	});
}

/** Review, reject or reopen one object against the revision on screen. */
export function reviewObject(
	row: WorkspaceRow,
	objectID: string,
	status: 'reviewed' | 'rejected' | 'proposed',
	reviewer: string
): Promise<{ revision: number; state: string }> {
	if (!row.label_set) return Promise.reject(new Error('This item has no label set.'));
	return request('/api/lidar/annotations/objects/review', {
		method: 'POST',
		headers: writeHeaders(reviewer),
		body: JSON.stringify({
			label_set_id: row.label_set.id,
			base_revision: row.label_set.revision,
			base_state: row.label_set.state_digest,
			object_id: objectID,
			status,
			reviewer
		})
	});
}

/**
 * A one-pack freeze draft: every object of each of the row's packs in one
 * partition of the role, over every sample. The server's preview says what
 * stops it; the freeze is the canonical backend's, never decided here.
 */
export function freezeDraft(
	row: WorkspaceRow,
	objects: ObjectSpan[],
	role: 'tuning' | 'held_out',
	samples: number
) {
	const ids = objects.filter((o) => o.status !== 'rejected').map((o) => o.object_id);
	return {
		schema: 'velocity.report/annotation-split-draft',
		schema_version: 1,
		packs: row.packs.map((pack) => ({
			dir: pack.handle,
			splits: [{ name: role, role, object_ids: ids }],
			episodes: [
				{
					episode_id: `${role}-all`,
					split: role,
					object_ids: ids,
					frame_intervals: [{ first_sample: 0, last_sample: Math.max(0, samples - 1) }]
				}
			]
		}))
	};
}

export type FreezePreview = {
	membership_problems: string[];
	physical_problems: string[];
	facet_problems?: string[];
	refusal?: string;
	would_freeze: boolean;
	split_digest?: string;
};

export function previewFreeze(draft: unknown): Promise<FreezePreview> {
	return request('/api/lidar/annotations/split/preview', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ draft })
	});
}

export function freezeSplit(
	draft: unknown,
	author: string,
	output: string
): Promise<{ name: string; split_digest: string; revision: number }> {
	return request('/api/lidar/annotations/split/freeze', {
		method: 'POST',
		headers: writeHeaders(author),
		body: JSON.stringify({ draft, author, output })
	});
}

/** Every problem a preview lists, in the order the server gave them. */
export function previewProblems(preview: FreezePreview): string[] {
	return [
		...preview.membership_problems,
		...preview.physical_problems,
		...(preview.facet_problems ?? []),
		...(preview.refusal ? [preview.refusal] : [])
	];
}

const STEP_STATUS: Record<string, { mark: string; text: string }> = {
	done: { mark: '✓', text: 'Done' },
	in_progress: { mark: '●', text: 'In progress' },
	not_started: { mark: '○', text: 'Not started' },
	blocked: { mark: '!', text: 'Blocked' },
	untracked: { mark: '?', text: 'Not tracked' }
};

/** A step status as a mark and a word. */
export function stepStatus(status: string): { mark: string; text: string } {
	return STEP_STATUS[status] ?? { mark: '?', text: status };
}

/** "1 of 3 steps done", from the stores. */
export function planSummary(plan: PlanProgress): string {
	return `${plan.steps_done} of ${plan.progress.length} steps done`;
}

/** The first outstanding task of a row that is not blocked, else the first. */
export function nextTask(row: WorkspaceRow): Task | undefined {
	return row.tasks.find((t) => !t.blocked) ?? row.tasks[0];
}

/** Bind a plan to a row (or unbind it with an empty key) against the binding loaded. */
export function bindPlan(
	planID: string,
	itemKey: string,
	baseRevision: number,
	actor: string
): Promise<PlanBinding> {
	return request('/api/lidar/workspace/plan-binding', {
		method: 'PUT',
		headers: writeHeaders(actor),
		body: JSON.stringify({
			plan_id: planID,
			item_key: itemKey,
			note: '',
			bound_by: actor,
			base_revision: baseRevision
		})
	});
}

export type Handoff =
	| { kind: 'none'; reason: string }
	| {
			kind: 'pack';
			packs: { handle: string; path: string; vrlog?: string; run: string }[];
			shared: boolean;
	  };

/**
 * What to open in VelocityVisualiser for a row: its packs' directories (any
 * of a label set's views shows the same labels), or why there is none yet.
 */
export function handoff(row: WorkspaceRow): Handoff {
	if (row.protected)
		return { kind: 'none', reason: 'Blind qualification data is not opened from here.' };
	const packs = row.packs
		.filter((p) => p.abs_dir && !p.error)
		.map((p) => ({
			handle: p.handle,
			path: p.abs_dir as string,
			vrlog: p.vrlog || undefined,
			run: p.run
		}));
	if (packs.length)
		return { kind: 'pack', packs, shared: packs.length > 1 && Boolean(row.label_set) };
	if (row.segments.some((s) => s.pack_missing))
		return {
			kind: 'none',
			reason: 'The pack the database names is not on disk: queue it again on the Segments page.'
		};
	if (row.segments.length)
		return {
			kind: 'none',
			reason: "No pack yet: queue the clip's pack on the Segments page, then refresh."
		};
	return { kind: 'none', reason: 'No readable pack.' };
}
