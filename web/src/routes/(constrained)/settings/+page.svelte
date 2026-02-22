<script lang="ts">
	import { resolve } from '$app/paths';
	import {
		getConfig,
		getTransitWorkerState,
		updateTransitWorker,
		type Config,
		type TransitRunInfo,
		type TransitWorkerState
	} from '$lib/api';
	import { displayTimezone, initializeTimezone, updateTimezone } from '$lib/stores/timezone';
	import { displayUnits, initializeUnits, updateUnits } from '$lib/stores/units';
	import { AVAILABLE_TIMEZONES, getTimezoneLabel, type Timezone } from '$lib/timezone';
	import { AVAILABLE_UNITS, getUnitLabel, type Unit } from '$lib/units';
	import { onMount } from 'svelte';
	import { Button, Card, Header, SelectField, Switch, Table } from 'svelte-ux';

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
			console.error('Failed to load configuration:', e);
			message = 'Failed to load configuration';
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
			console.error('Failed to load transit worker state:', e);
			// Silently fail - transit worker might not be available
		}
	}

	async function handleTransitWorkerToggle(enabled: boolean) {
		transitWorkerLoading = true;
		try {
			// Only trigger manual run when enabling, not when disabling
			const response = await updateTransitWorker({ enabled, trigger: enabled });
			transitWorkerEnabled = response.enabled;
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = enabled ? 'Transit worker enabled and run triggered!' : 'Transit worker disabled!';

			// Clear message after a few seconds
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Failed to update transit worker:', e);
			message = 'Failed to update transit worker';
			// Revert the toggle on error
			transitWorkerEnabled = !enabled;
		} finally {
			transitWorkerLoading = false;
		}
	}

	async function handleTransitWorkerRunNow() {
		if (!transitWorkerEnabled) {
			message = 'Enable the transit worker to run it.';
			return;
		}
		transitWorkerLoading = true;
		try {
			const response = await updateTransitWorker({ trigger: true });
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = 'Transit worker run triggered.';
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Failed to trigger transit worker run:', e);
			message = 'Failed to trigger transit worker run';
		} finally {
			transitWorkerLoading = false;
		}
	}

	async function handleTransitWorkerRunFullHistory() {
		if (!transitWorkerEnabled) {
			message = 'Enable the transit worker to run it.';
			return;
		}
		transitWorkerLoading = true;
		try {
			const response = await updateTransitWorker({ trigger_full_history: true });
			transitWorkerStatus = response;
			scheduleTransitWorkerRefresh();
			message = 'Full history run triggered.';
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Failed to trigger full history run:', e);
			message = 'Failed to trigger full history run';
		} finally {
			transitWorkerLoading = false;
		}
	}

	function handleUnitsChange(newUnits: Unit) {
		try {
			updateUnits(newUnits);
			message = 'Units updated automatically!';

			// Clear message after a few seconds
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Failed to update units:', e);
			message = 'Failed to update units';
		}
	}

	function handleTimezoneChange(newTimezone: Timezone) {
		try {
			updateTimezone(newTimezone);
			message = 'Timezone updated automatically!';

			// Clear message after a few seconds
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			console.error('Failed to update timezone:', e);
			message = 'Failed to update timezone';
		}
	}

	onMount(() => {
		loadConfig();
		loadTransitWorkerState();
		transitWorkerRefreshTimer = setInterval(loadTransitWorkerState, 30000);
		return () => {
			stopTransitWorkerRefresh();
		};
	});
</script>

<svelte:head>
	<title>Settings 🚴 velocity.report</title>
	<meta name="description" content="Manage your display preferences for units and timezone" />
</svelte:head>

<div id="main-content" class="space-y-6 p-4">
	<Header title="Settings" subheading="Manage your application settings and preferences." />

	<!-- Navigation to other settings pages -->
	<Card title="Settings Sections">
		<div class="space-y-2 p-4">
			<a
				href={resolve('/settings/serial')}
				class="hover:bg-surface-100 block rounded-lg border p-4 transition-colors"
			>
				<h3 class="font-semibold">Serial Configuration</h3>
				<p class="text-surface-content/70 text-sm">
					Configure and test radar sensor serial port connections
				</p>
			</a>
		</div>
	</Card>

	{#if loading}
		<Card>
			<div class="p-4" role="status" aria-live="polite">
				<p>Loading settings...</p>
			</div>
		</Card>
	{:else}
		<Card title="Display Units">
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

		<Card title="Display Timezone">
			<div class="space-y-4 p-4">
				<p class="text-surface-content/70 text-sm">
					Choose your preferred timezone for displaying timestamps. Changes are saved automatically
					and will override the server default ({getTimezoneLabel(config.timezone as Timezone)}).
					Daylight saving time (DST) is handled automatically.
				</p>

				<SelectField
					label="Timezone"
					bind:value={selectedTimezone}
					options={AVAILABLE_TIMEZONES}
					clearable={false}
				/>
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

		{#if message}
			<Card>
				<div class="p-4" role={message.includes('Failed') ? 'alert' : 'status'} aria-live="polite">
					<p
						class="text-sm"
						class:text-green-600={message.includes('automatically')}
						class:text-red-600={message.includes('Failed')}
					>
						{message}
					</p>
				</div>
			</Card>
		{/if}

		<Card title="Current Configuration">
			<div class="px-4 pb-4">
				<Table
					data={[
						{
							setting: 'Timezone',
							serverDefault: getTimezoneLabel(config.timezone as Timezone),
							yourSetting: getTimezoneLabel($displayTimezone)
						},
						{
							setting: 'Units',
							serverDefault: getUnitLabel(config.units as Unit),
							yourSetting: getUnitLabel($displayUnits)
						}
					]}
					columns={[
						{ name: 'setting', header: '' },
						{ name: 'serverDefault', header: 'Server Default' },
						{ name: 'yourSetting', header: 'Your Setting' }
					]}
				/>
			</div>
		</Card>
	{/if}
</div>
