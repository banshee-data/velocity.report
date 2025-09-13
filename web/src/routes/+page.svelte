<script lang="ts">
	import { onMount } from 'svelte';
	import { Card, Grid, Header } from 'svelte-ux';
	import { getConfig, getRadarStats, type Config, type RadarStats } from '../lib/api';
	import { displayUnits, initializeUnits } from '../lib/stores/units';
	import { getUnitLabel, type Unit } from '../lib/units';

	let stats: RadarStats[] = [];
	let config: Config = { units: 'mph', timezone: 'UTC' }; // default
	let totalCount = 0;
	let maxSpeed = 0;
	let loading = true;
	let error = '';

	// Reactive statement to reload data when units change
	$: if ($displayUnits && !loading) {
		loadStats($displayUnits);
	}

	async function loadConfig() {
		try {
			config = await getConfig();
			initializeUnits(config.units);
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load config';
		}
	}

	async function loadStats(units: Unit) {
		try {
			const statsData = await getRadarStats(units);
			stats = statsData;
			totalCount = stats.reduce((sum, s) => sum + (s.Count || 0), 0);
			// If backend returns an empty array, Math.max(...[]) === -Infinity
			// which causes toFixed to throw. Default to 0 when no stats.
			maxSpeed = stats.length > 0 ? Math.max(...stats.map((s) => s.MaxSpeed || 0)) : 0;
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load stats';
		}
	}

	async function loadData() {
		loading = true;
		error = '';
		try {
			await loadConfig();
			await loadStats($displayUnits);
		} finally {
			loading = false;
		}
	}

	onMount(loadData);
</script>

<svelte:head>
	<title>Dashboard 🚴 velocity.report</title>
</svelte:head>

<main class="space-y-6 p-4">
	<Header
		title="Dashboard"
		subheading="Vehicle traffic statistics and analytics over the past 14 days"
	/>

	{#if loading}
		<p>Loading stats…</p>
	{:else if error}
		<p class="text-red-600">{error}</p>
	{:else}
		<Grid autoColumns="18em" gap={8}>
			<Card title="Vehicle Count">
				<div class="pb-4 pl-4 pr-4 pt-0">
					<p class="text-3xl font-bold text-blue-600">{totalCount}</p>
					<p class="text-surface-content/70 text-sm">vehicles detected</p>
				</div>
			</Card>

			<Card title="Max Speed">
				<div class="pb-4 pl-4 pr-4 pt-0">
					<p class="text-3xl font-bold text-green-600">
						{maxSpeed.toFixed(1)}
						{getUnitLabel($displayUnits)}
					</p>
					<p class="text-surface-content/70 text-sm">last 14 days</p>
				</div>
			</Card>
		</Grid>
	{/if}
</main>
