<script lang="ts">
	/**
	 * LiDAR Status
	 *
	 * The operator's control page for the running pipeline: what it is reading,
	 * a replay of a capture or clip with or without a VRLOG recording, the
	 * tuning parameters, and grid, snapshot and track maintenance. It replaces
	 * the :8081 status page and is the one place a configured replay with
	 * recording is started by hand.
	 */
	import {
		applyLidarParams,
		getLidarParams,
		getLidarClip,
		getLidarClips,
		scanPcapFiles,
		type PcapFileInfo
	} from '#lib/api.js';
	import {
		DEFAULT_LIDAR_SENSOR_ID,
		defaultReplayRequest,
		getLidarStatus,
		getPlaybackStatus,
		getTuningConfigs,
		replayFieldsForClip,
		resumeLive,
		runMaintenance,
		startReplay,
		stopReplay,
		tuningConfigLabel,
		validateReplayRequest,
		withSettleRule,
		type LidarStatus,
		type MaintenanceAction,
		type PlaybackStatus,
		type ReplayStartRequest,
		type TuningConfigInfo
	} from '#lib/lidarControl.js';
	import type { LidarClip } from '#lib/types/lidar.js';
	import { resolve } from '$app/paths';
	import { onDestroy, onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	const POLL_MS = 2000;

	let status = $state<LidarStatus | null>(null);
	let playback = $state<PlaybackStatus | null>(null);
	let statusError = $state<string | null>(null);
	let sensorId = $derived(status?.sensor_id || DEFAULT_LIDAR_SENSOR_ID);

	// Replay form
	let clips = $state<LidarClip[]>([]);
	let configs = $state<TuningConfigInfo[]>([]);
	let configDir = $state('');
	let configsError = $state<string | null>(null);
	let captures = $state<PcapFileInfo[]>([]);
	let clipId = $state('');
	let form = $state<ReplayStartRequest>(defaultReplayRequest());
	let replayMessage = $state<string | null>(null);
	let replayError = $state<string | null>(null);
	let replayBusy = $state(false);

	const chosenConfig = $derived(configs.find((c) => c.path === form.tuning_config));

	// Parameters
	let paramsJSON = $state('');
	let paramsMessage = $state<string | null>(null);
	let paramsError = $state<string | null>(null);

	// Maintenance
	let maintenanceMessage = $state<string | null>(null);
	let maintenanceError = $state<string | null>(null);

	let timer: ReturnType<typeof setInterval> | null = null;

	async function refreshStatus() {
		try {
			const [s, p] = await Promise.all([getLidarStatus(), getPlaybackStatus()]);
			status = s;
			playback = p;
			statusError = null;
		} catch (e) {
			statusError = e instanceof Error ? e.message : String(e);
		}
	}

	async function loadFormOptions() {
		const [clipList, configList, captureList] = await Promise.allSettled([
			getLidarClips(sensorId),
			getTuningConfigs(),
			scanPcapFiles()
		]);
		if (clipList.status === 'fulfilled') clips = clipList.value;
		if (configList.status === 'fulfilled') {
			configs = configList.value.configs;
			configDir = configList.value.config_dir;
			configsError = null;
		} else {
			configsError = configList.reason instanceof Error ? configList.reason.message : null;
		}
		if (captureList.status === 'fulfilled') captures = captureList.value.files;
	}

	async function loadParams() {
		try {
			paramsJSON = JSON.stringify(await getLidarParams(sensorId), null, 2);
			paramsError = null;
		} catch (e) {
			paramsError = e instanceof Error ? e.message : String(e);
		}
	}

	async function chooseClip() {
		form.pcap_files = undefined;
		if (!clipId) return;
		const listed = clips.find((c) => c.clip_id === clipId);
		if (!listed) return;
		// A clip's captures come only with a single-clip fetch.
		try {
			Object.assign(form, replayFieldsForClip(await getLidarClip(clipId)));
		} catch (e) {
			replayError = `Could not load the clip: ${e instanceof Error ? e.message : String(e)}`;
		}
	}

	function onSettleChange() {
		form = withSettleRule(form);
	}

	async function handleStart() {
		replayMessage = null;
		replayError = validateReplayRequest(form);
		if (replayError) return;
		replayBusy = true;
		try {
			const res = await startReplay(sensorId, $state.snapshot(form));
			replayMessage = `Replaying ${res.pcap_file}${res.analysis_mode ? ', recording a VRLOG' : ''}${
				res.tuning_fingerprint ? `, tuning ${res.tuning_fingerprint}` : ''
			}.`;
			await refreshStatus();
		} catch (e) {
			replayError = e instanceof Error ? e.message : String(e);
		} finally {
			replayBusy = false;
		}
	}

	async function handleStop() {
		replayMessage = null;
		replayError = null;
		try {
			await stopReplay(sensorId);
			replayMessage = 'Replay stopped; back on live data with a fresh grid.';
			await refreshStatus();
		} catch (e) {
			replayError = e instanceof Error ? e.message : String(e);
		}
	}

	async function handleResumeLive() {
		replayMessage = null;
		replayError = null;
		try {
			await resumeLive(sensorId);
			replayMessage = 'Back on live data, keeping the grid the replay settled.';
			await refreshStatus();
		} catch (e) {
			replayError = e instanceof Error ? e.message : String(e);
		}
	}

	async function handleApplyParams() {
		paramsMessage = null;
		paramsError = null;
		let parsed: unknown;
		try {
			parsed = JSON.parse(paramsJSON);
		} catch (e) {
			paramsError = `Not valid JSON: ${e instanceof Error ? e.message : String(e)}`;
			return;
		}
		if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
			paramsError = 'Expected a JSON object.';
			return;
		}
		try {
			await applyLidarParams(sensorId, parsed as Record<string, unknown>);
			paramsMessage = 'Parameters applied ✓';
			await loadParams();
		} catch (e) {
			paramsError = e instanceof Error ? e.message : String(e);
		}
	}

	const MAINTENANCE: Array<{ key: MaintenanceAction; label: string; confirm?: string }> = [
		{
			key: 'resetGrid',
			label: 'Reset background grid',
			confirm: 'Reset the background grid? It relearns from the next frames.'
		},
		{ key: 'resetAcceptance', label: 'Reset acceptance metrics' },
		{ key: 'persistBackground', label: 'Save background snapshot' },
		{ key: 'cleanupSnapshots', label: 'Clean up snapshots' },
		{
			key: 'clearTracks',
			label: 'Clear tracks and observations',
			confirm: 'Delete every track and observation for this sensor? This cannot be undone.'
		},
		{
			key: 'clearRuns',
			label: 'Clear analysis runs',
			confirm: 'Delete every analysis run and its tracks for this sensor? This cannot be undone.'
		}
	];

	async function handleMaintenance(key: MaintenanceAction, label: string, question?: string) {
		if (question && !window.confirm(question)) return;
		maintenanceMessage = null;
		maintenanceError = null;
		try {
			await runMaintenance(sensorId, key);
			maintenanceMessage = `${label} ✓`;
		} catch (e) {
			const message = e instanceof Error ? e.message : String(e);
			// The server registers run clearing only behind a feature flag.
			maintenanceError =
				key === 'clearRuns' && /not found/i.test(message)
					? 'Clearing runs is turned off on this server (VELOCITY_REPORT_ENABLE_DESTRUCTIVE_LIDAR_API=1 turns it on).'
					: message;
		}
	}

	function formatRate(v: number | undefined, digits: number): string {
		return v == null ? '—' : v.toFixed(digits);
	}

	onMount(async () => {
		await refreshStatus();
		timer = setInterval(refreshStatus, POLL_MS);
		await Promise.all([loadFormOptions(), loadParams()]);
	});

	onDestroy(() => {
		if (timer) clearInterval(timer);
	});
</script>

<svelte:head>
	<title>LiDAR Status — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">LiDAR Status</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					What the pipeline is reading, replays and recordings, parameters and maintenance.
				</p>
			</div>
			<div class="flex gap-3 text-sm">
				<!-- eslint-disable svelte/no-navigation-without-resolve -->
				<a href={resolve('/lidar/debug')} class="text-primary underline">Debug charts</a>
				<a href={resolve('/lidar/regions')} class="text-primary underline">Regions</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			</div>
		</div>
	</div>

	<div class="flex-1 overflow-y-auto p-6">
		<div class="vr-content-wide space-y-6">
			{#if statusError}
				<div class="rounded bg-red-50 px-4 py-3 text-sm text-red-600">{statusError}</div>
			{/if}

			<div class="vr-stat-grid lg:grid-cols-3!">
				<section class="vr-chart-card">
					<h2 class="text-surface-content mb-2 text-sm font-semibold">Pipeline</h2>
					{#if status}
						<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
							<dt class="text-surface-content/60">Sensor</dt>
							<dd class="font-mono">{status.sensor_id}</dd>
							<dt class="text-surface-content/60">Source</dt>
							<dd>{status.data_source}{status.pcap_file ? ` · ${status.pcap_file}` : ''}</dd>
							<dt class="text-surface-content/60">UDP port</dt>
							<dd>{status.udp_port}</dd>
							<dt class="text-surface-content/60">Forwarding</dt>
							<dd>
								{status.forwarding_enabled
									? `${status.forward_addr}:${status.forward_port}`
									: 'off'}
							</dd>
							<dt class="text-surface-content/60">Uptime</dt>
							<dd>{status.uptime || '—'}</dd>
							<dt class="text-surface-content/60">Tuning</dt>
							<dd class="font-mono text-xs break-all">
								{status.tuning_config || 'server defaults'}{status.tuning_fingerprint
									? ` · ${status.tuning_fingerprint}`
									: ''}
							</dd>
						</dl>
					{:else}
						<p class="text-surface-content/50 text-sm">Loading…</p>
					{/if}
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content mb-2 text-sm font-semibold">Packets</h2>
					{#if status?.stats}
						<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
							<dt class="text-surface-content/60">Data rate</dt>
							<dd>{formatRate(status.stats.MBPerSec, 2)} MB/s</dd>
							<dt class="text-surface-content/60">Packet rate</dt>
							<dd>{formatRate(status.stats.PacketsPerSec, 1)} packets/s</dd>
							{#if status.stats.ParseEnabled}
								<dt class="text-surface-content/60">Point rate</dt>
								<dd>{formatRate(status.stats.PointsPerSec, 0)} points/s</dd>
							{/if}
							<dt class="text-surface-content/60">Dropped</dt>
							<dd>{status.stats.DroppedCount}</dd>
							<dt class="text-surface-content/60">Updated</dt>
							<dd>{new Date(status.stats.Timestamp).toLocaleTimeString()}</dd>
						</dl>
					{:else}
						<p class="text-surface-content/50 text-sm">No packet statistics yet.</p>
					{/if}
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content mb-2 text-sm font-semibold">Replay and recording</h2>
					{#if playback}
						<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
							<dt class="text-surface-content/60">Mode</dt>
							<dd>{playback.mode}</dd>
							<dt class="text-surface-content/60">Replay</dt>
							<dd>
								{playback.replay_active
									? `${playback.replay_pass || 'running'}${
											playback.replay_total_passes > 1
												? ` (of ${playback.replay_total_passes} passes)`
												: ''
										}`
									: 'idle'}
							</dd>
							<dt class="text-surface-content/60">Recording</dt>
							<dd class="break-all">
								{playback.recording
									? `${playback.recording_frames} frames → ${playback.recording_path ?? ''}`
									: 'no'}
							</dd>
							{#if playback.recording_run_id}
								<dt class="text-surface-content/60">Run</dt>
								<dd class="font-mono text-xs">{playback.recording_run_id}</dd>
							{/if}
						</dl>
					{:else}
						<p class="text-surface-content/50 text-sm">Loading…</p>
					{/if}
				</section>
			</div>

			<section class="vr-chart-card">
				<h2 class="text-surface-content mb-1 text-lg font-semibold">Replay a capture</h2>
				<p class="text-surface-content/60 mb-4 text-sm">
					Replays a capture from {status?.pcap_safe_dir || 'the capture directory'} through the pipeline.
					With recording on it creates an analysis run and writes a VRLOG; settling first replays once
					to settle the grid, reloads it, then records from the start. Starting resets the grid and tracks.
				</p>

				<div class="grid gap-4 md:grid-cols-2">
					<label class="vr-field">
						<span>Clip</span>
						<select class="vr-input" bind:value={clipId} onchange={chooseClip}>
							<option value="">(enter a capture by hand)</option>
							{#each clips as clip (clip.clip_id)}
								<option value={clip.clip_id}>
									{clip.description || clip.clip_id}
								</option>
							{/each}
						</select>
					</label>

					<label class="vr-field">
						<span>Tuning config{configDir ? ` (from ${configDir})` : ''}</span>
						<select class="vr-input" bind:value={form.tuning_config}>
							<option value="">(keep the server's current parameters)</option>
							{#each configs as c (c.path)}
								<option value={c.path} disabled={!c.applicable}>{tuningConfigLabel(c)}</option>
							{/each}
						</select>
						{#if chosenConfig}
							<span class="text-surface-content/50">
								Applies {chosenConfig.path} ({chosenConfig.engines}, solid body {chosenConfig.solid_body})
								to the running pipeline before the replay starts; the run records fingerprint {chosenConfig.fingerprint}.
							</span>
						{:else if configsError}
							<span class="text-surface-content/50">{configsError}</span>
						{/if}
					</label>

					<label class="vr-field md:col-span-2">
						<span>Capture file</span>
						<input
							class="vr-input font-mono"
							list="capture-files"
							placeholder="filename.pcapng"
							bind:value={form.pcap_file}
						/>
						<datalist id="capture-files">
							{#each captures as f (f.path)}
								<option value={f.path}></option>
							{/each}
						</datalist>
						{#if form.pcap_files?.length}
							<span class="text-surface-content/50">
								Replays the clip's {form.pcap_files.length} captures in order as one recording.
							</span>
						{/if}
					</label>
				</div>

				<div class="mt-4 flex flex-wrap gap-x-6 gap-y-2 text-sm">
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={form.analysis_mode} />
						Analyse and record a VRLOG
					</label>
					<label class="flex items-center gap-2">
						<input
							type="checkbox"
							bind:checked={form.settle_before_recording}
							onchange={onSettleChange}
						/>
						Settle the grid, reload, and record from the start
					</label>
				</div>

				<div class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
					<label class="vr-field">
						<span>Speed</span>
						<select class="vr-input" bind:value={form.speed_mode}>
							<option value="analysis">Analysis (no pacing)</option>
							<option value="realtime">Realtime (1×)</option>
							<option value="scaled">Scaled ratio</option>
						</select>
					</label>
					<label class="vr-field">
						<span>Speed ratio</span>
						<input
							class="vr-input"
							type="number"
							min="0.01"
							step="0.01"
							disabled={form.speed_mode !== 'scaled'}
							bind:value={form.speed_ratio}
						/>
					</label>
					<label class="vr-field">
						<span>Start (seconds)</span>
						<input
							class="vr-input"
							type="number"
							min="0"
							step="0.1"
							bind:value={form.start_seconds}
						/>
					</label>
					<label class="vr-field">
						<span>Duration (seconds, -1 for all)</span>
						<input class="vr-input" type="number" step="0.1" bind:value={form.duration_seconds} />
					</label>
				</div>

				<details class="mt-4">
					<summary class="text-surface-content/70 cursor-pointer text-sm">Debugging options</summary
					>
					<div class="mt-3 flex flex-wrap gap-x-6 gap-y-2 text-sm">
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={form.enable_debug} /> Foreground debug logging
						</label>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={form.enable_plots} /> Generate grid plots
						</label>
						<label class="flex items-center gap-2">
							<input type="checkbox" bind:checked={form.benchmark_mode} /> Benchmark logging
						</label>
					</div>
					<div class="mt-3 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
						<label class="vr-field">
							<span>Debug ring min</span>
							<input
								class="vr-input"
								type="number"
								min="0"
								max="39"
								bind:value={form.debug_ring_min}
							/>
						</label>
						<label class="vr-field">
							<span>Debug ring max</span>
							<input
								class="vr-input"
								type="number"
								min="0"
								max="39"
								bind:value={form.debug_ring_max}
							/>
						</label>
						<label class="vr-field">
							<span>Debug azimuth min (°)</span>
							<input
								class="vr-input"
								type="number"
								min="0"
								max="360"
								step="0.1"
								bind:value={form.debug_az_min}
							/>
						</label>
						<label class="vr-field">
							<span>Debug azimuth max (°)</span>
							<input
								class="vr-input"
								type="number"
								min="0"
								max="360"
								step="0.1"
								bind:value={form.debug_az_max}
							/>
						</label>
					</div>
				</details>

				<div class="mt-4 flex flex-wrap items-center gap-3">
					<Button
						variant="fill"
						color="primary"
						on:click={handleStart}
						disabled={replayBusy || status?.pcap_in_progress}
					>
						{replayBusy ? 'Starting…' : 'Start replay'}
					</Button>
					<Button variant="outline" on:click={handleStop} disabled={!status?.pcap_in_progress}>
						Stop replay
					</Button>
					<Button variant="outline" on:click={handleResumeLive}>Resume live, keep grid</Button>
					{#if replayMessage}<span class="text-sm text-green-600">{replayMessage}</span>{/if}
					{#if replayError}<span class="text-sm text-red-500">{replayError}</span>{/if}
				</div>
			</section>

			<div class="grid gap-6 lg:grid-cols-2">
				<section class="vr-chart-card">
					<h2 class="text-surface-content mb-1 text-lg font-semibold">Tuning parameters</h2>
					<p class="text-surface-content/60 mb-2 text-sm">
						The running parameters. Edit and apply to change them in place; an experiment arm runs
						from a tuning config file instead.
					</p>
					<textarea
						class="vr-input h-80 w-full font-mono text-xs"
						spellcheck="false"
						bind:value={paramsJSON}></textarea>
					<div class="mt-2 flex items-center gap-3">
						<Button variant="fill" color="primary" size="sm" on:click={handleApplyParams}
							>Apply</Button
						>
						<Button variant="outline" size="sm" on:click={loadParams}>Reload</Button>
						{#if paramsMessage}<span class="text-xs text-green-600">{paramsMessage}</span>{/if}
						{#if paramsError}<span class="text-xs text-red-500">{paramsError}</span>{/if}
					</div>
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content mb-1 text-lg font-semibold">Maintenance</h2>
					<p class="text-surface-content/60 mb-3 text-sm">Grid, snapshot and track housekeeping.</p>
					<div class="flex flex-col items-start gap-2">
						{#each MAINTENANCE as m (m.key)}
							<Button
								variant="outline"
								size="sm"
								color={m.confirm ? 'danger' : 'default'}
								on:click={() => handleMaintenance(m.key, m.label, m.confirm)}
							>
								{m.label}
							</Button>
						{/each}
					</div>
					{#if maintenanceMessage}
						<p class="mt-2 text-xs text-green-600">{maintenanceMessage}</p>
					{/if}
					{#if maintenanceError}
						<p class="mt-2 text-xs text-red-500">{maintenanceError}</p>
					{/if}

					<h3 class="text-surface-content mt-6 mb-1 text-sm font-semibold">JSON endpoints</h3>
					<ul class="text-primary space-y-0.5 text-xs">
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						{#each [['LiDAR status', '/api/lidar/status'], ['Data source', '/api/lidar/data_source'], ['Grid status', '/api/lidar/grid_status'], ['Grid heatmap', '/api/lidar/grid_heatmap'], ['Acceptance metrics', '/api/lidar/acceptance'], ['Latest snapshot', '/api/lidar/snapshot'], ['Snapshots', '/api/lidar/snapshots'], ['Active tracks', '/api/lidar/tracks/active'], ['Track summary', '/api/lidar/tracks/summary'], ['Clusters', '/api/lidar/clusters']] as [label, path] (path)}
							<li>
								<a
									href={`${path}?sensor_id=${encodeURIComponent(sensorId)}`}
									target="_blank"
									rel="noopener noreferrer"
									class="underline">{label}</a
								>
							</li>
						{/each}
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</ul>

					<h3 class="text-surface-content mt-4 mb-1 text-sm font-semibold">
						ASC exports (written on the server)
					</h3>
					<ul class="text-primary space-y-0.5 text-xs">
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						{#each [['Latest background snapshot', '/api/lidar/export_snapshot'], ['Next full frame', '/api/lidar/export_next_frame'], ['Latest foreground', '/api/lidar/export_foreground']] as [label, path] (path)}
							<li>
								<a
									href={`${path}?sensor_id=${encodeURIComponent(sensorId)}`}
									target="_blank"
									rel="noopener noreferrer"
									class="underline">{label}</a
								>
							</li>
						{/each}
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</ul>
				</section>
			</div>
		</div>
	</div>
</main>
