<script lang="ts">
	/**
	 * Windows — spans of a run ranked by a selector (T1 of the LiDAR workflow
	 * design; this page was Segments, and /lidar/segments redirects here).
	 *
	 * A tuning window worth labelling becomes a clip, and the clip's pack is
	 * cut for the macOS annotation window. A held-out pack (T16) is drawn by
	 * the random selector only, with its seed shown, and its windows are
	 * reviewed and frozen on the Labelling page. The API still calls a window
	 * a segment, the clip a case and the job that cuts its pack a clip job,
	 * until the vocabulary renames land with their aliases.
	 */
	import { replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getLidarRuns } from '#lib/api.js';
	import {
		baseName,
		capturesText,
		inventoryOrder,
		labelsText,
		reviewText,
		type AnnotationPack
	} from '#lib/labelSets.js';
	import {
		bindPlan,
		loadWorkspace,
		nsToISO,
		type PlanProgress,
		type WorkspaceRow
	} from '#lib/labellingWorkspace.js';
	import { clipQuery, linkedRun, runQuery, tracksQuery } from '#lib/lidarLinks.js';
	import {
		keepSelector,
		rankingQuery,
		requirementText,
		scoreText,
		segmentDetail,
		selectorGroups,
		statusText,
		stripImage,
		type SegmentSelector
	} from '#lib/segments.js';
	import type { Run } from '#lib/types/lidar.js';
	import {
		boundPlan,
		drawText,
		frozenText,
		heldOutSelectors,
		otherHeldOut,
		progressText,
		rowsBySegment,
		toneClass,
		windowState,
		type WindowState
	} from '#lib/windows.js';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	type Segment = {
		id: string;
		finder: string;
		window_start_unix_nanos: number;
		window_end_unix_nanos: number;
		peak_timestamp_ns: number;
		score: number;
		pair_seconds?: number;
		pairs?: number;
		leaders?: number;
		leader_changes?: number;
		closest_gap_m?: number;
		events?: number;
		capture?: string;
		offset_seconds?: number;
		status: string;
		clip_id?: string;
		job_id?: string;
		pack_dir?: string;
	};
	type Pack = AnnotationPack;

	// One line of the held-out table: a window the random draw lists, or a
	// held-out window already selected on this run that the draw does not.
	type HeldOutLine = {
		id: string;
		startISO: string;
		capture: string;
		offset?: number;
		finder: string;
		segment?: Segment;
		row?: WorkspaceRow;
		state: WindowState;
		progress: string;
	};

	// The Labelling page keeps the operator's name under this key; a plan
	// binding made here records the same name.
	const ACTOR_KEY = 'velocity.labelling.actor';

	let runs: Run[] = [];
	let runID = '';
	let role = 'tuning';
	let selectorID = 'following';
	let selectors: SegmentSelector[] = [];
	// The digest of the selector the table was ranked with, sent with a case
	// so that the server refuses it if the selector has changed since.
	let rankedDigest = '';
	let segments: Segment[] = [];
	let packs: Pack[] = [];
	let dismissed = new Set<string>();
	let selected: Segment | null = null;
	let busy = false;
	let error = '';
	let message = '';
	// The clip the last "Make clip" produced, linked beside its message.
	let madeClipID = '';
	// The ranking's score strip, an SVG drawn by the server with the ranking.
	let strip = '';

	// Held-out view: the workspace rows of this run's held-out windows, the
	// labelling plans, and the plan chosen to bind.
	let workspaceItems: WorkspaceRow[] = [];
	let plans: PlanProgress[] = [];
	let planID = '';
	let workspaceError = '';
	let actor = '';

	$: heldOut = role === 'held_out';
	$: groups = heldOut
		? [{ category: 'Random', selectors: heldOutSelectors(selectors) }]
		: selectorGroups(selectors, role);
	$: chosen = selectors.find((entry) => entry.id === selectorID);
	$: visible = segments.filter((entry) => !dismissed.has(entry.id));
	$: selectedPack = packs.find((pack) => pack.segment_id === selected?.id);
	$: inventory = inventoryOrder(packs);
	$: bySegment = rowsBySegment(workspaceItems);
	$: heldOutLines = heldOut ? linesFor(visible, workspaceItems, bySegment, packs, runID) : [];
	$: plan = plans.find((entry) => entry.id === planID);
	$: planRow = plan?.binding
		? workspaceItems.find((row) => row.key === plan.binding?.item_key)
		: undefined;

	function linesFor(
		drawn: Segment[],
		items: WorkspaceRow[],
		rows: Map<string, WorkspaceRow>,
		inventory: Pack[],
		run: string
	): HeldOutLine[] {
		const lines: HeldOutLine[] = drawn.map((segment) => {
			const row = rows.get(segment.id);
			const status =
				inventory.find((pack) => pack.segment_id === segment.id)?.status ?? segment.status;
			return {
				id: segment.id,
				startISO: new Date(segment.window_start_unix_nanos / 1e6).toISOString(),
				capture: captureName(segment.capture),
				offset: segment.offset_seconds,
				finder: segment.finder,
				segment,
				row,
				state: windowState(status, row),
				progress: progressText(segment.status, row)
			};
		});
		const drawnIDs = new Set(drawn.map((segment) => segment.id));
		for (const other of otherHeldOut(items, run, drawnIDs)) {
			const row = rows.get(other.segment_id);
			const status = other.pack_dir ? 'packed' : 'case';
			lines.push({
				id: other.segment_id,
				startISO: nsToISO(other.window_start_ns),
				capture: captureName(other.capture),
				finder: other.finder,
				row,
				state: windowState(status, row),
				progress: progressText(status, row)
			});
		}
		return lines;
	}

	async function api(path: string, init?: RequestInit) {
		const response = await fetch(path, init);
		if (!response.ok) {
			const body = await response.json().catch(() => ({}));
			throw new Error(body.error || `Request failed (${response.status})`);
		}
		return response.json();
	}

	// The server's first inventory after it starts reads every pack's review
	// file, tens of megabytes each. Only the status column uses the
	// inventory, so the table does not wait for it.
	async function loadPacks() {
		const inventory = await api('/api/lidar/annotations/packs').catch(() => ({ packs: [] }));
		packs = inventory.packs || [];
	}

	// The held-out view's review state, from the labelling workspace. Its
	// failure leaves the ranking usable and says that progress is unknown.
	// Event handlers test the role itself: the reactive heldOut flag is not
	// updated until after the handler that changed the role has run.
	async function loadHeldOutState() {
		if (role !== 'held_out' || !runID) return;
		workspaceError = '';
		try {
			const response = await loadWorkspace({ role: 'held_out', run: runID, limit: 200 });
			workspaceItems = response.items;
			plans = response.plans.plans;
			if (!plans.some((entry) => entry.id === planID)) {
				planID = plans.find((entry) => entry.binding?.item_key)?.id ?? plans[0]?.id ?? '';
			}
		} catch (cause) {
			workspaceItems = [];
			workspaceError =
				cause instanceof Error
					? `Review state could not be read: ${cause.message}. Progress below is unknown, not zero.`
					: 'Review state could not be read.';
		}
	}

	async function load() {
		if (!runID) return;
		busy = true;
		error = '';
		void loadPacks();
		const reviewState = loadHeldOutState();
		try {
			const ranking = await api(`/api/lidar/segments?${rankingQuery(runID, selectorID, role)}`);
			segments = ranking.windows || [];
			rankedDigest = ranking.selector?.digest ?? '';
			strip = ranking.strip_svg ?? '';
			selected = segments.find((item) => item.id === selected?.id) ?? null;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load windows.';
		} finally {
			busy = false;
		}
		await reviewState;
	}

	async function changeRun() {
		const url = new URL(page.url.href);
		url.searchParams.set('run', runID);
		// The same page with a query added; the rule accepts only a bare resolve().
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		replaceState(url, {});
		await load();
	}

	async function changeRole() {
		const allowed =
			role === 'held_out'
				? [{ category: 'Random', selectors: heldOutSelectors(selectors) }]
				: selectorGroups(selectors, role);
		selectorID = keepSelector(selectorID, allowed);
		await load();
	}

	// Replace the row rather than assigning to it. A row is a plain object, so
	// an assignment to one of its fields redraws nothing: the table would keep
	// offering "Make clip" for a window that already has one.
	function updateSegment(id: string, changes: Partial<Segment>) {
		segments = segments.map((entry) => (entry.id === id ? { ...entry, ...changes } : entry));
		if (selected?.id === id) selected = segments.find((entry) => entry.id === id) ?? null;
	}

	async function makeCase(segment: Segment) {
		busy = true;
		error = '';
		madeClipID = '';
		try {
			const result = await api(`/api/lidar/segments/${encodeURIComponent(segment.id)}/case`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: runID,
					selector: selectorID,
					role,
					selector_digest: rankedDigest
				})
			});
			updateSegment(segment.id, { clip_id: result.clip_id, status: 'case' });
			message =
				role === 'held_out'
					? `Window selected for the held-out pack; clip ${result.clip_id} is ready.`
					: `Clip ${result.clip_id} is ready.`;
			madeClipID = result.clip_id;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not make the clip.';
		} finally {
			busy = false;
		}
	}

	// Cuts the clip's annotation pack. The route and the job kind still say
	// clip; V18 of the vocabulary plan renames them pack.
	async function queuePack(segment: Segment) {
		if (!segment.clip_id) return;
		busy = true;
		error = '';
		madeClipID = '';
		try {
			const result = await api(`/api/lidar/clips/${encodeURIComponent(segment.clip_id)}/clip`, {
				method: 'POST'
			});
			updateSegment(segment.id, { job_id: result.job.job_id, status: 'clipping' });
			message = `Pack job ${result.job.job_id} queued.`;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not queue the pack.';
		} finally {
			busy = false;
		}
	}

	function rememberActor() {
		try {
			localStorage.setItem(ACTOR_KEY, actor);
		} catch {
			// A private window keeps no name; it is asked for again.
		}
	}

	// Binds the chosen plan to a held-out window, against the binding the page
	// loaded; the server refuses with 409 when someone bound it first.
	async function bind(line: HeldOutLine) {
		if (!plan || !line.row) return;
		if (!actor.trim()) {
			error = 'Enter your name before binding a plan: it is recorded with the binding.';
			return;
		}
		busy = true;
		error = '';
		try {
			await bindPlan(plan.id, line.row.key, plan.binding?.revision ?? 0, actor.trim());
			message = `${plan.id} is bound to window ${line.id}.`;
			await loadHeldOutState();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not bind the plan.';
		} finally {
			busy = false;
		}
	}

	function previewQuery(segment: Segment) {
		const run = runs.find((item) => item.run_id === runID);
		return tracksQuery({
			sensorId: run?.sensor_id || 'hesai-pandar40p',
			clipId: segment.clip_id,
			runId: runID,
			atNs: segment.peak_timestamp_ns || segment.window_start_unix_nanos
		});
	}

	function captureName(path?: string) {
		return path?.split(/[\\/]/).pop() || 'Unplaced';
	}

	function labellingHref(key: string) {
		return `${resolve('/lidar/labelling')}?${new URLSearchParams({ item: key })}`;
	}

	onMount(async () => {
		try {
			actor = localStorage.getItem(ACTOR_KEY) ?? '';
		} catch {
			actor = '';
		}
		// The full list carries every run's label progress, which takes a
		// while on a large database. The ranking needs only one run — the
		// linked one, else the newest — so it starts from that, and the list
		// fills the picker on arrival. The pack inventory does not depend on
		// the run.
		void loadPacks();
		const everyRun = getLidarRuns();
		// Its failure is reported below; this keeps one that comes before it
		// is awaited from being an unhandled rejection.
		everyRun.catch(() => undefined);
		const linked = linkedRun(page.url);
		try {
			const [newest, catalogue] = await Promise.all([
				linked ? Promise.resolve([]) : getLidarRuns({ limit: 1 }),
				api('/api/lidar/segments/selectors')
			]);
			runs = linked ? [{ run_id: linked } as Run] : newest;
			selectors = catalogue.selectors || [];
			runID = linked ?? runs[0]?.run_id ?? '';
			await Promise.all([
				load(),
				everyRun.then((listed) => {
					// A linked run the list does not hold stays in the picker,
					// so the page never shows one run's windows under another's id.
					runs = listed.some((run) => run.run_id === runID)
						? listed
						: [...runs.filter((run) => run.run_id === runID), ...listed];
				})
			]);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load window selectors.';
		}
	});
</script>

<svelte:head>
	<title>Windows — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between gap-4">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Windows</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Spans of a run ranked by a selector. Make a
					<a href={resolve('/lidar/clips')} class="text-primary hover:underline">clip</a>
					from one worth labelling and queue its pack; the macOS annotation window opens the pack. Held-out
					packs are drawn at random and reviewed on
					<a href={resolve('/lidar/labelling')} class="text-primary hover:underline">Labelling</a>.
				</p>
			</div>
			<Button variant="outline" on:click={load} disabled={busy}>
				{busy ? 'Loading...' : 'Refresh'}
			</Button>
		</div>
	</div>

	<div class="flex-1 space-y-4 overflow-y-auto p-6 pb-24">
		<div class="flex flex-wrap items-end gap-4 text-sm">
			<label class="text-surface-content/70 flex flex-col gap-1 font-medium"
				>Run
				<select
					class="border-surface-300 bg-surface-100 text-surface-content rounded border px-2 py-1.5 font-mono text-xs"
					bind:value={runID}
					on:change={changeRun}
				>
					{#each runs as run (run.run_id)}<option value={run.run_id}>{run.run_id}</option>{/each}
				</select>
			</label>
			<label class="text-surface-content/70 flex flex-col gap-1 font-medium"
				>Pack role
				<select
					class="border-surface-300 bg-surface-100 text-surface-content rounded border px-2 py-1.5"
					bind:value={role}
					on:change={changeRole}
				>
					<option value="tuning">Tuning</option><option value="held_out">Held-out</option>
				</select>
			</label>
			<label class="text-surface-content/70 flex flex-col gap-1 font-medium"
				>Selector
				{#if heldOut}
					<!-- A held-out pack is drawn at random; the selector is shown, not chosen. -->
					<select
						class="border-surface-300 bg-surface-100 text-surface-content rounded border px-2 py-1.5"
						disabled
						aria-describedby="held-out-rule"
					>
						<option>{drawText(chosen, visible.length)}</option>
					</select>
				{:else}
					<select
						class="border-surface-300 bg-surface-100 text-surface-content rounded border px-2 py-1.5"
						bind:value={selectorID}
						on:change={load}
					>
						{#each groups as group (group.category)}
							<optgroup label={group.category}>
								{#each group.selectors as entry (entry.id)}<option value={entry.id}
										>{entry.label}</option
									>{/each}
							</optgroup>
						{/each}
					</select>
				{/if}
			</label>
			{#if heldOut}
				<label class="text-surface-content/70 flex flex-col gap-1 font-medium"
					>Plan
					<select
						class="border-surface-300 bg-surface-100 text-surface-content rounded border px-2 py-1.5 font-mono text-xs"
						bind:value={planID}
						disabled={!plans.length}
					>
						{#each plans as entry (entry.id)}<option value={entry.id}
								>{entry.id} · {entry.title}</option
							>{:else}<option value="">no plans</option>{/each}
					</select>
				</label>
				<label class="text-surface-content/70 flex flex-col gap-1 font-medium"
					>Your name
					<input
						class="border-surface-300 bg-surface-100 rounded border px-2 py-1"
						bind:value={actor}
						on:change={rememberActor}
						placeholder="recorded with a binding"
					/>
				</label>
			{/if}
			{#if runID}
				<!-- eslint-disable svelte/no-navigation-without-resolve -->
				<a
					href={`${resolve('/lidar/runs')}?${runQuery(runID)}`}
					class="text-primary pb-2 hover:underline">Open run →</a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/if}
		</div>

		{#if heldOut}
			<p id="held-out-rule" class="rounded bg-amber-100 p-3 text-sm text-amber-950">
				Held-out windows are drawn by the random selector only, one per capture, so the pack is not
				shaped by what the finders look for. The draw is recorded with its selector and seed. Frozen
				held-out splits are refused as tuning data. Tracker previews are hidden for blind review.
			</p>
		{:else if chosen}
			<p class="text-surface-content/70 text-sm">
				{chosen.description} Ranked by {scoreText(chosen)}{#if chosen.require.length}; requires {requirementText(
						chosen
					)}{/if}.
			</p>
		{/if}
		{#if error}<p role="alert" class="rounded bg-red-50 px-4 py-3 text-sm text-red-600">
				{error}
			</p>{/if}
		{#if workspaceError && heldOut}<p
				role="alert"
				class="rounded bg-red-50 px-4 py-3 text-sm text-red-600"
			>
				{workspaceError}
			</p>{/if}
		{#if message}<p role="status" class="text-surface-content text-sm">
				{message}
				{#if madeClipID}
					<!-- eslint-disable svelte/no-navigation-without-resolve -->
					<a
						href={`${resolve('/lidar/clips')}?${clipQuery(madeClipID)}`}
						class="text-primary ml-1 hover:underline">Open clip →</a
					>
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				{/if}
			</p>{/if}

		{#if strip && segments.length && !heldOut}
			<section
				aria-label="Per-capture score strip"
				class="border-surface-content/10 bg-surface-100 overflow-x-auto rounded-lg border p-3"
			>
				<img src={stripImage(strip)} alt="Window scores grouped by capture" />
			</section>
		{/if}

		{#if heldOut}
			<div class="border-surface-content/10 bg-surface-100 overflow-x-auto rounded-lg border">
				<table class="w-full text-left text-sm">
					<caption class="sr-only">Held-out windows of this run</caption>
					<thead
						><tr class="text-surface-content/70"
							><th class="p-3 font-medium">Window UTC</th><th class="p-3 font-medium"
								>Capture / offset</th
							><th class="p-3 font-medium">State</th><th class="p-3 font-medium">Progress</th><th
								class="p-3 font-medium">Plan</th
							><th class="p-3 font-medium">Actions</th></tr
						></thead
					>
					<tbody>
						{#each heldOutLines as line (line.id)}
							{@const bound = line.row ? boundPlan(plans, line.row.key) : undefined}
							<tr class="border-surface-content/10 border-t">
								<td class="p-3 font-mono text-xs">{line.startISO}</td>
								<td class="p-3"
									>{line.capture}{#if line.offset !== undefined}
										<span class="text-surface-content/60 font-mono text-xs"
											>@ {line.offset.toFixed(1)} s</span
										>{/if}{#if line.finder !== 'random'}
										<span class="text-surface-content/60 text-xs">· chosen by {line.finder}</span
										>{/if}</td
								>
								<td class="p-3"
									><span
										class={`rounded px-1.5 py-0.5 text-xs whitespace-nowrap ${toneClass(line.state.tone)}`}
										>{line.state.state}</span
									></td
								>
								<td class="text-surface-content/70 p-3">{line.progress}</td>
								<td class="p-3 font-mono text-xs">{bound?.id ?? '—'}</td>
								<td class="text-primary space-x-3 p-3 whitespace-nowrap">
									{#if line.segment && !line.segment.clip_id}
										<button
											class="hover:underline disabled:opacity-40"
											disabled={busy || !line.segment.capture}
											on:click={() => line.segment && makeCase(line.segment)}>Select</button
										>
									{:else if line.segment && !line.segment.job_id && line.segment.status === 'case'}
										<button
											class="hover:underline disabled:opacity-40"
											disabled={busy}
											on:click={() => line.segment && queuePack(line.segment)}>Cut pack</button
										>
									{/if}
									{#if line.row}
										<!-- eslint-disable svelte/no-navigation-without-resolve -->
										<a class="hover:underline" href={labellingHref(line.row.key)}
											>{line.state.state === 'frozen' ? 'Open sealed' : 'Review in Labelling →'}</a
										>
										<!-- eslint-enable svelte/no-navigation-without-resolve -->
										{#if plan && !bound && line.state.state !== 'frozen'}
											<button
												class="hover:underline disabled:opacity-40"
												disabled={busy}
												on:click={() => bind(line)}>Bind {plan.id}</button
											>
										{/if}
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
				{#if !busy && !heldOutLines.length}<p class="text-surface-content/50 p-4 text-sm">
						No windows drawn for this run: its captures are not indexed, or the run has none.
					</p>{/if}
			</div>

			{#if heldOutLines.length}
				<div
					class="border-surface-content/10 bg-surface-100 sticky bottom-0 flex flex-wrap items-center gap-3 rounded-lg border p-3 text-sm shadow"
					aria-label="Held-out pack"
				>
					<span
						>Held-out · run <span class="font-mono">{runID}</span> · {frozenText(
							heldOutLines.map((line) => line.state)
						)}{#if plan?.binding}
							· <span class="font-mono">{plan.id}</span> bound to
							<span class="font-mono">{planRow?.title ?? plan.binding.item_key}</span>{/if}</span
					>
					<span class="grow"></span>
					<a href={resolve('/lidar/labelling')} class="text-primary hover:underline"
						>Freeze on Labelling →</a
					>
				</div>
			{/if}
		{:else}
			<div class="border-surface-content/10 bg-surface-100 overflow-x-auto rounded-lg border">
				<table class="w-full text-left text-sm">
					<thead
						><tr class="text-surface-content/70"
							><th class="p-3 font-medium">Window UTC</th><th class="p-3 font-medium"
								>Capture / offset</th
							><th class="p-3 font-medium">Score</th><th class="p-3 font-medium">Finder detail</th
							><th class="p-3 font-medium">Status</th><th class="p-3 font-medium">Actions</th></tr
						></thead
					>
					<tbody>
						{#each visible as segment (segment.id)}
							<tr class="border-surface-content/10 border-t">
								<td class="p-3">{new Date(segment.window_start_unix_nanos / 1e6).toISOString()}</td>
								<td class="p-3"
									>{captureName(segment.capture)}{#if segment.capture}
										@ {segment.offset_seconds?.toFixed(1)} s{/if}</td
								>
								<td class="p-3">{segment.score.toFixed(2)}</td>
								<td class="p-3">{segmentDetail(segment, segment.finder)}</td>
								<td class="p-3"
									>{statusText(
										packs.find((pack) => pack.segment_id === segment.id)?.status ?? segment.status
									)}</td
								>
								<td class="text-primary space-x-3 p-3 whitespace-nowrap">
									<button class="hover:underline" on:click={() => (selected = segment)}
										>Details</button
									>
									<!-- The rule accepts only a bare resolve() call; this link adds a query to one. -->
									<!-- eslint-disable svelte/no-navigation-without-resolve -->
									<a
										class="hover:underline"
										href={`${resolve('/lidar/tracks')}?${previewQuery(segment)}`}
										>Preview in Tracks</a
									>
									<!-- eslint-enable svelte/no-navigation-without-resolve -->
									{#if !segment.clip_id}<button
											class="hover:underline disabled:opacity-40"
											disabled={busy || !segment.capture}
											on:click={() => makeCase(segment)}>Make clip</button
										>{:else}
										<!-- eslint-disable svelte/no-navigation-without-resolve -->
										<a
											class="hover:underline"
											href={`${resolve('/lidar/clips')}?${clipQuery(segment.clip_id)}`}>Open clip</a
										>
										<!-- eslint-enable svelte/no-navigation-without-resolve -->
										{#if !segment.job_id}<button
												class="hover:underline disabled:opacity-40"
												disabled={busy}
												on:click={() => queuePack(segment)}>Queue pack</button
											>{/if}
									{/if}
									<button
										class="hover:underline"
										on:click={() => {
											dismissed = new Set([...dismissed, segment.id]);
										}}>Hide</button
									>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
				{#if !busy && !visible.length}<p class="text-surface-content/50 p-4 text-sm">
						No windows for this run and selector.
					</p>{/if}
			</div>
		{/if}

		{#if selected && !heldOut}
			<section
				class="border-surface-content/10 bg-surface-100 space-y-1 rounded-lg border p-4 text-sm"
			>
				<h2 class="text-surface-content text-lg font-semibold">Selected window</h2>
				<p>
					{captureName(selected.capture)} · {new Date(
						selected.window_start_unix_nanos / 1e6
					).toISOString()}
				</p>
				<p>Window {selected.id} · {statusText(selected.status)}</p>
				{#if selected.clip_id}<p>
						Clip
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a
							href={`${resolve('/lidar/clips')}?${clipQuery(selected.clip_id)}`}
							class="text-primary font-mono hover:underline">{selected.clip_id}</a
						>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</p>{/if}
				{#if selected.job_id}<p>Pack job {selected.job_id}</p>{/if}
				{#if selectedPack}<p>
						Pack: {selectedPack.pack_dir} · {selectedPack.proposal_layers} proposal layers · {selectedPack.reviewed_objects}
						reviewed objects · {selectedPack.reviewed_masks} reviewed masks
					</p>
					<p>{labelsText(selectedPack)}</p>
					{#each capturesText(selectedPack) as line, i (i)}<p class="text-surface-content/70">
							{line}
						</p>{/each}{/if}
				<p class="text-surface-content/70">
					Open the pack directory in the macOS annotation window to label it.
				</p>
			</section>
		{/if}

		{#if inventory.length > 0 && !heldOut}
			<section
				class="border-surface-content/10 bg-surface-100 space-y-3 rounded-lg border p-4 text-sm"
				aria-label="Annotation packs"
			>
				<h2 class="text-surface-content text-lg font-semibold">Annotation packs</h2>
				<p class="text-surface-content/70">
					Labels live in label sets, which cite a capture's returns by packet, so one set applies to
					every run of the capture. Verified captures are the files checked to hold its packets.
					Edit in the macOS annotation window.
				</p>
				<ul class="space-y-2">
					{#each inventory as pack (pack.pack_dir)}
						<li>
							<p class="font-mono">{baseName(pack.pack_dir)}</p>
							<p>{labelsText(pack)} · {reviewText(pack)}</p>
							{#each capturesText(pack) as line, i (i)}<p class="text-surface-content/70">
									{line}
								</p>{/each}
							{#if pack.error}<p class="text-danger">{pack.error}</p>{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	</div>
</main>
