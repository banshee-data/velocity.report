<script lang="ts">
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { Card, Header, SelectField, Switch, Table } from 'svelte-ux';
	import {
		getConfig,
		getTransitWorkerState,
		updateTransitWorker,
		type Config
	} from '../../lib/api';
	import { displayTimezone, initializeTimezone, updateTimezone } from '../../lib/stores/timezone';
	import { displayUnits, initializeUnits, updateUnits } from '../../lib/stores/units';
	import { AVAILABLE_TIMEZONES, getTimezoneLabel, type Timezone } from '../../lib/timezone';
	import { AVAILABLE_UNITS, getUnitLabel, type Unit } from '../../lib/units';

	let config: Config = { units: 'mph', timezone: 'UTC' };
	let selectedUnits: Unit = 'mph';
	let selectedTimezone: Timezone = 'UTC';
	let loading = true;
	let message = '';
	let transitWorkerEnabled = true;
	let transitWorkerLoading = false;

	// Initialize from stores - these will update when the stores change
	$: selectedUnits = $displayUnits;
	$: selectedTimezone = $displayTimezone;

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
	});
</script>

<svelte:head>
	<title>Settings 🚴 velocity.report</title>
	<meta name="description" content="Manage your display preferences for units and timezone" />
</svelte:head>

<main id="main-content" class="space-y-6 p-4">
	<Header title="Settings" subheading="Manage your application settings and preferences." />

	<!-- Navigation to other settings pages -->
	<Card title="Settings Sections">
		<div class="space-y-2 p-4">
			<a
				href={resolve('/settings/serial')}
				class="hover:bg-surface-100 rounded-lg p-4 block border transition-colors"
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
					enabled, it runs hourly in the background. Toggling it on will also trigger an immediate
					run.
				</p>

				<div class="gap-3 flex items-center">
					<Switch
						checked={transitWorkerEnabled}
						disabled={transitWorkerLoading}
						on:change={(e) => handleTransitWorkerToggle((e as CustomEvent).detail.value)}
					/>
					<span class="text-sm">
						{transitWorkerEnabled ? 'Enabled (runs hourly)' : 'Disabled'}
					</span>
				</div>

				{#if transitWorkerLoading}
					<p class="text-surface-content/70 text-xs italic">Updating...</p>
				{/if}
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
</main>
