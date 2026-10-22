<script lang="ts">
	/**
	 * Run a Sweep
	 *
	 * Starts and follows a manual parameter sweep, an auto-tune, or a HINT
	 * (human-in-the-loop) sweep, and applies what it recommends. It replaces
	 * the :8081 sweep dashboard; the requests it sends are the dashboard's.
	 */
	import {
		applyLidarParams,
		continueHINT,
		getHINTState,
		getLidarReplayCase,
		getLidarReplayCases,
		startHINTSweep,
		stopHINT
	} from '#lib/api.js';
	import SweepResults from '#lib/components/lidar/SweepResults.svelte';
	import {
		DEFAULT_LIDAR_SENSOR_ID,
		getAutoTuneStatus,
		getLidarStatus,
		getManualSweepStatus,
		getSuspendedSweep,
		resumeAutoTune,
		startAutoTune,
		startManualSweep,
		stopAutoTune,
		stopManualSweep,
		suspendAutoTune,
		waitForHINTChange,
		type SuspendedSweep,
		type SweepRunStatus
	} from '#lib/lidarControl.js';
	import { runQuery } from '#lib/lidarLinks.js';
	import {
		autoTuneEtaSeconds,
		comboLabel,
		formatParamValues,
		formatScore,
		hintGates,
		RECOMMENDATION_METRIC_KEYS
	} from '#lib/sweepResults.js';
	import {
		applySweepSetup,
		buildAutoTuneRequest,
		buildHintRequest,
		buildManualRequest,
		buildSweepSetup,
		defaultSweepForm,
		estimateSweep,
		formatSeconds,
		newParamRow,
		normaliseParamMap,
		PARAM_NAMES,
		PARAM_SCHEMA,
		paramLabel,
		replaySlotProblem,
		validateSweepForm,
		WEIGHT_FIELDS,
		type SweepMode
	} from '#lib/sweepSetup.js';
	import type { HINTState, LidarReplayCase } from '#lib/types/lidar.js';
	import { resolve } from '$app/paths';
	import { onDestroy, onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	const POLL_MS = 3000;
	const MODES: Array<{ value: SweepMode; label: string }> = [
		{ value: 'manual', label: 'Manual sweep' },
		{ value: 'auto', label: 'Auto-tune' },
		{ value: 'hint', label: 'Human-in-the-loop' }
	];

	const sensorId = DEFAULT_LIDAR_SENSOR_ID;

	let form = $state(defaultSweepForm());
	let clips = $state<LidarReplayCase[]>([]);
	let error = $state<string | null>(null);
	let starting = $state(false);
	let stopping = $state(false);

	let runStatus = $state<SweepRunStatus | null>(null);
	let hint = $state<HINTState | null>(null);
	let suspended = $state<SuspendedSweep | null>(null);
	let applyMessage = $state<string | null>(null);

	// HINT continue controls
	let nextDurationMins = $state(60);
	let addRound = $state(false);
	let continueMessage = $state<string | null>(null);

	// JSON setup editor
	let setupOpen = $state(false);
	let setupJSON = $state('');

	const estimate = $derived(estimateSweep(form));
	const running = $derived(
		form.mode === 'hint'
			? !!hint && !['idle', 'completed', 'failed'].includes(hint.status)
			: runStatus?.status === 'running'
	);
	const results = $derived(runStatus?.results ?? []);
	const recommendation = $derived(
		form.mode === 'hint' && hint?.status === 'completed'
			? hint.recommendation
			: runStatus?.recommendation
	);
	const eta = $derived(runStatus ? autoTuneEtaSeconds(runStatus, Date.now()) : null);

	let pollTimer: ReturnType<typeof setTimeout> | null = null;
	let hintAbort: AbortController | null = null;
	let lastHintStatus = '';

	function stopPolling() {
		if (pollTimer) clearTimeout(pollTimer);
		pollTimer = null;
		hintAbort?.abort();
		hintAbort = null;
	}

	async function pollRun() {
		try {
			runStatus = form.mode === 'auto' ? await getAutoTuneStatus() : await getManualSweepStatus();
			if (runStatus.status !== 'running') stopping = false;
			if (runStatus.status === 'running' || runStatus.status === 'suspended') {
				pollTimer = setTimeout(pollRun, POLL_MS);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			pollTimer = setTimeout(pollRun, POLL_MS * 2);
		}
	}

	function notify(title: string, body: string) {
		if (typeof Notification !== 'undefined' && Notification.permission === 'granted') {
			new Notification(title, { body });
		}
	}

	/** Read the HINT state, then long-poll until it changes. */
	async function pollHint() {
		try {
			hint = (await getHINTState()) as unknown as HINTState;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			pollTimer = setTimeout(pollHint, 5000);
			return;
		}
		if (hint.status !== lastHintStatus) {
			if (hint.status === 'awaiting_labels') {
				notify(
					`Labels needed — round ${hint.current_round}`,
					'The HINT sweep is waiting for track labels.'
				);
			} else if (hint.status === 'completed') {
				notify('HINT sweep complete', 'Parameter optimisation finished.');
			}
			lastHintStatus = hint.status;
		}
		if (['idle', 'completed', 'failed'].includes(hint.status)) {
			stopping = false;
			return;
		}
		hintAbort = new AbortController();
		try {
			await waitForHINTChange(hint.status, hintAbort.signal);
			void pollHint();
		} catch (e) {
			if (!(e instanceof DOMException && e.name === 'AbortError')) {
				pollTimer = setTimeout(pollHint, 5000);
			}
		}
	}

	function startPolling() {
		stopPolling();
		if (form.mode === 'hint') void pollHint();
		else void pollRun();
	}

	function setMode(mode: SweepMode) {
		if (running) return;
		form.mode = mode;
		runStatus = null;
		applyMessage = null;
		error = null;
		startPolling();
	}

	async function chooseClip() {
		const listed = clips.find((c) => c.replay_case_id === form.replayCaseId);
		if (!listed) return;
		form.pcapFile = listed.pcap_file;
		form.pcapStartSecs = listed.pcap_start_secs ?? 0;
		form.pcapDurationSecs =
			listed.pcap_duration_secs != null && listed.pcap_duration_secs > 0
				? listed.pcap_duration_secs
				: -1;
		if (listed.file_count > 1) {
			const full = await getLidarReplayCase(listed.replay_case_id).catch(() => listed);
			const first = [...(full.files ?? [])].sort((a, b) => a.ordinal - b.ordinal)[0];
			if (first) form.pcapFile = first.pcap_file;
		}
	}

	function addRow() {
		form.rows.push(newParamRow());
	}

	function setRowParam(index: number, name: string) {
		form.rows[index] = newParamRow(name);
	}

	function removeRow(index: number) {
		form.rows.splice(index, 1);
	}

	async function handleStart() {
		error = validateSweepForm(form);
		if (error) return;
		starting = true;
		applyMessage = null;
		try {
			error = replaySlotProblem(form, await getLidarStatus());
			if (error) return;
			if (form.mode === 'hint') {
				if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
					void Notification.requestPermission();
				}
				await startHINTSweep(buildHintRequest(form));
			} else if (form.mode === 'auto') {
				await startAutoTune(buildAutoTuneRequest(form));
			} else {
				await startManualSweep(buildManualRequest(form));
			}
			startPolling();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			starting = false;
		}
	}

	async function handleStop() {
		stopping = true;
		try {
			if (form.mode === 'hint') await stopHINT();
			else if (form.mode === 'auto') await stopAutoTune();
			else await stopManualSweep();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			stopping = false;
		}
	}

	async function handleSuspend() {
		try {
			await suspendAutoTune();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	async function handleResume(sweepId?: string) {
		try {
			await resumeAutoTune(sweepId);
			suspended = null;
			form.mode = 'auto';
			startPolling();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	async function handleContinue() {
		continueMessage = null;
		try {
			await continueHINT(nextDurationMins, addRound);
			continueMessage = 'Continued ✓';
		} catch (e) {
			continueMessage = e instanceof Error ? e.message : String(e);
		}
	}

	async function applyRecommendation() {
		if (!recommendation) return;
		const params = Object.fromEntries(
			Object.entries(recommendation).filter(([k]) => !RECOMMENDATION_METRIC_KEYS.has(k))
		);
		try {
			await applyLidarParams(sensorId, params);
			applyMessage = `Applied ${Object.keys(params).length} parameters ✓`;
		} catch (e) {
			applyMessage = e instanceof Error ? e.message : String(e);
		}
	}

	function toggleSetup() {
		setupOpen = !setupOpen;
		if (setupOpen) setupJSON = JSON.stringify(buildSweepSetup(form), null, 2);
	}

	function applySetup() {
		try {
			form = applySweepSetup(form, JSON.parse(setupJSON));
			setupOpen = false;
			error = null;
		} catch (e) {
			error = `Not valid JSON: ${e instanceof Error ? e.message : String(e)}`;
		}
	}

	onMount(async () => {
		clips = await getLidarReplayCases(sensorId).catch(() => [] as LidarReplayCase[]);
		suspended = await getSuspendedSweep().catch(() => null);
		// Pick up a sweep already running, in whichever mode it runs.
		const [manual, auto, hintState] = await Promise.all([
			getManualSweepStatus().catch(() => null),
			getAutoTuneStatus().catch(() => null),
			getHINTState().catch(() => null) as Promise<HINTState | null>
		]);
		if (hintState && !['idle', 'completed', 'failed'].includes(hintState.status))
			form.mode = 'hint';
		else if (auto && ['running', 'suspended'].includes(auto.status)) form.mode = 'auto';
		else if (manual?.status === 'running') form.mode = 'manual';
		startPolling();
	});

	onDestroy(stopPolling);
</script>

<svelte:head>
	<title>Run a Sweep — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Run a Sweep</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Vary tuning parameters over live data or a replay, score each combination, and apply the
					best.
				</p>
			</div>
			<!-- eslint-disable svelte/no-navigation-without-resolve -->
			<a href={resolve('/lidar/sweeps')} class="text-primary text-sm underline">All sweeps</a>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		</div>
		<div class="mt-3 flex gap-2" role="tablist" aria-label="Sweep mode">
			{#each MODES as m (m.value)}
				<button
					type="button"
					role="tab"
					aria-selected={form.mode === m.value}
					disabled={running && form.mode !== m.value}
					class="rounded px-3 py-1 text-sm {form.mode === m.value
						? 'bg-primary text-primary-content'
						: 'bg-surface-200 text-surface-content/70 disabled:opacity-40'}"
					onclick={() => setMode(m.value)}>{m.label}</button
				>
			{/each}
		</div>
	</div>

	<div class="flex-1 overflow-y-auto p-6">
		<div class="vr-content-wide space-y-6">
			{#if suspended?.found && !running}
				<div class="rounded bg-yellow-50 px-4 py-3 text-sm text-yellow-800">
					<strong>Suspended sweep found</strong> — started
					{suspended.started_at
						? new Date(suspended.started_at).toLocaleString()
						: 'at an unknown time'}, completed round {suspended.checkpoint_round}.
					<button
						type="button"
						class="ml-2 font-semibold underline"
						onclick={() => handleResume(suspended?.sweep_id)}>Resume</button
					>
				</div>
			{/if}
			{#if error}
				<div class="rounded bg-red-50 px-4 py-3 text-sm text-red-600">{error}</div>
			{/if}

			<div class="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
				<section class="vr-chart-card">
					<div class="mb-2 flex items-center justify-between">
						<h2 class="text-surface-content text-lg font-semibold">Parameters</h2>
						<div class="flex gap-2">
							<Button variant="outline" size="sm" on:click={toggleSetup}>
								{setupOpen ? 'Close JSON' : 'Edit as JSON'}
							</Button>
							<Button variant="outline" size="sm" on:click={addRow} disabled={running}
								>Add parameter</Button
							>
						</div>
					</div>
					{#if form.mode === 'hint'}
						<p class="text-surface-content/60 mb-2 text-xs">
							With no rows, HINT sweeps the foreground clustering parameters, and the background
							ones too when background tuning is on.
						</p>
					{:else if form.mode === 'auto'}
						<p class="text-surface-content/60 mb-2 text-xs">
							Auto-tune samples each range at {form.valuesPerParam} values a round and narrows it round
							by round; listed values and steps are ignored.
						</p>
					{/if}

					{#if setupOpen}
						<textarea
							class="vr-input mb-2 h-64 w-full font-mono text-xs"
							spellcheck="false"
							bind:value={setupJSON}></textarea>
						<Button variant="fill" color="primary" size="sm" on:click={applySetup}
							>Load into form</Button
						>
					{/if}

					<div class="space-y-3">
						{#each form.rows as row, i (i)}
							{@const schema = PARAM_SCHEMA[row.name]}
							<div class="border-surface-content/10 rounded border p-3">
								<div class="flex flex-wrap items-end gap-3">
									<label class="vr-field min-w-56 flex-1">
										<span>Parameter</span>
										<select
											class="vr-input"
											value={row.name}
											onchange={(e) => setRowParam(i, e.currentTarget.value)}
										>
											<option value="">Choose a parameter</option>
											{#each PARAM_NAMES as name (name)}
												<option value={name}>{PARAM_SCHEMA[name].label}</option>
											{/each}
										</select>
									</label>
									{#if schema && ['float64', 'int', 'int64'].includes(schema.type)}
										<label class="vr-field w-28">
											<span>Start</span>
											<input
												class="vr-input"
												type="number"
												step={schema.step}
												bind:value={row.start}
											/>
										</label>
										<label class="vr-field w-28">
											<span>End</span>
											<input
												class="vr-input"
												type="number"
												step={schema.step}
												bind:value={row.end}
											/>
										</label>
										{#if form.mode === 'manual'}
											<label class="vr-field w-24">
												<span>Step</span>
												<input
													class="vr-input"
													type="number"
													step={schema.step}
													bind:value={row.step}
												/>
											</label>
										{/if}
									{/if}
									{#if schema && form.mode === 'manual'}
										<label class="vr-field min-w-40 flex-1">
											<span
												>{schema.type === 'bool' || schema.type === 'string'
													? 'Values'
													: 'Values (override the range)'}</span
											>
											<input
												class="vr-input"
												placeholder={schema.type === 'string'
													? 'e.g. 500ms, 1s'
													: 'e.g. 0.01, 0.02'}
												bind:value={row.values}
											/>
										</label>
									{/if}
									<button
										type="button"
										class="text-surface-content/50 hover:text-surface-content px-2 pb-1 text-lg"
										aria-label="Remove {schema?.label ?? 'parameter'}"
										onclick={() => removeRow(i)}>×</button
									>
								</div>
								{#if schema}
									<p class="text-surface-content/50 mt-1 text-xs">{schema.desc}</p>
								{/if}
							</div>
						{/each}
					</div>

					{#if estimate}
						<p class="text-surface-content/70 mt-3 text-xs">
							{estimate.permutations} combinations{estimate.withSeed > estimate.permutations
								? ` × seed toggle = ${estimate.withSeed}`
								: ''}{estimate.rounds > 1 ? ` × ${estimate.rounds} rounds = ${estimate.total}` : ''} ·
							{formatSeconds(estimate.measurementSecs)} measured per combination · about {formatSeconds(
								estimate.runtimeSecs
							)} in all
						</p>
					{/if}
				</section>

				<div class="space-y-6">
					<section class="vr-chart-card space-y-3">
						<h2 class="text-surface-content text-lg font-semibold">Data</h2>
						{#if form.mode !== 'hint'}
							<label class="vr-field">
								<span>Source</span>
								<select class="vr-input" bind:value={form.dataSource}>
									<option value="live">Live</option>
									<option value="pcap">Capture replay</option>
									<option value="replay_case">Clip</option>
								</select>
							</label>
						{/if}
						{#if form.mode === 'hint' || form.dataSource === 'replay_case'}
							<label class="vr-field">
								<span>Clip</span>
								<select class="vr-input" bind:value={form.replayCaseId} onchange={chooseClip}>
									<option value="">Choose a clip</option>
									{#each clips as clip (clip.replay_case_id)}
										<option value={clip.replay_case_id}>{clip.description || clip.pcap_file}</option
										>
									{/each}
								</select>
							</label>
						{/if}
						{#if form.mode !== 'hint' && form.dataSource !== 'live'}
							<label class="vr-field">
								<span>Capture file</span>
								<input class="vr-input font-mono" bind:value={form.pcapFile} />
							</label>
							<div class="grid grid-cols-2 gap-3">
								<label class="vr-field">
									<span>Start (s)</span>
									<input
										class="vr-input"
										type="number"
										step="0.1"
										min="0"
										bind:value={form.pcapStartSecs}
									/>
								</label>
								<label class="vr-field">
									<span>Duration (s, -1 for all)</span>
									<input
										class="vr-input"
										type="number"
										step="0.1"
										bind:value={form.pcapDurationSecs}
									/>
								</label>
							</div>
						{/if}
					</section>

					{#if form.mode !== 'hint'}
						<section class="vr-chart-card grid grid-cols-2 gap-3">
							<h2 class="text-surface-content col-span-2 text-lg font-semibold">Measurement</h2>
							<label class="vr-field">
								<span>Seed behaviour</span>
								<select class="vr-input" bind:value={form.seed}>
									<option value="true">True</option>
									<option value="false">False</option>
									<option value="toggle">Toggle</option>
								</select>
							</label>
							<label class="vr-field">
								<span>Iterations</span>
								<input
									class="vr-input"
									type="number"
									min="1"
									max="500"
									bind:value={form.iterations}
								/>
							</label>
							<label class="vr-field">
								<span>Interval</span>
								<input class="vr-input" bind:value={form.interval} />
							</label>
							<label class="vr-field">
								<span>Settle time</span>
								<input class="vr-input" bind:value={form.settleTime} />
							</label>
							<label class="vr-field col-span-2">
								<span>Region settle</span>
								<select class="vr-input" bind:value={form.settleMode}>
									<option value="once">Settle once, reuse</option>
									<option value="per_combo">Per combination</option>
								</select>
							</label>
						</section>
					{/if}

					{#if form.mode !== 'manual'}
						<section class="vr-chart-card grid grid-cols-2 gap-3">
							<h2 class="text-surface-content col-span-2 text-lg font-semibold">
								{form.mode === 'auto' ? 'Auto-tune' : 'HINT'}
							</h2>
							{#if form.mode === 'auto'}
								<label class="vr-field">
									<span>Max rounds</span>
									<input
										class="vr-input"
										type="number"
										min="1"
										max="10"
										bind:value={form.maxRounds}
									/>
								</label>
							{:else}
								<label class="vr-field">
									<span>Rounds</span>
									<input
										class="vr-input"
										type="number"
										min="1"
										max="10"
										bind:value={form.hintRounds}
									/>
								</label>
							{/if}
							<label class="vr-field">
								<span>Values per parameter</span>
								<input
									class="vr-input"
									type="number"
									min="2"
									max="20"
									bind:value={form.valuesPerParam}
								/>
							</label>
							<label class="vr-field">
								<span>Top K</span>
								<input class="vr-input" type="number" min="1" max="50" bind:value={form.topK} />
							</label>
							{#if form.mode === 'auto'}
								<label class="vr-field">
									<span>Objective</span>
									<select class="vr-input" bind:value={form.objective}>
										<option value="acceptance">Acceptance rate</option>
										<option value="weighted">Weighted</option>
										{#if form.dataSource === 'replay_case' && form.replayCaseId}
											<option value="ground_truth">Ground truth (clip labels)</option>
										{/if}
									</select>
								</label>
								{#if form.objective === 'weighted'}
									{#each WEIGHT_FIELDS as w (w.key)}
										<label class="vr-field">
											<span>{w.label}</span>
											<input
												class="vr-input"
												type="number"
												step={w.step}
												bind:value={form.weights[w.key]}
											/>
										</label>
									{/each}
									<p class="text-surface-content/60 col-span-2 text-xs">
										Acceptance criteria: a combination over any limit is rejected. Leave blank for
										none.
									</p>
									<label class="vr-field">
										<span>Max fragmentation</span>
										<input
											class="vr-input"
											bind:value={form.acceptance.maxFragmentation}
											placeholder="0.10"
										/>
									</label>
									<label class="vr-field">
										<span>Max unbounded</span>
										<input
											class="vr-input"
											bind:value={form.acceptance.maxUnbounded}
											placeholder="0.05"
										/>
									</label>
									<label class="vr-field">
										<span>Max empty boxes</span>
										<input
											class="vr-input"
											bind:value={form.acceptance.maxEmptyBoxes}
											placeholder="0.10"
										/>
									</label>
								{/if}
							{:else}
								<label class="vr-field">
									<span>Label threshold (%)</span>
									<input
										class="vr-input"
										type="number"
										min="10"
										max="100"
										step="5"
										bind:value={form.hintThresholdPct}
									/>
								</label>
								<label class="vr-field">
									<span>Class coverage (JSON)</span>
									<input
										class="vr-input"
										placeholder={'{"vehicle":3}'}
										bind:value={form.hintClassCoverage}
									/>
								</label>
								<label class="vr-field">
									<span>Temporal spread (s)</span>
									<input
										class="vr-input"
										type="number"
										min="0"
										bind:value={form.hintTemporalSpreadSecs}
									/>
								</label>
								<label class="col-span-2 flex items-center gap-2 text-sm">
									<input type="checkbox" bind:checked={form.hintCarryOver} /> Carry labels over between
									rounds
								</label>
								<label class="col-span-2 flex items-center gap-2 text-sm">
									<input type="checkbox" bind:checked={form.hintTuneBackground} /> Tune background parameters
									too
								</label>
							{/if}
						</section>
					{/if}

					<section class="vr-chart-card space-y-3">
						<div class="flex flex-wrap gap-2">
							{#if !running}
								<Button variant="fill" color="primary" on:click={handleStart} disabled={starting}>
									{starting
										? 'Starting…'
										: `Start ${MODES.find((m) => m.value === form.mode)?.label.toLowerCase()}`}
								</Button>
							{:else}
								<Button variant="fill" color="danger" on:click={handleStop} disabled={stopping}>
									{stopping ? 'Stopping after this combination…' : 'Stop'}
								</Button>
								{#if form.mode === 'auto'}
									<Button variant="outline" on:click={handleSuspend}>Suspend</Button>
								{/if}
							{/if}
							{#if form.mode === 'auto' && runStatus?.status === 'suspended'}
								<Button variant="outline" on:click={() => handleResume()}>Resume</Button>
							{/if}
						</div>

						{#if form.mode !== 'hint' && runStatus && runStatus.status !== 'idle'}
							<div class="text-sm">
								<span class="bg-surface-200 rounded px-2 py-0.5 text-xs font-medium"
									>{runStatus.status}</span
								>
								{#if runStatus.total_rounds}
									Round {runStatus.round || 1}/{runStatus.total_rounds} —
								{/if}
								{runStatus.completed_combos ?? 0} / {runStatus.total_combos ?? 0} combinations
								{#if eta}— about {formatSeconds(eta)} left{/if}
							</div>
							{#if runStatus.status === 'running' && runStatus.current_combo}
								<p class="text-surface-content/60 text-xs">
									Current: {comboLabel(runStatus.current_combo)} → acceptance {(
										Number(runStatus.current_combo.overall_accept_mean ?? 0) * 100
									).toFixed(1)}%
								</p>
							{/if}
							{#if runStatus.round_results?.length}
								{@const last = runStatus.round_results[runStatus.round_results.length - 1]}
								<p class="text-surface-content/60 text-xs">
									Last round best: {formatScore(last.best_score)} — {formatParamValues(
										last.best_params
									)}
								</p>
							{/if}
							{#if runStatus.error}
								<p class="text-sm text-red-500">{runStatus.error}</p>
							{/if}
							{#if runStatus.warnings?.length}
								<details>
									<summary class="text-surface-content/60 cursor-pointer text-xs"
										>Warnings ({runStatus.warnings.length})</summary
									>
									<ul class="text-surface-content/60 mt-1 list-disc pl-4 text-xs">
										{#each runStatus.warnings as w, i (i)}<li>{w}</li>{/each}
									</ul>
								</details>
							{/if}
						{/if}

						{#if form.mode === 'hint' && hint && hint.status !== 'idle'}
							<div class="text-sm">
								<strong>Round {hint.current_round} / {hint.total_rounds}</strong> —
								<span class="bg-surface-200 rounded px-2 py-0.5 text-xs font-medium"
									>{hint.status.replace(/_/g, ' ')}</span
								>
							</div>
							{#if hint.status === 'awaiting_labels'}
								{@const pctDone = hint.label_progress?.progress_pct ?? 0}
								{@const gates = hintGates(hint)}
								<div>
									<div class="text-surface-content/60 mb-1 flex justify-between text-xs">
										<span
											>{hint.label_progress?.labelled ?? 0}/{hint.label_progress?.total ?? 0} labelled</span
										>
										<span>{pctDone.toFixed(1)}%</span>
									</div>
									<div class="bg-surface-200 relative h-2 rounded">
										<div
											class="bg-primary h-2 rounded"
											style:width="{Math.min(100, pctDone)}%"
										></div>
										<div
											class="absolute top-[-2px] h-3 w-0.5 bg-yellow-500"
											style:left="{hint.min_label_threshold * 100}%"
											title="{(hint.min_label_threshold * 100).toFixed(0)}% threshold"
										></div>
									</div>
									{#if hint.labels_carried_over > 0}
										<p class="text-surface-content/50 mt-1 text-xs">
											↻ {hint.labels_carried_over} labels carried over
										</p>
									{/if}
									<ul class="mt-2 space-y-0.5 text-xs">
										{#each gates as g (g.label)}<li>{g.met ? '✅' : '⬜'} {g.label}</li>{/each}
									</ul>
								</div>
								{#if hint.reference_run_id}
									<!-- eslint-disable svelte/no-navigation-without-resolve -->
									<a
										href={`${resolve('/lidar/runs')}?${runQuery(hint.reference_run_id)}`}
										class="text-primary text-sm underline">Open the reference run to label →</a
									>
									<!-- eslint-enable svelte/no-navigation-without-resolve -->
								{/if}
								<div class="flex flex-wrap items-end gap-3">
									<label class="vr-field w-40">
										<span>Next sweep (minutes)</span>
										<input class="vr-input" type="number" min="1" bind:value={nextDurationMins} />
									</label>
									<label class="flex items-center gap-2 pb-1 text-sm">
										<input type="checkbox" bind:checked={addRound} /> Add a round
									</label>
									<Button
										variant="fill"
										color="primary"
										size="sm"
										on:click={handleContinue}
										disabled={!gates[0].met}>Continue to sweep</Button
									>
								</div>
								{#if continueMessage}<p class="text-xs">{continueMessage}</p>{/if}
							{:else if hint.status === 'running_sweep' && hint.auto_tune_state}
								{@const ats = hint.auto_tune_state as Record<string, number>}
								<p class="text-surface-content/60 text-xs">
									Sweep: {ats.completed_combos}/{ats.total_combos} combinations (round {ats.round}/{ats.total_rounds})
								</p>
							{:else if hint.status === 'running_reference'}
								<p class="text-surface-content/60 text-xs">Creating the reference run…</p>
							{:else if hint.status === 'failed'}
								<p class="text-sm text-red-500">{hint.error || 'Unknown error'}</p>
							{/if}
							{#if hint.round_history?.length}
								<ul class="border-surface-content/10 space-y-1 border-t pt-2 text-xs">
									{#each hint.round_history as rnd (rnd.round)}
										<li>
											<strong>Round {rnd.round}</strong>
											{#if rnd.best_score != null}— score {formatScore(rnd.best_score)}{/if}
											{#if rnd.labels_carried_over > 0}(↻ {rnd.labels_carried_over} labels){/if}
											{#if rnd.reference_run_id}
												<!-- eslint-disable svelte/no-navigation-without-resolve -->
												<a
													href={`${resolve('/lidar/runs')}?${runQuery(rnd.reference_run_id)}`}
													class="text-primary underline">run →</a
												>
												<!-- eslint-enable svelte/no-navigation-without-resolve -->
											{/if}
										</li>
									{/each}
								</ul>
							{/if}
						{/if}
					</section>
				</div>
			</div>

			{#if recommendation && Object.keys(recommendation).length > 0}
				{@const params = normaliseParamMap(recommendation)}
				<section class="vr-chart-card">
					<div class="mb-2 flex items-center justify-between">
						<h2 class="text-surface-content text-lg font-semibold">Recommendation</h2>
						<div class="flex items-center gap-3">
							{#if applyMessage}<span class="text-xs">{applyMessage}</span>{/if}
							<Button variant="fill" color="primary" size="sm" on:click={applyRecommendation}
								>Apply to the running pipeline</Button
							>
						</div>
					</div>
					<table class="text-xs">
						<tbody>
							{#each Object.entries(params) as [key, value] (key)}
								<tr>
									<td class="text-surface-content/70 py-0.5 pr-4">{paramLabel(key)}</td>
									<td class="py-0.5 font-mono"
										>{typeof value === 'number' && !Number.isInteger(value)
											? value.toFixed(4)
											: String(value)}</td
									>
								</tr>
							{/each}
						</tbody>
					</table>
				</section>
			{/if}

			{#if form.mode !== 'hint'}
				<section>
					<h2 class="text-surface-content mb-3 text-lg font-semibold">Results</h2>
					<SweepResults {results} />
				</section>
			{/if}
		</div>
	</div>
</main>
