<script lang="ts">
	import {
		disableTailscale,
		enableTailscale,
		getConfig,
		getTailscaleStatus,
		getTransitWorkerState,
		updateTransitWorker,
		type Config,
		type TailscaleStatus,
		type TransitRunInfo,
		type TransitWorkerState
	} from '$lib/api';
	import QRCode from 'qrcode';
	import { displayTimezone, initializeTimezone, updateTimezone } from '$lib/stores/timezone';
	import { displayUnits, initializeUnits, updateUnits } from '$lib/stores/units';
	import { AVAILABLE_TIMEZONES, getTimezoneLabel, type Timezone } from '$lib/timezone';
	import { AVAILABLE_UNITS, getUnitLabel, type Unit } from '$lib/units';
	import { onMount } from 'svelte';
	import { Button, Card, Header, SelectField, Switch } from 'svelte-ux';

	let config: Config = { units: 'mph', timezone: 'UTC' };
	let selectedUnits: Unit = 'mph';
	let selectedTimezone: Timezone = 'UTC';
	let loading = true;
	let message = '';
	let transitWorkerEnabled = true;
	let transitWorkerLoading = false;
	let transitWorkerStatus: TransitWorkerState | null = null;
	let currentRun: TransitRunInfo | null = null;
	let resolvedLastRun: TransitRunInfo | null = null;
	let transitWorkerRefreshTimer: ReturnType<typeof setInterval> | null = null;
	let transitWorkerRefreshTimeouts: Array<ReturnType<typeof setTimeout>> = [];

	// Tailscale state.  We poll fast (2s) while a login is in progress so
	// the UI surfaces the auth URL promptly, and slow (30s) once the node
	// is connected — no need to hammer the local API in steady state.
	let tailscaleStatus: TailscaleStatus | null = null;
	// Switch needs a plain (non-derived) `checked` binding — when this
	// is a `$:` reactive expression, svelte-ux's Switch can suppress
	// the `on:change` dispatch because the prop snaps back before the
	// click event finishes propagating, leaving the toggle inert.  We
	// sync it from tailscaleStatus.daemon_running in syncTailscaleState
	// instead.
	let tailscaleEnabled = false;
	let tailscaleConnected = false;
	let tailscaleLoading = false;
	let tailscaleError = '';
	let tailscaleQrDataUrl = '';
	let lastQrUrl = '';
	let tailscalePollTimer: ReturnType<typeof setInterval> | null = null;
	let tailscalePollFastInterval = false;

	function formatTimestamp(value?: string) {
		if (!value || value.startsWith('0001-01-01')) {
			return 'N/A';
		}
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) {
			return 'N/A';
		}
		try {
			return new Intl.DateTimeFormat('en-GB', {
				timeZone: $displayTimezone,
				dateStyle: 'medium',
				timeStyle: 'medium'
			}).format(date);
		} catch {
			return date.toLocaleString();
		}
	}

	function formatDuration(durationMs?: number) {
		if (durationMs === undefined || durationMs === null) {
			return 'N/A';
		}
		const totalSeconds = Math.max(0, Math.round(durationMs / 1000));
		const minutes = Math.floor(totalSeconds / 60);
		const seconds = totalSeconds % 60;
		if (minutes > 0) {
			return `${minutes}m ${seconds}s`;
		}
		return `${seconds}s`;
	}

	function formatTrigger(trigger?: string) {
		if (!trigger) {
			return 'Unknown';
		}
		switch (trigger) {
			case 'initial':
				return 'Initial';
			case 'periodic':
				return 'Scheduled';
			case 'manual':
				return 'Manual';
			case 'full-history':
				return 'Full history';
			default:
				return trigger;
		}
	}

	function getRunError(run?: TransitRunInfo | null) {
		return run?.error || undefined;
	}

	function resolveLastRun(status: TransitWorkerState | null): TransitRunInfo | null {
		if (!status) {
			return null;
		}
		if (status.last_run) {
			return status.last_run;
		}
		if (status.last_run_at && !status.last_run_at.startsWith('0001-01-01')) {
			return {
				started_at: status.last_run_at,
				finished_at: status.last_run_at,
				error: status.last_run_error
			};
		}
		return null;
	}

	function scheduleTransitWorkerRefresh() {
		transitWorkerRefreshTimeouts.forEach((timeoutId) => clearTimeout(timeoutId));
		transitWorkerRefreshTimeouts = [];
		transitWorkerRefreshTimeouts.push(setTimeout(loadTransitWorkerState, 1200));
		transitWorkerRefreshTimeouts.push(setTimeout(loadTransitWorkerState, 5000));
	}

	function stopTransitWorkerRefresh() {
		if (transitWorkerRefreshTimer) {
			clearInterval(transitWorkerRefreshTimer);
			transitWorkerRefreshTimer = null;
		}
		transitWorkerRefreshTimeouts.forEach((timeoutId) => clearTimeout(timeoutId));
		transitWorkerRefreshTimeouts = [];
	}

	// Initialize from stores - these will update when the stores change
	$: selectedUnits = $displayUnits;
	$: selectedTimezone = $displayTimezone;
	$: currentRun = transitWorkerStatus?.current_run ?? null;
	$: resolvedLastRun = resolveLastRun(transitWorkerStatus);

	// Auto-save when selections change (but avoid initial store load triggering this)
	$: if (selectedUnits && selectedUnits !== $displayUnits && !loading) {
		handleUnitsChange(selectedUnits);
	}
	$: if (selectedTimezone && selectedTimezone !== $displayTimezone && !loading) {
		handleTimezoneChange(selectedTimezone);
	}

	async function loadConfig() {
		try {
			config = await getConfig();
			// Initialize both stores with localStorage data and server defaults
			initializeUnits(config.units);
			initializeTimezone(config.timezone);
		} catch (e) {
			console.error('Could not load configuration:', e);
			message = 'Could not load the configuration. Check the server is running.';
		} finally {
			loading = false;
		}
	}

	async function loadTransitWorkerState() {
		try {
			const state = await getTransitWorkerState();
			transitWorkerEnabled = state.enabled;
			transitWorkerStatus = state;
		} catch (e) {
			console.error('Could not load transit worker state:', e);
			// Silently fail - transit worker might not be available
		}
	}

	async function loadTailscaleStatus() {
		// Skip background polls while the user is toggling Tailscale on or
		// off — disable in particular takes several seconds (tailscale
		// logout + systemctl stop + mask) and a poll mid-flight can roll
		// the Switch's `checked` state back to true before the action
		// completes, which the user sees as the toggle "snapping back".
		if (tailscaleLoading) return;
		try {
			const next = await getTailscaleStatus();
			tailscaleStatus = next;
			syncTailscaleState(next);
			tuneTailscalePolling(next);
			updateTailscaleQr(next.login_url);
		} catch (e) {
			console.error('Could not load Tailscale status:', e);
		}
	}

	function tuneTailscalePolling(status: TailscaleStatus | null) {
		// Fast cadence while we're waiting for the user to complete login
		// (the login URL only appears once tailscaled emits BrowseToURL on
		// the IPN bus, and we want it on screen quickly).  Otherwise back
		// off to a slow poll that just refreshes peer count etc.
		const wantsFast =
			!!status &&
			(status.login_in_progress ||
				!!status.login_url ||
				status.backend_state === 'NeedsLogin' ||
				status.backend_state === 'Starting');
		if (wantsFast === tailscalePollFastInterval && tailscalePollTimer) return;
		tailscalePollFastInterval = wantsFast;
		if (tailscalePollTimer) clearInterval(tailscalePollTimer);
		tailscalePollTimer = setInterval(loadTailscaleStatus, wantsFast ? 2000 : 30000);
	}

	async function handleTailscaleToggle(enabled: boolean) {
		tailscaleLoading = true;
		tailscaleError = '';
		try {
			tailscaleStatus = enabled ? await enableTailscale() : await disableTailscale();
			syncTailscaleState(tailscaleStatus);
			tuneTailscalePolling(tailscaleStatus);
			updateTailscaleQr(tailscaleStatus.login_url);
		} catch (e) {
			console.error('Could not update Tailscale:', e);
			tailscaleError = (e as Error).message || 'Could not update Tailscale.';
		} finally {
			tailscaleLoading = false;
		}
	}

	async function copyLoginUrl() {
		if (!tailscaleStatus?.login_url) return;
		try {
			await navigator.clipboard.writeText(tailscaleStatus.login_url);
			message = 'Login URL copied to clipboard.';
			setTimeout(() => (message = ''), 3000);
		} catch (e) {
			console.error('clipboard write failed:', e);
		}
	}

	// Render the QR code only when the login URL actually changes — done
	// imperatively from updateTailscaleQr() rather than a reactive block
	// because writing tailscaleQrDataUrl would otherwise re-fire the
	// statement and trip the infinite-reactive-loop lint.
	async function updateTailscaleQr(url: string | undefined) {
		if (!url) {
			tailscaleQrDataUrl = '';
			lastQrUrl = '';
			return;
		}
		if (url === lastQrUrl) return;
		lastQrUrl = url;
		try {
			tailscaleQrDataUrl = await QRCode.toDataURL(url, { width: 220, margin: 1 });
		} catch (e) {
			console.error('QR encode failed:', e);
			tailscaleQrDataUrl = '';
		}
	}

	function syncTailscaleState(status: TailscaleStatus | null) {
		tailscaleEnabled = !!status?.daemon_running;
		tailscaleConnected = status?.backend_state === 'Running';
	}

	async function handleTransitWorkerToggle(enabled: boolean) {
		transitWorkerLoading = true;
		try {
			// Only trigger manual run when enabling, not when disabling
			const response = await updateTransitWorker({ enabled, trigger: enabled });
			transitWorkerEnabled = response.enabled;
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = enabled ? 'Transit worker enabled: run started.' : 'Transit worker disabled.';

			// Clear message after a few seconds
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Could not update transit worker:', e);
			message = 'Could not update the transit worker. Try again shortly.';
			// Revert the toggle on error
			transitWorkerEnabled = !enabled;
		} finally {
			transitWorkerLoading = false;
		}
	}

	async function handleTransitWorkerRunNow() {
		if (!transitWorkerEnabled) {
			message = 'Enable the transit worker first.';
			return;
		}
		transitWorkerLoading = true;
		try {
			const response = await updateTransitWorker({ trigger: true });
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = 'Transit worker run started.';
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Could not trigger transit worker run:', e);
			message = 'Could not start the transit worker run.';
		} finally {
			transitWorkerLoading = false;
		}
	}

	async function handleTransitWorkerRunFullHistory() {
		if (!transitWorkerEnabled) {
			message = 'Enable the transit worker first.';
			return;
		}
		transitWorkerLoading = true;
		try {
			const response = await updateTransitWorker({ trigger_full_history: true });
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = 'Full history reprocessing started.';
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Could not start full history run:', e);
			message = 'Could not start full history reprocessing.';
		} finally {
			transitWorkerLoading = false;
		}
	}

	function handleUnitsChange(newUnits: Unit) {
		try {
			updateUnits(newUnits);
			message = 'Units updated.';

			// Clear message after a few seconds
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Could not update units:', e);
			message = 'Could not save the units change.';
		}
	}

	function handleTimezoneChange(newTimezone: Timezone) {
		try {
			updateTimezone(newTimezone);
			message = 'Timezone updated.';

			// Clear message after a few seconds
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Could not update timezone:', e);
			message = 'Could not save the timezone change.';
		}
	}

	onMount(() => {
		loadConfig();
		loadTransitWorkerState();
		loadTailscaleStatus();
		transitWorkerRefreshTimer = setInterval(loadTransitWorkerState, 30000);
		return () => {
			stopTransitWorkerRefresh();
			if (tailscalePollTimer) {
				clearInterval(tailscalePollTimer);
				tailscalePollTimer = null;
			}
		};
	});
</script>

<svelte:head>
	<title>Settings 🚴 velocity.report</title>
	<meta name="description" content="Manage your display preferences for units and timezone" />
</svelte:head>

<div id="main-content" class="space-y-6 p-4">
	<Header title="Settings" subheading="Display units, timezone, and transit worker" />

	{#if loading}
		<Card>
			<div class="p-4" role="status" aria-live="polite">
				<p>Loading settings...</p>
			</div>
		</Card>
	{:else}
		<div class="grid grid-cols-1 items-stretch gap-6 md:grid-cols-2">
			<Card title="Display Units" class="h-full">
				<div class="space-y-4 p-4">
					<p class="text-surface-content/70 text-sm">
						Choose your preferred units for displaying speed values. Changes are saved automatically
						and will override the server default ({getUnitLabel(config.units as Unit)}).
					</p>

					<SelectField
						label="Speed Units"
						bind:value={selectedUnits}
						options={AVAILABLE_UNITS}
						clearable={false}
					/>
				</div>
			</Card>

			<Card title="Display Timezone" class="h-full">
				<div class="space-y-4 p-4">
					<p class="text-surface-content/70 text-sm">
						Choose your preferred timezone for displaying timestamps. Changes are saved
						automatically and will override the server default ({getTimezoneLabel(
							config.timezone as Timezone
						)}).
					</p>

					<SelectField
						label="Timezone"
						bind:value={selectedTimezone}
						options={AVAILABLE_TIMEZONES}
						clearable={false}
					/>
				</div>
			</Card>
		</div>

		{#if message}
			<Card>
				<div
					class="p-4"
					role={message.includes('Could not') ? 'alert' : 'status'}
					aria-live="polite"
				>
					<p
						class="text-sm"
						class:text-green-600={!message.includes('Could not')}
						class:text-red-600={message.includes('Could not')}
					>
						{message}
					</p>
				</div>
			</Card>
		{/if}

		<Card title="Current Configuration">
			<div class="grid grid-cols-1 gap-6 px-4 pb-4 md:grid-cols-2">
				<div>
					<p class="text-surface-content/60 mb-2 text-xs font-medium tracking-wide uppercase">
						Display Units
					</p>
					<div class="text-sm">
						<div class="flex justify-between gap-4 py-1">
							<span class="text-surface-content/60">Server default</span>
							<span>{getUnitLabel(config.units as Unit)}</span>
						</div>
						<div class="flex justify-between gap-4 py-1">
							<span class="text-surface-content/60">Your setting</span>
							<span class="font-medium">{getUnitLabel($displayUnits)}</span>
						</div>
					</div>
				</div>
				<div>
					<p class="text-surface-content/60 mb-2 text-xs font-medium tracking-wide uppercase">
						Display Timezone
					</p>
					<div class="text-sm">
						<div class="flex justify-between gap-4 py-1">
							<span class="text-surface-content/60">Server default</span>
							<span>{getTimezoneLabel(config.timezone as Timezone)}</span>
						</div>
						<div class="flex justify-between gap-4 py-1">
							<span class="text-surface-content/60">Your setting</span>
							<span class="font-medium">{getTimezoneLabel($displayTimezone)}</span>
						</div>
					</div>
				</div>
			</div>
		</Card>

		<Card title="Transit Worker">
			<div class="space-y-4 p-4">
				<p class="text-surface-content/70 text-sm">
					The transit worker periodically processes raw radar data into vehicle transits. When
					enabled, it runs on a schedule configured by the server. Toggling it on will also trigger
					an immediate run.
				</p>

				<div class="flex flex-wrap items-center gap-3">
					<Switch
						checked={transitWorkerEnabled}
						disabled={transitWorkerLoading}
						on:change={(e) => handleTransitWorkerToggle((e as CustomEvent).detail.value)}
					/>
					<span class="text-sm">
						{transitWorkerEnabled ? 'Enabled (runs on schedule)' : 'Disabled'}
					</span>
					<div class="flex flex-wrap items-center gap-2">
						<Button
							variant="outline"
							on:click={handleTransitWorkerRunNow}
							disabled={transitWorkerLoading || !transitWorkerEnabled || !!currentRun}
						>
							Run now
						</Button>
						<Button
							variant="outline"
							on:click={handleTransitWorkerRunFullHistory}
							disabled={transitWorkerLoading || !transitWorkerEnabled || !!currentRun}
						>
							Run full history
						</Button>
					</div>
				</div>

				{#if transitWorkerLoading}
					<p class="text-surface-content/70 text-xs italic">Updating...</p>
				{/if}

				<div class="grid gap-4 text-sm md:grid-cols-2">
					<div class="border-surface-content/20 bg-surface-100 rounded border p-3">
						<p class="text-surface-content/60 text-xs uppercase">Current Run</p>
						<p class="mt-1 font-medium">
							{transitWorkerStatus ? (currentRun ? 'Running' : 'Idle') : 'Unknown'}
						</p>
						<p class="text-surface-content/70 text-xs">
							Started: {formatTimestamp(currentRun?.started_at)}
						</p>
						<p class="text-surface-content/70 text-xs">
							Trigger: {formatTrigger(currentRun?.trigger)}
						</p>
					</div>

					<div class="border-surface-content/20 bg-surface-100 rounded border p-3">
						<p class="text-surface-content/60 text-xs uppercase">Most Recent Run</p>
						{#if !transitWorkerStatus}
							<p class="mt-1 font-medium">Unknown</p>
						{:else if resolvedLastRun}
							<p class="mt-1 font-medium">
								{getRunError(resolvedLastRun) ? 'Failed' : 'Completed'}
							</p>
						{:else}
							<p class="mt-1 font-medium">No runs yet</p>
						{/if}
						<p class="text-surface-content/70 text-xs">
							Finished: {formatTimestamp(resolvedLastRun?.finished_at)}
						</p>
						<p class="text-surface-content/70 text-xs">
							Duration: {formatDuration(resolvedLastRun?.duration_ms)}
						</p>
						<p class="text-surface-content/70 text-xs">
							Trigger: {formatTrigger(resolvedLastRun?.trigger)}
						</p>
						{#if getRunError(resolvedLastRun)}
							<p class="text-xs text-red-600">
								Error: {getRunError(resolvedLastRun)}
							</p>
						{/if}
					</div>
				</div>
			</div>
		</Card>

		<Card title="Tailscale">
			<div class="space-y-4 p-4">
				<p class="text-surface-content/70 text-sm">
					Tailscale puts this device on your private tailnet so you can reach the web UI from
					anywhere and SSH in without opening any ports on your LAN. When enabled, the device phones
					home to Tailscale's coordination server; when disabled, it does not. Tailscale SSH and
					publishing the web UI on <code>https://&lt;hostname&gt;.&lt;tailnet&gt;.ts.net</code> are turned
					on automatically.
				</p>

				<div class="flex flex-wrap items-center gap-3">
					<Switch
						checked={tailscaleEnabled}
						disabled={tailscaleLoading}
						on:change={(e) => {
							const target = e.target as HTMLInputElement | null;
							if (!target) return;
							handleTailscaleToggle(target.checked);
						}}
					/>
					<span class="text-sm">
						{#if !tailscaleStatus}
							Loading...
						{:else if tailscaleConnected}
							Connected
						{:else if tailscaleEnabled}
							{tailscaleStatus.backend_state || 'Starting'}
						{:else}
							Disabled
						{/if}
					</span>
					{#if tailscaleLoading}
						<span class="text-surface-content/70 text-xs italic">Updating...</span>
					{/if}
				</div>

				{#if tailscaleError}
					<p class="text-xs text-red-600" role="alert">{tailscaleError}</p>
				{/if}

				{#if tailscaleEnabled && !tailscaleConnected && tailscaleStatus?.login_url}
					<div
						class="border-surface-content/20 bg-surface-100 grid gap-4 rounded border p-4 md:grid-cols-[auto_1fr]"
					>
						{#if tailscaleQrDataUrl}
							<img
								src={tailscaleQrDataUrl}
								alt="Tailscale login QR code"
								class="bg-white p-2"
								width="220"
								height="220"
							/>
						{/if}
						<div class="space-y-2">
							<p class="text-sm font-medium">Finish enrolment</p>
							<p class="text-surface-content/70 text-xs">
								Open the link below or scan the QR code on your phone. After signing in, this device
								joins your tailnet and the page will update on its own.
							</p>
							<!-- eslint-disable svelte/no-navigation-without-resolve -->
							<a
								href={tailscaleStatus.login_url}
								target="_blank"
								rel="noopener noreferrer"
								class="block text-sm break-all text-blue-600 underline"
							>
								{tailscaleStatus.login_url}
							</a>
							<Button variant="outline" on:click={copyLoginUrl}>Copy login URL</Button>
						</div>
					</div>
				{:else if tailscaleEnabled && !tailscaleConnected}
					<p class="text-surface-content/70 text-xs italic">
						Waiting for Tailscale to issue a login URL...
					</p>
				{/if}

				{#if tailscaleConnected}
					<div class="grid gap-4 text-sm md:grid-cols-2">
						<div class="border-surface-content/20 bg-surface-100 rounded border p-3">
							<p class="text-surface-content/60 text-xs uppercase">MagicDNS</p>
							<p class="mt-1 font-medium break-all">
								{tailscaleStatus?.magic_dns || tailscaleStatus?.hostname || 'unknown'}
							</p>
							{#if tailscaleStatus?.magic_dns}
								<!-- eslint-disable svelte/no-navigation-without-resolve -->
								<a
									href={`https://${tailscaleStatus.magic_dns}`}
									target="_blank"
									rel="noopener noreferrer"
									class="text-xs text-blue-600 underline"
								>
									Open web UI on tailnet
								</a>
							{/if}
						</div>
						<div class="border-surface-content/20 bg-surface-100 rounded border p-3">
							<p class="text-surface-content/60 text-xs uppercase">Tailnet</p>
							<p class="mt-1 font-medium">{tailscaleStatus?.tailnet_name || 'unknown'}</p>
							<p class="text-surface-content/70 text-xs">
								{tailscaleStatus?.peer_count ?? 0} peer(s) visible
							</p>
						</div>
					</div>
				{/if}
			</div>
		</Card>
	{/if}
</div>
