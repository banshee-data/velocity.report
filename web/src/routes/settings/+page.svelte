<script lang="ts">
	import { onMount } from 'svelte';
	import { Card, Header, SelectField, Table } from 'svelte-ux';
	import { getConfig, type Config } from '../../lib/api';
	import { displayTimezone, initializeTimezone, updateTimezone } from '../../lib/stores/timezone';
	import { displayUnits, initializeUnits, updateUnits } from '../../lib/stores/units';
	import { AVAILABLE_TIMEZONES, getTimezoneLabel, type Timezone } from '../../lib/timezone';
	import { AVAILABLE_UNITS, getUnitLabel, type Unit } from '../../lib/units';

	let config: Config = { units: 'mph', timezone: 'UTC' };
	let selectedUnits: Unit = 'mph';
	let selectedTimezone: Timezone = 'UTC';
	let loading = true;
	let message = '';

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
			message = 'Failed to load configuration';
		} finally {
			loading = false;
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
			message = 'Failed to update timezone';
		}
	}

	onMount(loadConfig);
</script>

<svelte:head>
	<title>Settings 🚴 velocity.report</title>
</svelte:head>

<main class="space-y-6 p-4">
	<Header title="Settings" subheading="Manage your application settings and preferences." />

	{#if loading}
		<Card>
			<div class="p-4">
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

		{#if message}
			<Card>
				<div class="p-4">
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
