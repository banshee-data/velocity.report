<script lang="ts">
	import { Card, Grid, Header } from 'svelte-ux';
	import { onMount } from 'svelte';
	import { getRadarStats, type RadarStats } from '../lib/api';

	let stats: RadarStats[] = [];
	let totalCount = 0;
	let maxSpeed = 0;
	let loading = true;
	let error = '';

	async function loadStats() {
		try {
			stats = await getRadarStats();
			totalCount = stats.reduce((sum, s) => sum + (s.Count || 0), 0);
			maxSpeed = Math.max(...stats.map((s) => s.MaxSpeed || 0));
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load stats';
		} finally {
			loading = false;
		}
	}

	onMount(loadStats);
</script>

<svelte:head>
	<title>Dashboard 🚴 velocity.report</title>
</svelte:head>

<div class="space-y-6 p-4">
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
					<p class="text-3xl font-bold text-green-600">{maxSpeed} mph</p>
					<p class="text-surface-content/70 text-sm">last 14 days</p>
				</div>
			</Card>
		</Grid>
	{/if}
</div>
