<script lang="ts">
	/**
	 * Labelling — what exists, how far each layer of labelling and review has
	 * got, what is outstanding and why, and what each item may be used for.
	 *
	 * Everything shown is the server's: progress is read from the label sets
	 * and physical-reference documents, workflow notes from the database, and
	 * eligibility from the server's guard. Hiding something here protects
	 * nothing; the server refuses what is not allowed. Blind qualification
	 * items arrive as counts only, and nothing on this page fetches their
	 * samples, labels or previews.
	 */
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import {
		cohortText,
		countFraction,
		countText,
		evaluationText,
		flagText,
		freezeDraft,
		freezeSplit,
		loadItem,
		loadWorkspace,
		nsToISO,
		previewFreeze,
		previewProblems,
		reviewObject,
		roleText,
		saveWorkflow,
		shortDigest,
		stageText,
		STAGE_ORDER,
		tbdText,
		whereText,
		workflowFor,
		WorkspaceError,
		type FreezePreview,
		type LayerProgress,
		type WorkspaceDetail,
		type WorkspaceFilter,
		type WorkspaceResponse,
		type WorkspaceRow
	} from '#lib/labellingWorkspace.js';
	import { tracksQuery } from '#lib/lidarLinks.js';
	import { onMount, tick } from 'svelte';
	import { Button } from 'svelte-ux';

	const PAGE = 50;
	const ACTOR_KEY = 'velocity.labelling.actor';

	let data: WorkspaceResponse | null = null;
	let loadError = '';
	let busy = false;
	let filter: WorkspaceFilter = { limit: PAGE, offset: 0 };
	let detail: WorkspaceDetail | null = null;
	let detailError = '';
	let actor = '';
	let message = '';
	let actionError = '';

	// Workflow editor state, per layer, for the open item.
	let editLayer = 'spans';
	let editState = 'open';
	let editAssignee = '';
	let editReviewer = '';
	let editBlocker = '';
	let editTBD = '';
	let editNote = '';

	// Freeze form.
	let freezeRole: 'tuning' | 'held_out' = 'tuning';
	let freezeName = '';
	let preview: FreezePreview | null = null;

	function rememberActor() {
		try {
			localStorage.setItem(ACTOR_KEY, actor);
		} catch {
			// A private window keeps no name; it is asked for again.
		}
	}

	async function load() {
		busy = true;
		loadError = '';
		try {
			data = await loadWorkspace(filter);
		} catch (cause) {
			data = null;
			loadError =
				cause instanceof Error
					? `The inventory could not be read: ${cause.message}. Nothing below is a count of zero.`
					: 'The inventory could not be read.';
		} finally {
			busy = false;
		}
	}

	// reload keeps the message a write just set; opening another item clears it.
	async function openItem(key: string, reload = false) {
		detailError = '';
		if (!reload) {
			message = '';
			actionError = '';
		}
		preview = null;
		try {
			detail = await loadItem(key);
			pickLayer(editLayer);
			freezeName = `${(detail.row.label_set?.name || detail.row.key).replace(/[^A-Za-z0-9._-]/g, '-')}-r1.json`;
			await tick();
			document.getElementById('item-detail')?.scrollIntoView({ block: 'start' });
		} catch (cause) {
			detail = null;
			detailError = cause instanceof Error ? cause.message : 'The item could not be read.';
		}
		// The open item is kept in the address, so a reload reopens it.
		const url = new URL(page.url.href);
		url.searchParams.set('item', key);
		// The same page with a query added; the rule accepts only a bare resolve().
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		replaceState(url, {});
	}

	function closeItem() {
		detail = null;
		const url = new URL(page.url.href);
		url.searchParams.delete('item');
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		replaceState(url, {});
	}

	function pickLayer(layer: string) {
		editLayer = layer;
		const current = detail ? workflowFor(detail.row, layer) : undefined;
		editState = current?.state ?? 'open';
		editAssignee = current?.assignee ?? '';
		editReviewer = current?.reviewer ?? '';
		editBlocker = current?.blocker ?? '';
		editTBD = current?.tbd_reason ?? '';
		editNote = current?.note ?? '';
	}

	function revisionOf(row: WorkspaceRow): string {
		if (row.label_set?.state_digest)
			return `label-set:${row.label_set.revision}:${row.label_set.state_digest}`;
		return row.packs[0]?.pack_digest ? `pack:${row.packs[0].pack_digest}` : '';
	}

	function needActor(): boolean {
		if (actor.trim()) return true;
		actionError = 'Enter your name first: every write records who made it.';
		return false;
	}

	async function saveLayer() {
		if (!detail || !needActor()) return;
		actionError = '';
		const current = workflowFor(detail.row, editLayer);
		try {
			await saveWorkflow(
				{
					item_key: detail.row.key,
					layer: editLayer,
					state: editState,
					assignee: editAssignee,
					reviewer: editReviewer,
					blocker: editBlocker,
					tbd_reason: editTBD,
					note: editNote,
					source_revision: revisionOf(detail.row)
				},
				current?.revision ?? 0,
				actor
			);
			message = `Saved the ${editLayer} workflow item. It changes no review status.`;
			await Promise.all([openItem(detail.row.key, true), load()]);
		} catch (cause) {
			if (cause instanceof WorkspaceError && cause.status === 409) {
				actionError =
					'Someone saved this item after you opened it. It has been reloaded: check their change and save again.';
				await openItem(detail.row.key, true);
			} else {
				actionError = cause instanceof Error ? cause.message : 'Could not save.';
			}
		}
	}

	async function review(objectID: string, status: 'reviewed' | 'rejected' | 'proposed') {
		if (!detail || !needActor()) return;
		actionError = '';
		try {
			const out = await reviewObject(detail.row, objectID, status, actor);
			message = `${objectID} is ${status}: label set revision ${out.revision}. Its masks keep their own review.`;
			await Promise.all([openItem(detail.row.key, true), load()]);
		} catch (cause) {
			if (cause instanceof WorkspaceError && cause.status === 409) {
				actionError =
					'The label set changed after you opened it (another reviewer, or the macOS tool). Reloaded: review again.';
				await openItem(detail.row.key, true);
			} else {
				actionError = cause instanceof Error ? cause.message : 'Could not save the review.';
			}
		}
	}

	function samplesOf(row: WorkspaceRow): number {
		return Math.max(0, ...row.packs.map((pack) => pack.samples ?? 0));
	}

	async function runPreview() {
		if (!detail) return;
		actionError = '';
		try {
			preview = await previewFreeze(
				freezeDraft(detail.row, detail.objects, freezeRole, samplesOf(detail.row))
			);
		} catch (cause) {
			preview = null;
			actionError = cause instanceof Error ? cause.message : 'The preview failed.';
		}
	}

	async function runFreeze() {
		if (!detail || !preview?.would_freeze || !needActor()) return;
		actionError = '';
		try {
			const out = await freezeSplit(
				freezeDraft(detail.row, detail.objects, freezeRole, samplesOf(detail.row)),
				actor,
				freezeName
			);
			message = `Froze ${out.name}: ${shortDigest(out.split_digest)}, revision ${out.revision}.`;
			preview = null;
			await Promise.all([openItem(detail.row.key, true), load()]);
		} catch (cause) {
			actionError = cause instanceof Error ? cause.message : 'The freeze failed.';
		}
	}

	function applyFilter() {
		filter = { ...filter, offset: 0 };
		void load();
	}

	function pageBy(step: number) {
		filter = { ...filter, offset: Math.max(0, (filter.offset ?? 0) + step * PAGE) };
		void load();
	}

	function layers(row: WorkspaceRow): LayerProgress[] {
		const out = [...(row.progress?.layers ?? [])];
		// Physical references are per pack: each card names its pack.
		for (const pack of row.packs)
			for (const l of pack.physical?.layers ?? [])
				out.push({ ...l, title: `${l.title} — ${pack.handle}` });
		return out;
	}

	function previewLink(row: WorkspaceRow, atNs: string): string | null {
		// No tracker preview of held-out or protected data: blind review.
		if (row.protected || row.roles.some((role) => role.endsWith(':held_out'))) return null;
		const run = row.segments.find((s) => s.run_id)?.run_id;
		if (!run) return null;
		return `${resolve('/lidar/tracks')}?${tracksQuery({ runId: run, atNs })}`;
	}

	$: stages = data
		? STAGE_ORDER.filter((s) => data?.totals.stages[s]).map((s) => [s, data!.totals.stages[s]])
		: [];
	$: unavailable = data ? Object.entries(data.sources).filter(([, state]) => !state.available) : [];

	onMount(async () => {
		try {
			actor = localStorage.getItem(ACTOR_KEY) ?? '';
		} catch {
			actor = '';
		}
		await load();
		const key = page.url.searchParams.get('item');
		if (key) await openItem(key);
	});
</script>

<svelte:head>
	<title>Labelling — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between gap-4">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Labelling</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Captures, segments, packs, label sets and frozen splits; each layer's progress with its
					own denominator, what is outstanding and why, and what each item may be used for. Labels
					are edited in the macOS annotation window; this page reviews objects, keeps workflow notes
					and freezes splits.
				</p>
			</div>
			<div class="flex items-end gap-2">
				<label class="text-surface-content/70 flex flex-col gap-1 text-xs font-medium"
					>Your name
					<input
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1 text-sm"
						bind:value={actor}
						on:change={rememberActor}
						placeholder="recorded with every write"
					/>
				</label>
				<Button variant="outline" on:click={load} disabled={busy}
					>{busy ? 'Loading...' : 'Refresh'}</Button
				>
			</div>
		</div>
	</div>

	<div class="flex-1 space-y-4 overflow-y-auto p-6">
		{#if loadError}
			<p role="alert" class="rounded bg-red-50 px-4 py-3 text-sm text-red-700">{loadError}</p>
		{/if}
		{#if !data && busy}
			<p class="text-surface-content/60 text-sm">Reading the label sets and splits…</p>
		{/if}

		{#if detailError}<p role="alert" class="rounded bg-red-50 px-4 py-3 text-sm text-red-700">
				{detailError}
			</p>{/if}

		{#if detail}
			{@const row = detail.row}
			<section
				id="item-detail"
				aria-label="Item detail"
				class="border-primary/40 bg-surface-100 space-y-4 rounded-lg border p-4 text-sm"
			>
				<div class="flex items-start justify-between">
					<div>
						<h2 class="text-surface-content text-lg font-semibold">{row.title}</h2>
						<p class="text-surface-content/60 font-mono text-xs">{row.key}</p>
					</div>
					<Button size="sm" on:click={closeItem}>Close</Button>
				</div>
				{#if message}<p role="status" class="rounded bg-green-50 px-3 py-2 text-green-900">
						{message}
					</p>{/if}
				{#if actionError}<p role="alert" class="rounded bg-red-50 px-3 py-2 text-red-700">
						{actionError}
					</p>{/if}
				{#if detail.restricted}
					<p class="rounded bg-amber-50 p-3 text-amber-950">
						Blind qualification data: shown as counts only. No samples, labels, objects or previews
						are loaded into this session, and review and evaluation are refused by the server while
						no storage-level access control protects it.
					</p>
				{/if}
				{#if row.error}<p class="text-red-700">Unavailable: {row.error}</p>{/if}

				<!-- Lineage. -->
				<div>
					<h3 class="font-medium">Lineage</h3>
					<ol class="mt-1 space-y-1">
						<li>
							<span class="text-surface-content/60">Capture:</span>
							{#each row.captures as c (c.basename + (c.sha256 ?? ''))}<span class="mr-3"
									>{c.basename}
									{c.sha256 ? shortDigest(c.sha256) : '(hash unverified)'}</span
								>{:else}none known{/each}
							· {cohortText(row.cohort)}{#if row.site_id}, site {row.site_id}{/if}
						</li>
						<li>
							<span class="text-surface-content/60">Selected segments:</span>
							{#each row.segments as s (s.segment_id)}
								<span class="mr-3"
									>{s.segment_id} ({s.role}, {s.finder}{#if s.selector}/{s.selector}{/if}{#if s.window_start_ns},
										{nsToISO(s.window_start_ns)}{/if}) → clip {s.clip_id || '—'}{#if s.pack_missing}
										<span class="text-red-700">pack missing on disk</span>{/if}</span
								>
							{:else}none recorded{/each}
						</li>
						<li>
							<span class="text-surface-content/60">Runs and packs:</span>
							{#each row.packs as p (p.handle + p.pack_digest)}
								<span class="mr-3 font-mono text-xs">{p.handle} ({shortDigest(p.pack_digest)})</span
								>
							{/each}
						</li>
						<li>
							<span class="text-surface-content/60">Label set:</span>
							{#if row.label_set}{row.label_set.name || row.label_set.id}, head revision {row
									.label_set.revision} ({shortDigest(row.label_set.state_digest)}){:else}none yet{/if}
						</li>
						<li>
							<span class="text-surface-content/60">Frozen splits:</span>
							{#each row.splits as s (s.name)}
								<span class="mr-3"
									>{s.name} ({shortDigest(s.split_digest)}, {s.roles.join('/')}) pins revision {s.pinned_revision}{#if s.head_moved}
										— <span class="text-amber-800">historical: the head is {s.head_revision}</span
										>{/if}</span
								>
							{:else}none{/each}
						</li>
						<li>
							<span class="text-surface-content/60">Evaluation use:</span>
							{evaluationText(row)}{#if row.evaluation?.reasons.length}: {row.evaluation.reasons.join(
									'; '
								)}{/if}
						</li>
					</ol>
				</div>

				<!-- Progress by layer. -->
				<div>
					<h3 class="font-medium">Progress by layer</h3>
					<div class="mt-1 grid gap-2 md:grid-cols-2">
						{#each layers(row) as layer, i (layer.layer + i)}
							<div class="border-surface-content/10 rounded border p-2">
								<p class="font-medium">
									{layer.title}
									<span class="text-surface-content/60 text-xs">· {whereText(layer.where)}</span>
								</p>
								{#each layer.counts as count (count.name)}<p>{countText(count)}</p>{/each}
								{#if layer.note}<p class="text-surface-content/60 text-xs">{layer.note}</p>{/if}
							</div>
						{/each}
					</div>
				</div>

				<!-- Outstanding work. -->
				<div>
					<h3 class="font-medium">Outstanding</h3>
					<ul class="mt-1 list-disc space-y-1 pl-5">
						{#each row.tasks as task, i (task.layer + i)}
							<li>
								{task.action} — {task.reason}{task.depends_on
									? ` (${task.blocked ? 'blocked by' : 'after'} ${task.depends_on})`
									: ''} · {whereText(task.where)}{task.tbd ? ` · TBD: ${tbdText(task.tbd)}` : ''}
								{#if task.handoff}<span class="text-surface-content/70 block font-mono text-xs"
										>{task.handoff}</span
									>{/if}
							</li>
						{:else}<li>Nothing outstanding.</li>{/each}
					</ul>
				</div>

				{#if !detail.restricted && detail.objects.length}
					<!-- Object review: spans, class and identity. -->
					<div>
						<h3 class="font-medium">Objects: spans, class and identity</h3>
						<p class="text-surface-content/60 text-xs">
							Reviewing an object reviews its span, class and identity at label-set revision {row
								.label_set?.revision}. It does not review its masks, which are reviewed in the macOS
							annotation window, nor any physical reference.
						</p>
						<table class="mt-1 w-full text-left text-xs">
							<thead
								><tr class="text-surface-content/70"
									><th class="p-1">Object</th><th class="p-1">Class</th><th class="p-1">Status</th
									><th class="p-1">Span</th><th class="p-1">Masks (reviewed / proposed)</th><th
										class="p-1">Review</th
									></tr
								></thead
							>
							<tbody>
								{#each detail.objects as o (o.object_id)}
									{@const link = previewLink(row, o.first_ns)}
									<tr class="border-surface-content/10 border-t">
										<td class="p-1 font-mono">{o.object_id}</td>
										<td class="p-1">{o.class || 'unstated'}</td>
										<td class="p-1">{o.status}</td>
										<td class="p-1"
											>{o.has_span
												? `windows ${o.first_window}–${o.last_window}` +
													(o.first_ns !== '0' ? ` (${nsToISO(o.first_ns)})` : '')
												: 'no mask yet'}</td
										>
										<td class="p-1"
											>{o.masks} ({o.reviewed_masks} / {o.proposed_masks}) of {o.span_windows} frames</td
										>
										<td class="space-x-2 p-1 whitespace-nowrap">
											{#if o.status !== 'reviewed'}<button
													class="text-primary hover:underline"
													on:click={() => review(o.object_id, 'reviewed')}>Review</button
												>{/if}
											{#if o.status !== 'rejected'}<button
													class="text-primary hover:underline"
													on:click={() => review(o.object_id, 'rejected')}>Reject</button
												>{/if}
											{#if o.status !== 'proposed'}<button
													class="text-primary hover:underline"
													on:click={() => review(o.object_id, 'proposed')}>Reopen</button
												>{/if}
											<!-- eslint-disable svelte/no-navigation-without-resolve -->
											{#if link}<a class="text-primary hover:underline" href={link}>Scene</a>{/if}
											<!-- eslint-enable svelte/no-navigation-without-resolve -->
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
						<p class="text-surface-content/60 mt-1 text-xs">
							Scene opens the run in the shared scene player at the span's start; it is not offered
							for held-out or protected items.
						</p>
					</div>
				{/if}

				<!-- Workflow notes: never annotation truth. -->
				<div>
					<h3 class="font-medium">Workflow</h3>
					<p class="text-surface-content/60 text-xs">
						Assignments, blockers, TBD reasons and notes, kept per layer. Marking a layer done is a
						statement, not a review: it changes no annotation status.
					</p>
					<div class="mt-2 flex flex-wrap items-end gap-2">
						<label class="flex flex-col gap-1 text-xs"
							>Layer
							<select
								class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
								value={editLayer}
								on:change={(e) => pickLayer(e.currentTarget.value)}
							>
								{#each detail.layers as l (l)}<option value={l}>{l.replace('_', ' ')}</option
									>{/each}
							</select>
						</label>
						<label class="flex flex-col gap-1 text-xs"
							>State
							<select
								class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
								bind:value={editState}
							>
								{#each detail.states as s (s)}<option value={s}>{s.replace('_', ' ')}</option
									>{/each}
							</select>
						</label>
						<label class="flex flex-col gap-1 text-xs"
							>Assignee<input
								class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
								bind:value={editAssignee}
							/></label
						>
						<label class="flex flex-col gap-1 text-xs"
							>Reviewer<input
								class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
								bind:value={editReviewer}
							/></label
						>
						{#if editState === 'blocked'}
							<label class="flex flex-col gap-1 text-xs"
								>Blocked by<input
									class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
									bind:value={editBlocker}
								/></label
							>
						{/if}
						{#if editState === 'tbd'}
							<label class="flex flex-col gap-1 text-xs"
								>TBD because
								<select
									class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
									bind:value={editTBD}
								>
									<option value="">Choose a reason</option>
									{#each detail.tbd_reasons as r (r)}<option value={r}>{tbdText(r)}</option>{/each}
								</select>
							</label>
						{/if}
						<label class="flex min-w-64 flex-1 flex-col gap-1 text-xs"
							>Note<input
								class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
								bind:value={editNote}
							/></label
						>
						<Button size="sm" variant="fill" color="primary" on:click={saveLayer}>Save</Button>
					</div>
					{#if row.workflow.length}
						<ul class="mt-2 space-y-1 text-xs">
							{#each row.workflow as item (item.layer)}
								<li>
									<span class="font-medium">{item.layer.replace('_', ' ')}</span>: {item.state.replace(
										'_',
										' '
									)}{#if item.tbd_reason}
										({tbdText(item.tbd_reason)}){/if}{#if item.assignee}
										· {item.assignee}{/if}{#if item.reviewer}
										· reviewer {item.reviewer}{/if}{#if item.blocker}
										· blocked by {item.blocker}{/if}{#if item.note}
										· {item.note}{/if} · revision {item.revision} by {item.updated_by}, {nsToISO(
										item.updated_at_ns
									)}
									{#if item.source_revision && item.source_revision !== revisionOf(row)}<span
											class="text-amber-800"
										>
											(written against an older revision)</span
										>{/if}
								</li>
							{/each}
						</ul>
					{/if}
				</div>

				{#if !detail.restricted && row.label_set}
					<!-- Freeze, through the canonical backend. -->
					<div>
						<h3 class="font-medium">Freeze</h3>
						<p class="text-surface-content/60 text-xs">
							The preview runs every check the freeze runs and lists every blocker. A window
							selected for tuning can never be held out; a freeze pins the label-set revision, and a
							later edit makes that revision historical without changing the split.
						</p>
						<div class="mt-2 flex flex-wrap items-end gap-2">
							<label class="flex flex-col gap-1 text-xs"
								>Role
								<select
									class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
									bind:value={freezeRole}
									on:change={() => (preview = null)}
								>
									<option value="tuning">Tuning</option><option value="held_out">Held out</option>
								</select>
							</label>
							<label class="flex flex-col gap-1 text-xs"
								>Split file<input
									class="border-surface-300 bg-surface-100 rounded border px-2 py-1 font-mono"
									bind:value={freezeName}
								/></label
							>
							<Button size="sm" on:click={runPreview}>Preview</Button>
							<Button
								size="sm"
								variant="fill"
								color="primary"
								disabled={!preview?.would_freeze}
								on:click={runFreeze}>Freeze</Button
							>
						</div>
						{#if preview}
							{#if preview.would_freeze}
								<p class="mt-2 text-green-900">
									Would freeze as {shortDigest(preview.split_digest)}.
								</p>
							{:else}
								<ul class="mt-2 list-disc space-y-1 pl-5 text-red-700">
									{#each previewProblems(preview) as problem, i (i)}<li>{problem}</li>{/each}
								</ul>
							{/if}
						{/if}
					</div>
				{/if}

				{#if !detail.restricted}
					<div>
						<h3 class="font-medium">Native work</h3>
						<p class="text-surface-content/70">
							Masks, mask review, propagation, physical body and keyframe authoring and following
							records are made in VelocityVisualiser's annotation window (Annotation pane → open
							pack):
						</p>
						<ul class="font-mono text-xs">
							{#each row.packs as p (p.handle)}<li>{p.handle}</li>{/each}
						</ul>
						<p class="text-surface-content/60 text-xs">
							Those operations are not available in the browser. Save there, then Refresh here.
						</p>
					</div>
				{/if}

				<details>
					<summary class="font-medium">Revision history</summary>
					{#if detail.journal_error}<p class="text-red-700">{detail.journal_error}</p>{/if}
					<ul class="mt-1 space-y-1 text-xs">
						{#each [...detail.journal].reverse() as entry (entry.revision)}
							<li>
								Label set revision {entry.revision}: {entry.operation} by {entry.author}{#if entry.created_utc},
									{entry.created_utc}{/if}
							</li>
						{:else}<li>No local journal (a published copy carries none).</li>{/each}
						{#each detail.workflow_history as item (item.layer + item.revision)}
							<li>
								Workflow {item.layer.replace('_', ' ')} revision {item.revision}: {item.state.replace(
									'_',
									' '
								)} by {item.updated_by},
								{nsToISO(item.updated_at_ns)}
							</li>
						{/each}
					</ul>
				</details>
			</section>
		{/if}

		{#if data}
			<!-- Protection: what the guard enforces and what is still missing. -->
			<section
				aria-label="Held-out protection"
				class="rounded-lg border p-4 text-sm {data.eligibility.protection_complete
					? 'border-green-300 bg-green-50 text-green-950'
					: 'border-amber-300 bg-amber-50 text-amber-950'}"
			>
				<h2 class="text-base font-semibold">
					{data.eligibility.protection_complete
						? 'Held-out protection is complete'
						: 'Held-out protection is incomplete'}
				</h2>
				<p class="mt-1">
					Cohort manifest: {data.eligibility.manifest.state}{#if data.eligibility.manifest.digest}
						({shortDigest(data.eligibility.manifest.digest)}, revision {data.eligibility.manifest
							.revision}){/if}. Frozen splits read: {data.eligibility.registry.splits}, holding out {data
						.eligibility.registry.held_out_spans} stretches. Policy {shortDigest(
						data.eligibility.policy_digest
					)}.
				</p>
				{#if Object.keys(data.eligibility.manifest.cohorts).length}
					<p class="mt-1">
						{#each Object.entries(data.eligibility.manifest.cohorts) as [cohort, count] (cohort)}
							<span class="mr-3"
								>{cohortText(cohort)}: {count.sites} sites, {count.captures} captures</span
							>
						{/each}
					</p>
				{/if}
				<ul class="mt-2 list-disc space-y-1 pl-5">
					{#each data.eligibility.gaps as gap (gap)}<li>{gap}</li>{/each}
				</ul>
				<p class="mt-2">
					{#each data.eligibility.purposes as purpose (purpose.purpose)}
						<span class="mr-3 inline-block"
							>{purpose.purpose.replace(/_/g, ' ')}: {purpose.available
								? 'available'
								: `unavailable (${purpose.reason})`}</span
						>
					{/each}
				</p>
				<p class="mt-2 text-xs opacity-80">
					The server decides eligibility at every guarded entry point before a sample loads. This
					page only reports it.
				</p>
			</section>

			{#each unavailable as [name, state] (name)}
				<p role="alert" class="rounded bg-red-50 px-4 py-2 text-sm text-red-700">
					{name.replace(/_/g, ' ')} unavailable: {state.error}. Rows from it are missing, not zero.
				</p>
			{/each}

			<!-- Overview: each layer on its own, label sets counted once. -->
			<section aria-label="Progress by layer" class="space-y-2">
				<h2 class="text-surface-content text-lg font-semibold">Progress by layer</h2>
				<p class="text-surface-content/60 text-sm">
					{data.totals.label_sets} label sets (each once, however many runs view it) · {data.totals
						.packs} packs · {data.totals.captures} captures · {data.totals.segments} selected segments
					· {data.totals.runs} runs · {data.totals.splits} frozen splits · {data.totals
						.outstanding_tasks} outstanding tasks ({data.totals.blocked_tasks} blocked, {data.totals
						.tbd_tasks} TBD){#if data.totals.unreadable}
						· <span class="text-red-700">{data.totals.unreadable} label sets unreadable</span>{/if}
				</p>
				<div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
					{#each data.totals.layers as layer (layer.layer)}
						<div class="border-surface-content/10 bg-surface-100 rounded-lg border p-3 text-sm">
							<h3 class="font-medium">{layer.title}</h3>
							<p class="text-surface-content/60 text-xs">{whereText(layer.where)}</p>
							{#each layer.counts as count (count.name)}
								<div class="mt-2">
									<p>{countText(count)}</p>
									{#if countFraction(count) !== null}
										<div
											class="bg-surface-300 mt-1 h-1.5 rounded"
											role="img"
											aria-label={countText(count)}
										>
											<div
												class="bg-primary h-1.5 rounded"
												style="width: {(countFraction(count) ?? 0) * 100}%"
											></div>
										</div>
									{/if}
								</div>
							{/each}
							{#if layer.note}<p class="text-surface-content/60 mt-2 text-xs">{layer.note}</p>{/if}
						</div>
					{:else}
						<p class="text-surface-content/60 text-sm">
							No label set has been read yet, so no layer has a count.
						</p>
					{/each}
				</div>
				<p class="text-surface-content/60 text-xs">
					Physical references live in each pack, not in the label set, so they are shown per pack in
					an item's detail rather than summed across views.
				</p>
				{#if stages.length}
					<p class="text-sm">
						{#each stages as [stage, n] (stage)}<span class="mr-3"
								>{stageText(String(stage))}: {n}</span
							>{/each}
					</p>
				{/if}
			</section>

			<!-- Filters, applied by the server. -->
			<form
				class="flex flex-wrap items-end gap-3 text-sm"
				on:submit|preventDefault={applyFilter}
				aria-label="Filters"
			>
				<label class="flex flex-col gap-1"
					>Search
					<input
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.q}
						placeholder="capture, pack, label set"
					/>
				</label>
				<label class="flex flex-col gap-1"
					>Capture
					<input
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.capture}
						placeholder="name or sha256"
					/>
				</label>
				<label class="flex flex-col gap-1"
					>Run
					<select
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.run}
					>
						<option value="">Any</option>
						{#each data.facets.runs ?? [] as run (run)}<option value={run}>{run}</option>{/each}
					</select>
				</label>
				<label class="flex flex-col gap-1"
					>Kind
					<select
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.kind}
					>
						<option value="">Any</option><option value="label_set">Label set</option><option
							value="pack">Pack without labels</option
						><option value="segment">Segment without a pack</option>
					</select>
				</label>
				<label class="flex flex-col gap-1"
					>Stage
					<select
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.stage}
					>
						<option value="">Any</option>
						{#each STAGE_ORDER as stage (stage)}<option value={stage}>{stageText(stage)}</option
							>{/each}
					</select>
				</label>
				<label class="flex flex-col gap-1"
					>Cohort
					<select
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.cohort}
					>
						<option value="">Any</option>
						{#each ['development', 'validation', 'qualification', 'unassigned', 'conflicting'] as c (c)}<option
								value={c}>{cohortText(c)}</option
							>{/each}
					</select>
				</label>
				<label class="flex flex-col gap-1"
					>Split or selection role
					<select
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.role}
					>
						<option value="">Any</option><option value="tuning">Tuning</option><option
							value="held_out">Held out</option
						><option value="none">None</option>
					</select>
				</label>
				<label class="flex flex-col gap-1"
					>Evaluation
					<select
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.eligible}
					>
						<option value="">Any</option><option value="yes">Allowed</option><option value="no"
							>Refused or TBD</option
						>
					</select>
				</label>
				<label class="flex flex-col gap-1"
					>Flag
					<select
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.flag}
					>
						<option value="">Any</option>
						{#each ['blocked', 'tbd', 'ready_for_review', 'ready_for_freeze', 'missing_files', 'unavailable', 'stale_revision'] as f (f)}<option
								value={f}>{flagText(f)}</option
							>{/each}
					</select>
				</label>
				<label class="flex flex-col gap-1"
					>Assigned to
					<input
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={filter.assignee}
						placeholder="operator or reviewer"
					/>
				</label>
				<Button type="submit" variant="fill" color="primary">Apply</Button>
			</form>

			<!-- Inventory. -->
			<section
				aria-label="Inventory"
				class="border-surface-content/10 bg-surface-100 overflow-x-auto rounded-lg border"
			>
				<table class="w-full text-left text-sm">
					<thead
						><tr class="text-surface-content/70"
							><th class="p-3 font-medium">Item</th><th class="p-3 font-medium">Stage</th><th
								class="p-3 font-medium">Cohort / roles</th
							><th class="p-3 font-medium">Objects reviewed</th><th class="p-3 font-medium"
								>Masks reviewed</th
							><th class="p-3 font-medium">Evaluation</th><th class="p-3 font-medium">Flags</th></tr
						></thead
					>
					<tbody>
						{#each data.items as row (row.key)}
							{@const review = row.progress?.layers.find((l) => l.layer === 'object_review')}
							{@const masks = row.progress?.layers.find((l) => l.layer === 'mask_review')}
							<tr class="border-surface-content/10 border-t align-top">
								<td class="p-3">
									<button
										class="text-primary text-left hover:underline"
										on:click={() => openItem(row.key)}>{row.title}</button
									>
									<p class="text-surface-content/60 text-xs">
										{row.kind.replace('_', ' ')} · {row.packs.length} packs · {row.captures
											.map((c) => c.basename + (c.verified ? '' : ' (name only)'))
											.join(', ') || 'no capture known'}
									</p>
								</td>
								<td class="p-3">{stageText(row.stage)}</td>
								<td class="p-3"
									>{cohortText(row.cohort)}{#each row.roles as role (role)}<span
											class="text-surface-content/60 block text-xs">{roleText(role)}</span
										>{/each}</td
								>
								<td class="p-3"
									>{review ? countText(review.counts[0]) : row.error ? 'Unknown' : '—'}</td
								>
								<td class="p-3"
									>{masks ? countText(masks.counts[0]) : row.error ? 'Unknown' : '—'}</td
								>
								<td class="p-3">{evaluationText(row)}</td>
								<td class="p-3"
									>{#each row.flags as f (f)}<span
											class="bg-surface-200 mr-1 mb-1 inline-block rounded px-1.5 py-0.5 text-xs"
											>{flagText(f)}</span
										>{/each}</td
								>
							</tr>
						{/each}
					</tbody>
				</table>
				{#if !data.items.length}
					<p class="text-surface-content/60 p-4 text-sm">
						{data.total === 0 && !unavailable.length
							? 'Nothing matches. With no filter, this means no pack, label set or segment exists yet.'
							: 'Nothing matches these filters.'}
					</p>
				{/if}
				<div class="flex items-center justify-between p-3 text-xs">
					<span
						>{data.total === 0 ? 0 : data.offset + 1}–{Math.min(
							data.offset + data.limit,
							data.total
						)}
						of {data.total}</span
					>
					<span class="space-x-2">
						<Button size="sm" disabled={data.offset === 0} on:click={() => pageBy(-1)}
							>Previous</Button
						>
						<Button
							size="sm"
							disabled={data.offset + data.limit >= data.total}
							on:click={() => pageBy(1)}>Next</Button
						>
					</span>
				</div>
			</section>

			<!-- Work queue. -->
			<section aria-label="Work queue" class="space-y-2">
				<h2 class="text-surface-content text-lg font-semibold">Work queue</h2>
				<ul class="space-y-2 text-sm">
					{#each data.queue as task, i (task.key + task.layer + i)}
						<li class="border-surface-content/10 bg-surface-100 rounded border p-3">
							<p>
								<span class="font-medium">{task.action}</span>
								<button
									class="text-primary ml-2 text-xs hover:underline"
									on:click={() => openItem(task.key)}>{task.title}</button
								>
							</p>
							<p class="text-surface-content/70">Why: {task.reason}</p>
							{#if task.depends_on}<p class="text-surface-content/70">
									{task.blocked ? 'Blocked by' : 'Depends on'}: {task.depends_on}
								</p>{/if}
							<p class="text-surface-content/60 text-xs">
								{task.layer.replace('_', ' ')} · needs {task.needs.join(', ')} · {whereText(
									task.where
								)}{#if task.tbd}
									· TBD: {tbdText(task.tbd)}{/if}{#if task.plan}
									· {task.plan}{/if}{#if task.source_revision}
									· at {task.source_revision.length > 40
										? task.source_revision.slice(0, 40) + '…'
										: task.source_revision}{/if}
							</p>
							{#if task.handoff}<p class="text-surface-content/70 font-mono text-xs">
									{task.handoff}
								</p>{/if}
						</li>
					{:else}
						<li class="text-surface-content/60">Nothing outstanding for these filters.</li>
					{/each}
				</ul>
			</section>

			<!-- Plans: planned work, not evidence. -->
			<section aria-label="Labelling plans" class="space-y-2 text-sm">
				<h2 class="text-surface-content text-lg font-semibold">Labelling plans</h2>
				<p class="text-surface-content/60">
					What the specs say is to be done. A plan is never evidence that labelling has occurred;
					progress above comes from the stores alone.
				</p>
				{#if !data.plans.available}<p class="text-surface-content/60">{data.plans.error}</p>{/if}
				{#each data.plans.plans as plan (plan.id)}
					<details class="border-surface-content/10 bg-surface-100 rounded border p-3">
						<summary
							><span class="font-mono">{plan.id}</span> — {plan.title} ({plan.status}){#if plan.depends_on.length};
								after {plan.depends_on.join(', ')}{/if}</summary
						>
						<ol class="mt-2 list-decimal space-y-1 pl-5">
							{#each plan.steps as step (step.id)}<li>
									<span class="font-medium">{step.id}</span> ({step.kind}{#if step.human_estimate}, {step.human_estimate}
										of a person's time{/if}): {step.note}
								</li>{/each}
						</ol>
						<p class="text-surface-content/60 mt-1 text-xs">{plan.path}</p>
					</details>
				{/each}
			</section>
		{/if}

		<p class="text-surface-content/60 text-xs">
			Definitions, sources of truth and enforcement boundaries are in the labelling workspace guide
			(docs/lidar/operations/labelling-workspace.md).
		</p>
	</div>
</main>
