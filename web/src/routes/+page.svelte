<script>
	import { Card, Grid } from 'svelte-ux';
</script>

<svelte:head>
	<title>Dashboard 🚴 velocity.report</title>
</svelte:head>

<div class="space-y-6">
	<header>
		<h1 class="text-3xl font-bold text-gray-900">Dashboard</h1>
		<p class="text-gray-600">Vehicle traffic statistics and analytics</p>
	</header>

	<Grid autoColumns="18em" gap={8}>
		<Card title="Today's Traffic">
			<p class="text-3xl font-bold text-blue-600">###</p>
			<p class="text-sm text-gray-500">vehicles detected</p>
		</Card>

		<Card title="Max Speed">
			<p class="text-3xl font-bold text-green-600">## mph</p>
			<p class="text-sm text-gray-500">last 24 hours</p>
		</Card>
	</Grid>
</div>
