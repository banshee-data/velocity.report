<script lang="ts">
	/**
	 * Captures — what is on the capture volumes, and what can be made from it.
	 *
	 * Field capture writes a rolling series of five-minute files, so the unit
	 * that matters is not the file but the session: the run of files whose
	 * packet clocks abut. A static stretch worth replaying routinely spans
	 * several of them, which is why selection here is by session and by an
	 * ordered set of captures rather than by one file at a time.
	 */
	import {
		cancelCaptureJob,
		createReplayCaseFromCaptures,
		setReplayCaseLocation,
		getCaptureFiles,
		getCaptureJobs,
		getCapturePeriods,
		getCaptureRoots,
		getCaptureSessions,
		scanCaptureRoots,
		setCaptureSessionLabel,
		startCaptureMotionPass
	} from '$lib/api';
	import {
		formatClock,
		formatDay,
		formatDuration,
		formatGap,
		formatSize,
		gradeSelection,
		jobProgressPercent,
		periodTrim,
		probedFiles,
		sessionShare,
		trimWindow
	} from '$lib/captures/timeline';
	import CoverageTimeline from '$lib/components/lidar/CoverageTimeline.svelte';
	import MotionLegend from '$lib/components/lidar/MotionLegend.svelte';
	import MotionStrip from '$lib/components/lidar/MotionStrip.svelte';
	import {
		JOB_QUEUED,
		JOB_RUNNING,
		PROBE_FAILED,
		PROBE_OK,
		PROBE_PENDING,
		SCAN_NEVER,
		SCAN_UNREACHABLE,
		type CaptureFile,
		type CaptureJob,
		type CaptureRoot,
		type CaptureSession,
		type MotionPeriod,
		type ScanRootResult
	} from '$lib/types/captures';
	import { resolve } from '$app/paths';
	import { onDestroy, onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	let roots: CaptureRoot[] = [];
	let sessions: CaptureSession[] = [];
	let jobs: CaptureJob[] = [];
	let periodsBySession: Record<string, MotionPeriod[]> = {};

	let selectedRootId: string | null = null;
	let expandedSessionId: string | null = null;
	let sessionFiles: CaptureFile[] = [];
	let filesLoading = false;

	let loading = true;
	let error: string | null = null;
	let scanning = false;
	let lastScan: ScanRootResult | null = null;
	let scanPoll: ReturnType<typeof setTimeout> | null = null;

	// Selection for building a case. Keyed by capture_file_id so a re-fetch
	// does not lose what the operator picked.
	let selectedFileIds: string[] = [];
	let creating = false;
	let createError: string | null = null;
	let createdMessage: string | null = null;
	let caseDescription = '';
	let caseSensorId = 'hesai-pandar40p';
	// Trim into the selection, in seconds — how much of the first capture's
	// leading motion to skip, and how long the case runs. Blank means "from
	// the very start" / "to the very end" of the selection.
	let caseStartSecs = '';
	let caseDurationSecs = '';

	let labelDraft = '';
	let jobPoll: ReturnType<typeof setInterval> | null = null;

	// Where the capture was taken. A static segment is only useful as a scene
	// once it has a place, and this is the moment the operator knows it.
	let originLat = '';
	let originLon = '';
	let geoSource = 'operator';
	let locationNote: string | null = null;

	$: activeRoot = roots.find((r) => r.root_id === selectedRootId) ?? roots[0] ?? null;
	$: visibleSessions = activeRoot
		? sessions.filter((s) => s.root_id === activeRoot.root_id)
		: sessions;
	$: expandedSession = visibleSessions.find((s) => s.session_id === expandedSessionId) ?? null;
	$: selectedFiles = sessionFiles.filter((f) => selectedFileIds.includes(f.capture_file_id));
	$: verdict = gradeSelection(selectedFiles);
	$: sequenceStartNs = verdict.files[0]?.first_packet_ns ?? null;
	$: trimmedWindow =
		sequenceStartNs != null ? trimWindow(sequenceStartNs, caseStartSecs, caseDurationSecs) : null;
	$: expandedPeriods = expandedSessionId ? (periodsBySession[expandedSessionId] ?? []) : [];
	$: expandedShare = sessionShare(expandedPeriods);
	$: activeJobs = jobs.filter((j) => j.state === JOB_QUEUED || j.state === JOB_RUNNING);
	$: sessionJob = expandedSessionId
		? activeJobs.find((j) => j.session_id === expandedSessionId)
		: undefined;

	async function loadIndex() {
		loading = true;
		error = null;
		try {
			const [rootsResult, sessionsResult, jobsResult] = await Promise.all([
				getCaptureRoots(),
				getCaptureSessions(),
				getCaptureJobs({ limit: 25 })
			]);
			roots = rootsResult;
			sessions = sessionsResult;
			jobs = jobsResult;
			if (!selectedRootId && roots.length > 0) selectedRootId = roots[0].root_id;
			await loadPeriodsFor(sessionsResult);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the capture index.';
		} finally {
			loading = false;
		}
	}

	/**
	 * Load the motion timeline for each session so the coverage view can be
	 * filled in. Sessions without a pass simply come back empty, which the
	 * strip renders as "no motion pass yet" rather than as an empty session.
	 */
	async function loadPeriodsFor(list: CaptureSession[]) {
		const results = await Promise.all(
			list.map(async (s) => {
				try {
					const r = await getCapturePeriods(s.session_id);
					return [s.session_id, r.periods] as const;
				} catch {
					return [s.session_id, [] as MotionPeriod[]] as const;
				}
			})
		);
		periodsBySession = Object.fromEntries(results);
	}

	async function runScan(probe: boolean) {
		scanning = true;
		error = null;
		lastScan = null;
		try {
			const result = await scanCaptureRoots({
				rootId: activeRoot?.root_id,
				probe,
				async: probe
			});
			lastScan = result.roots[0] ?? null;
			if (probe) {
				startScanPolling(activeRoot?.root_id);
				return;
			}
			await loadIndex();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not scan the capture volumes.';
		} finally {
			if (!probe) scanning = false;
		}
	}

	function startScanPolling(rootId?: string) {
		stopScanPolling();
		const poll = async () => {
			try {
				await loadIndex();
				const root = roots.find((r) => r.root_id === rootId) ?? activeRoot;
				if (!root?.scan_in_progress) {
					scanning = false;
					scanPoll = null;
					return;
				}
				scanPoll = setTimeout(poll, 2000);
			} catch {
				scanning = false;
				scanPoll = null;
			}
		};
		scanPoll = setTimeout(poll, 1000);
	}

	function stopScanPolling() {
		if (scanPoll) clearTimeout(scanPoll);
		scanPoll = null;
	}

	async function toggleSession(sessionId: string) {
		if (expandedSessionId === sessionId) {
			expandedSessionId = null;
			sessionFiles = [];
			selectedFileIds = [];
			return;
		}
		expandedSessionId = sessionId;
		selectedFileIds = [];
		caseStartSecs = '';
		caseDurationSecs = '';
		createdMessage = null;
		createError = null;
		labelDraft = visibleSessions.find((s) => s.session_id === sessionId)?.label ?? '';
		filesLoading = true;
		try {
			sessionFiles = await getCaptureFiles({ sessionId });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the session captures.';
		} finally {
			filesLoading = false;
		}
	}

	function toggleFile(fileId: string) {
		selectedFileIds = selectedFileIds.includes(fileId)
			? selectedFileIds.filter((id) => id !== fileId)
			: [...selectedFileIds, fileId];
		createdMessage = null;
		createError = null;
	}

	/**
	 * Select every capture the period covers. This is the common case by a
	 * distance: an operator wants the stretch the motion pass found, not an
	 * arbitrary set of files, and the stretch usually spans a boundary.
	 */
	function selectPeriod(period: MotionPeriod) {
		const matched = sessionFiles.filter(
			(f) =>
				f.first_packet_ns != null &&
				f.last_packet_ns != null &&
				f.last_packet_ns > period.start_ns &&
				f.first_packet_ns < period.end_ns
		);
		selectedFileIds = matched.map((f) => f.capture_file_id);
		caseDescription = `${expandedSession?.label || 'session'} ${period.label}`;

		// The period usually starts after the first matched file's own start —
		// that file's leading seconds are motion, from before the sensor
		// settled. Defaulting the trim to the period's own bounds excludes
		// them, rather than replaying the whole first file.
		const ordered = probedFiles(matched);
		const firstNs = ordered[0]?.first_packet_ns;
		if (firstNs != null) {
			const trim = periodTrim(period, firstNs);
			caseStartSecs = trim.startSecs.toFixed(2);
			caseDurationSecs = trim.durationSecs.toFixed(2);
		}
		createdMessage = null;
		createError = null;
	}

	async function queueMotionPass() {
		if (!expandedSessionId) return;
		error = null;
		try {
			await startCaptureMotionPass(expandedSessionId);
			jobs = await getCaptureJobs({ limit: 25 });
			startJobPolling();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not queue the motion pass.';
		}
	}

	async function cancelJob(jobId: string) {
		try {
			await cancelCaptureJob(jobId);
			jobs = await getCaptureJobs({ limit: 25 });
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not cancel the job.';
		}
	}

	async function saveLabel() {
		if (!expandedSessionId) return;
		try {
			await setCaptureSessionLabel(expandedSessionId, labelDraft.trim());
			sessions = await getCaptureSessions();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not name the session.';
		}
	}

	/** Parses a trim field to seconds, or undefined for blank/unparseable text. */
	function parseOptionalSecs(text: string): number | undefined {
		const trimmed = text.trim();
		if (trimmed === '') return undefined;
		const n = Number(trimmed);
		return Number.isFinite(n) ? n : undefined;
	}

	async function createCase() {
		if (!verdict.usable || verdict.files.length === 0) return;
		creating = true;
		createError = null;
		createdMessage = null;
		locationNote = null;
		try {
			const created = await createReplayCaseFromCaptures({
				sensor_id: caseSensorId,
				pcap_files: verdict.files.map((f) => f.rel_path),
				pcap_start_secs: parseOptionalSecs(caseStartSecs),
				pcap_duration_secs: parseOptionalSecs(caseDurationSecs),
				description: caseDescription || undefined,
				session_id: expandedSessionId ?? undefined
			});
			createdMessage = `Created ${created.replay_case_id} over ${verdict.files.length} capture${verdict.files.length === 1 ? '' : 's'}.`;

			// The position is recorded against the case once it exists, so a
			// bad fix cannot stop the case being created — it can be corrected
			// afterwards without losing the selection's work.
			const lat = Number(originLat);
			const lon = Number(originLon);
			if (originLat.trim() !== '' && originLon.trim() !== '') {
				try {
					const loc = await setReplayCaseLocation(created.replay_case_id, {
						origin_lat: lat,
						origin_lon: lon,
						geographic_source: geoSource
					});
					createdMessage += ` Located at ${loc.s2_l10_display} (L10) · ${loc.s2_l13_display} (L13) · ${loc.s2_l16_display} (L16).`;
				} catch (e) {
					locationNote =
						e instanceof Error
							? `Case created, but the location was not recorded: ${e.message}`
							: 'Case created, but the location was not recorded.';
				}
			}
			selectedFileIds = [];
		} catch (e) {
			createError = e instanceof Error ? e.message : 'Could not create the replay case.';
		} finally {
			creating = false;
		}
	}

	/**
	 * Poll while work is outstanding, and stop when it is not. A motion pass
	 * runs for minutes, so the page has to show it moving; polling an idle
	 * queue for ever would be a needless request every few seconds.
	 */
	function startJobPolling() {
		if (jobPoll) return;
		jobPoll = setInterval(async () => {
			try {
				jobs = await getCaptureJobs({ limit: 25 });
				if (!jobs.some((j) => j.state === JOB_QUEUED || j.state === JOB_RUNNING)) {
					stopJobPolling();
					sessions = await getCaptureSessions();
					await loadPeriodsFor(sessions);
				}
			} catch {
				stopJobPolling();
			}
		}, 3000);
	}

	function stopJobPolling() {
		if (jobPoll) {
			clearInterval(jobPoll);
			jobPoll = null;
		}
	}

	function probeTone(state: string): string {
		if (state === PROBE_OK) return 'text-emerald-600';
		if (state === PROBE_FAILED) return 'text-red-600';
		return 'text-surface-content/40';
	}

	function seamTone(grade: string): string {
		if (grade === 'seamless') return 'text-emerald-600';
		if (grade === 'acceptable') return 'text-amber-600';
		return 'text-red-600';
	}

	onMount(() => {
		loadIndex().then(() => {
			if (jobs.some((j) => j.state === JOB_QUEUED || j.state === JOB_RUNNING)) startJobPolling();
		});
	});

	onDestroy(() => {
		stopJobPolling();
		stopScanPolling();
	});
</script>

<svelte:head><title>Captures — velocity.report</title></svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<header class="flex items-start justify-between gap-4">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Captures</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					What is on the capture volumes, grouped into the sessions their packet clocks form. A
					replay case is a window over a session, which may span several capture files.
				</p>
			</div>
			<div class="flex shrink-0 gap-4">
				<a href={resolve('/lidar/scene-map')} class="text-primary text-sm hover:underline">
					Scene map →
				</a>
				<a href={resolve('/lidar/replay-cases')} class="text-primary text-sm hover:underline">
					Replay cases →
				</a>
			</div>
		</header>
	</div>

	<div class="flex flex-1 overflow-hidden">
		<div class="flex-1 overflow-y-auto p-4">
			<!-- Volume state -->
			<section class="border-surface-300 bg-surface-100 mb-4 rounded border p-3">
				<div class="flex flex-wrap items-center gap-3">
					{#if roots.length > 1}
						<select
							class="border-surface-300 bg-surface-100 rounded border px-2 py-1 font-mono text-xs"
							bind:value={selectedRootId}
						>
							{#each roots as root (root.root_id)}
								<option value={root.root_id}>{root.path}</option>
							{/each}
						</select>
					{:else if activeRoot}
						<span class="text-surface-content font-mono text-xs">{activeRoot.path}</span>
					{/if}

					{#if activeRoot?.last_scan_state === SCAN_UNREACHABLE}
						<span class="rounded bg-red-100 px-2 py-0.5 text-xs text-red-700"
							>volume unreachable</span
						>
					{:else if activeRoot?.last_scan_state === SCAN_NEVER}
						<span class="bg-surface-200 text-surface-content/60 rounded px-2 py-0.5 text-xs">
							never scanned
						</span>
					{:else if activeRoot?.last_scan_at_ns}
						<span class="text-surface-content/50 text-xs">
							scanned {formatDay(activeRoot.last_scan_at_ns)}
							{formatClock(activeRoot.last_scan_at_ns)}
						</span>
					{/if}

					<span class="flex-1"></span>

					<Button size="sm" variant="outline" disabled={scanning} on:click={() => runScan(false)}>
						{scanning ? 'Scanning…' : 'Quick scan'}
					</Button>
					<Button
						size="sm"
						variant="fill"
						color="primary"
						disabled={scanning}
						on:click={() => runScan(true)}
					>
						Scan and probe
					</Button>
				</div>

				<p class="text-surface-content/40 mt-2 text-xs">
					A quick scan lists what is there. Probing reads each new capture in full to learn its
					packet extent, which is what sessions and replay cases are built from — minutes per volume
					on a first pass, and nothing at all when nothing has changed.
				</p>

				{#if activeRoot?.last_scan_error}
					<p class="mt-2 rounded bg-red-50 px-3 py-2 text-xs text-red-600">
						{activeRoot.last_scan_error}
					</p>
				{/if}

				{#if lastScan}
					<p class="border-surface-300 text-surface-content/70 mt-2 border-t pt-2 text-xs">
						<span class="font-mono">{lastScan.drift}</span>
						{#if lastScan.probed > 0}· probed {lastScan.probed}{/if}
						{#if lastScan.probe_failed > 0}
							· <span class="text-red-600">{lastScan.probe_failed} could not be probed</span>
						{/if}
					</p>
				{/if}
			</section>

			{#if error}
				<div class="mb-4 rounded bg-red-50 px-3 py-2 text-sm text-red-600">{error}</div>
			{/if}

			{#if activeJobs.length > 0}
				<section class="mb-4 rounded border border-amber-300 bg-amber-50 p-3">
					<h2 class="mb-2 text-sm font-medium text-amber-900">Running</h2>
					{#each activeJobs as job (job.job_id)}
						{@const percent = jobProgressPercent(job.progress_current, job.progress_total)}
						<div class="flex items-center gap-3 py-1 text-xs">
							<span class="font-mono text-amber-900">{job.kind}</span>
							<div class="h-1.5 flex-1 overflow-hidden rounded bg-amber-200">
								<div class="h-full bg-amber-600" style="width: {percent ?? 0}%"></div>
							</div>
							<span class="text-amber-800">{job.detail || job.state}</span>
							<button class="text-amber-700 hover:underline" on:click={() => cancelJob(job.job_id)}>
								cancel
							</button>
						</div>
					{/each}
				</section>
			{/if}

			{#if loading}
				<p class="text-surface-content/50 py-8 text-center text-sm">Loading the capture index…</p>
			{:else if roots.length === 0}
				<div class="border-surface-300 rounded border border-dashed p-8 text-center">
					<h2 class="text-surface-content mb-1 text-lg">No capture volume configured</h2>
					<p class="text-surface-content/60 mx-auto max-w-lg text-sm">
						Capture volumes are set when the server starts, with
						<code class="bg-surface-200 rounded px-1 font-mono text-xs">--lidar-capture-root</code>.
						They are deliberately not addable from here: the safe-directory boundary that stops
						replay reading arbitrary server paths only holds while the set of readable volumes is
						fixed outside the API.
					</p>
				</div>
			{:else}
				<!-- Coverage -->
				<section class="border-surface-300 bg-surface-100 mb-4 rounded border p-3">
					<div class="mb-3 flex items-center justify-between">
						<h2 class="text-surface-content text-sm font-medium">Coverage</h2>
						<MotionLegend compact />
					</div>
					<CoverageTimeline
						sessions={visibleSessions}
						{periodsBySession}
						selectedSessionId={expandedSessionId}
						onSelect={toggleSession}
					/>
				</section>

				<!-- Sessions -->
				<section class="border-surface-300 overflow-hidden rounded border">
					<div
						class="border-surface-300 bg-surface-200 text-surface-content/60 flex items-center gap-3 border-b px-3 py-2 text-xs"
					>
						<span class="flex-1">Session</span>
						<span class="w-40">Window</span>
						<span class="w-16 text-right">Captures</span>
						<span class="w-20 text-right">Size</span>
						<span class="w-24 text-right">Static</span>
						<span class="w-24">Joins</span>
					</div>

					{#if visibleSessions.length === 0}
						<p class="text-surface-content/50 px-3 py-8 text-center text-sm">
							No sessions on this volume yet. Scan and probe to build them from the captures' packet
							clocks.
						</p>
					{/if}

					{#each visibleSessions as s (s.session_id)}
						{@const share = sessionShare(periodsBySession[s.session_id] ?? [])}
						<div class="border-surface-300 border-b last:border-b-0">
							<button
								class="hover:bg-surface-100 flex w-full items-center gap-3 px-3 py-2 text-left text-sm"
								on:click={() => toggleSession(s.session_id)}
							>
								<span class="text-surface-content flex-1 truncate">
									{s.label || s.session_id}
									{#if s.label}<span class="text-surface-content/40 ml-2 font-mono text-xs"
											>{s.session_id}</span
										>{/if}
								</span>
								<span class="text-surface-content/70 w-40 font-mono text-xs">
									{formatDay(s.start_ns)}
									{formatClock(s.start_ns)}–{formatClock(s.end_ns)}
								</span>
								<span class="text-surface-content/70 w-16 text-right text-xs">{s.file_count}</span>
								<span class="text-surface-content/70 w-20 text-right text-xs"
									>{formatSize(s.size_bytes)}</span
								>
								<span class="w-24 text-right text-xs">
									{#if share.staticSecs > 0}
										<span class="text-emerald-700">{formatDuration(share.staticSecs)}</span>
									{:else}
										<span class="text-surface-content/30">—</span>
									{/if}
								</span>
								<span class="w-24 text-xs {seamTone(s.worst_seam)}">
									{s.worst_seam}
									{#if s.lost_ns > 0}
										<span class="text-surface-content/40">{formatGap(s.lost_ns / 1_000_000)}</span>
									{/if}
								</span>
							</button>

							{#if expandedSessionId === s.session_id}
								<div class="border-surface-300 bg-surface-100 border-t px-3 py-3">
									<!-- Session timeline -->
									<div class="mb-3">
										<MotionStrip
											periods={expandedPeriods}
											files={sessionFiles}
											startNs={s.start_ns}
											endNs={s.end_ns}
											height={28}
											showBoundaries
											showClockAxis
										/>
										<div
											class="text-surface-content/50 mt-1 flex justify-between font-mono text-xs"
										>
											<span>{formatClock(s.start_ns, true)}</span>
											<span
												>{formatDuration((s.end_ns - s.start_ns) / 1_000_000_000)} continuous</span
											>
											<span>{formatClock(s.end_ns, true)}</span>
										</div>
										<div class="mt-2"><MotionLegend /></div>
									</div>

									<!-- Session actions -->
									<div class="mb-3 flex flex-wrap items-center gap-2">
										<input
											class="border-surface-300 bg-surface-100 rounded border px-2 py-1 text-xs"
											placeholder="name this site, e.g. broadway_columbus"
											bind:value={labelDraft}
										/>
										<Button size="sm" variant="outline" on:click={saveLabel}>Save name</Button>
										<span class="flex-1"></span>
										{#if sessionJob}
											<span class="text-xs text-amber-700">
												motion pass {sessionJob.state}{sessionJob.detail
													? ` · ${sessionJob.detail}`
													: ''}
											</span>
										{:else}
											<Button size="sm" variant="outline" on:click={queueMotionPass}>
												{expandedPeriods.length > 0 ? 'Re-run motion pass' : 'Run motion pass'}
											</Button>
										{/if}
									</div>

									<!-- Periods, when a pass has run -->
									{#if expandedPeriods.length > 0}
										<div class="mb-3">
											<h3 class="text-surface-content/60 mb-1 text-xs font-medium">
												Periods — {formatDuration(expandedShare.staticSecs)} static of
												{formatDuration(expandedShare.staticSecs + expandedShare.motionSecs)}
											</h3>
											<div class="flex flex-wrap gap-1.5">
												{#each expandedPeriods as p (p.period_id)}
													<button
														class="rounded border px-2 py-1 font-mono text-xs {p.type === 'static'
															? 'border-emerald-300 bg-emerald-50 text-emerald-800 hover:bg-emerald-100'
															: 'border-amber-300 bg-amber-50 text-amber-800 hover:bg-amber-100'}"
														title="{formatClock(p.start_ns, true)} → {formatClock(p.end_ns, true)}"
														on:click={() => selectPeriod(p)}
													>
														{p.label}
														<span class="opacity-60"
															>{formatDuration(p.duration_ns / 1_000_000_000)}</span
														>
													</button>
												{/each}
											</div>
											<p class="text-surface-content/40 mt-1 text-xs">
												Selecting a period picks every capture it covers — which for a static
												stretch is usually more than one.
											</p>
										</div>
									{/if}

									<!-- Captures -->
									{#if filesLoading}
										<p class="text-surface-content/50 py-3 text-xs">Loading captures…</p>
									{:else}
										<div class="border-surface-300 overflow-hidden rounded border">
											{#each sessionFiles as f (f.capture_file_id)}
												<label
													class="border-surface-300 hover:bg-surface-200 flex items-center gap-3 border-b px-2 py-1.5 text-xs last:border-b-0"
												>
													<input
														type="checkbox"
														checked={selectedFileIds.includes(f.capture_file_id)}
														disabled={f.probe_state !== PROBE_OK}
														on:change={() => toggleFile(f.capture_file_id)}
													/>
													<span class="text-surface-content flex-1 truncate font-mono"
														>{f.rel_path}</span
													>
													<span class="text-surface-content/60 w-24 font-mono">
														{f.first_packet_ns ? formatClock(f.first_packet_ns, true) : '—'}
													</span>
													<span class="text-surface-content/60 w-16 text-right">
														{f.first_packet_ns && f.last_packet_ns
															? formatDuration(
																	(f.last_packet_ns - f.first_packet_ns) / 1_000_000_000
																)
															: '—'}
													</span>
													<span class="text-surface-content/60 w-20 text-right"
														>{formatSize(f.size_bytes)}</span
													>
													<span class="w-20 {probeTone(f.probe_state)}">
														{f.probe_state === PROBE_PENDING ? 'not probed' : f.probe_state}
													</span>
													{#if !f.present}
														<span class="text-red-600">missing</span>
													{/if}
												</label>
											{/each}
										</div>
									{/if}

									<!-- Build a case -->
									{#if selectedFileIds.length > 0}
										<div class="border-primary/40 bg-primary/5 mt-3 rounded border p-3">
											<h3 class="text-surface-content mb-2 text-sm font-medium">
												New replay case from {verdict.files.length} capture{verdict.files.length ===
												1
													? ''
													: 's'}
											</h3>

											{#if verdict.seams.length > 0}
												<ul class="mb-2 space-y-0.5 font-mono text-xs">
													{#each verdict.seams as seam (seam.beforePath + seam.afterPath)}
														<li class={seamTone(seam.grade)}>
															{seam.beforePath} → {seam.afterPath}: {seam.grade} ({formatGap(
																seam.gapMs
															)})
														</li>
													{/each}
												</ul>
											{/if}

											{#if verdict.usable}
												<p class="text-surface-content/70 mb-2 text-xs">
													{formatDuration(verdict.coveredSecs)} across {verdict.files.length} capture{verdict
														.files.length === 1
														? ''
														: 's'}{verdict.lostMs > 0
														? `, ${formatGap(verdict.lostMs)} lost at the joins`
														: ''}. No unioned file is written; replay reads the captures in order.
												</p>
												<div class="flex flex-wrap items-center gap-2">
													<input
														class="border-surface-300 bg-surface-100 flex-1 rounded border px-2 py-1 text-xs"
														placeholder="description"
														bind:value={caseDescription}
													/>
													<input
														class="border-surface-300 bg-surface-100 w-48 rounded border px-2 py-1 font-mono text-xs"
														placeholder="sensor id"
														bind:value={caseSensorId}
													/>
													<Button
														size="sm"
														variant="fill"
														color="primary"
														disabled={creating}
														on:click={createCase}
													>
														{creating ? 'Creating…' : 'Create replay case'}
													</Button>
												</div>

												<div class="mt-2 flex flex-wrap items-center gap-2">
													<span class="text-surface-content/50 text-xs">Trim</span>
													<input
														class="border-surface-300 bg-surface-100 w-24 rounded border px-2 py-1 font-mono text-xs"
														placeholder="start (s)"
														bind:value={caseStartSecs}
													/>
													<input
														class="border-surface-300 bg-surface-100 w-24 rounded border px-2 py-1 font-mono text-xs"
														placeholder="duration (s)"
														bind:value={caseDurationSecs}
													/>
													{#if trimmedWindow}
														<span class="text-surface-content/50 font-mono text-xs">
															→ {formatClock(trimmedWindow.startNs, true)}{trimmedWindow.endNs
																? ` – ${formatClock(trimmedWindow.endNs, true)}`
																: ' – end of selection'}
														</span>
													{/if}
													<span class="text-surface-content/40 text-xs">
														optional — seconds into the selection to skip leading motion, and how
														long the case runs
													</span>
												</div>

												<div class="mt-2 flex flex-wrap items-center gap-2">
													<span class="text-surface-content/50 text-xs">Captured at</span>
													<input
														class="border-surface-300 bg-surface-100 w-32 rounded border px-2 py-1 font-mono text-xs"
														placeholder="latitude"
														bind:value={originLat}
													/>
													<input
														class="border-surface-300 bg-surface-100 w-32 rounded border px-2 py-1 font-mono text-xs"
														placeholder="longitude"
														bind:value={originLon}
													/>
													<select
														class="border-surface-300 bg-surface-100 rounded border px-2 py-1 text-xs"
														bind:value={geoSource}
													>
														<option value="operator">entered by hand</option>
														<option value="surveyed">surveyed site origin</option>
														<option value="fix">fix from the capture</option>
													</select>
													<span class="text-surface-content/40 text-xs">
														optional — indexes the case as an S2 site on the scene map
													</span>
												</div>
											{:else}
												<p class="rounded bg-red-50 px-3 py-2 text-xs text-red-600">
													{verdict.reason}
												</p>
											{/if}

											{#if createError}
												<p class="mt-2 rounded bg-red-50 px-3 py-2 text-xs text-red-600">
													{createError}
												</p>
											{/if}
											{#if createdMessage}
												<p class="mt-2 rounded bg-emerald-50 px-3 py-2 text-xs text-emerald-700">
													{createdMessage}
												</p>
											{/if}
											{#if locationNote}
												<p class="mt-2 rounded bg-amber-50 px-3 py-2 text-xs text-amber-700">
													{locationNote}
												</p>
											{/if}
										</div>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</section>
			{/if}
		</div>
	</div>
</main>
