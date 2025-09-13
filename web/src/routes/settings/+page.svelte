<script lang="ts">
	import { onMount } from 'svelte';
	import { Card, Header, SelectField } from 'svelte-ux';
	import { getConfig, type Config } from '../../lib/api';
	import { displayUnits, initializeUnits, updateUnits } from '../../lib/stores/units';
	import { AVAILABLE_UNITS, getUnitLabel, type Unit } from '../../lib/units';

	let config: Config = { units: 'mph' };
	let selectedUnits: Unit = 'mph';
	let loading = true;
	let message = '';

	// Initialize selectedUnits from the store - this will update when the store changes
	$: selectedUnits = $displayUnits;

	// Auto-save when selection changes (but avoid initial store load triggering this)
	$: if (selectedUnits && selectedUnits !== $displayUnits && !loading) {
		handleUnitsChange(selectedUnits);
	}

	async function loadConfig() {
		try {
			config = await getConfig();
			// Initialize the store with localStorage data and server default
			initializeUnits(config.units);
		} catch (e) {
			message = 'Failed to load configuration';
		} finally {
			loading = false;
		}
	}

	function handleUnitsChange(newUnits: Unit) {
		try {
			updateUnits(newUnits);
			message = 'Units updated!';

			// Clear message after a few seconds
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (e) {
			message = 'Failed to update units';
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

				{#if message}
					<p
						class="text-sm"
						class:text-green-600={message.includes('automatically')}
						class:text-red-600={message.includes('Failed')}
					>
						{message}
					</p>
				{/if}
			</div>
		</Card>

		<Card title="Current Configuration">
			<div class="space-y-2 p-4">
				<div class="flex justify-between">
					<span class="text-surface-content/70">Server Default:</span>
					<span>{getUnitLabel(config.units as Unit)}</span>
				</div>
				<div class="flex justify-between">
					<span class="text-surface-content/70">Your Preference:</span>
					<span>{getUnitLabel($displayUnits)}</span>
				</div>
			</div>
		</Card>
	{/if}
</main>
