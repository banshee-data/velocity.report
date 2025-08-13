<svelte:head>
	<title>Dashboard 🚴 velocity.report</title>
</svelte:head>

<div class="space-y-6">
	<header>
		<h1 class="text-3xl font-bold text-gray-900">Dashboard</h1>
		<p class="text-gray-600">Vehicle traffic statistics and analytics</p>
	</header>

	<main class="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
		<div class="rounded-lg bg-white p-6 shadow">
			<h2 class="text-lg font-semibold text-gray-900">Today's Traffic</h2>
			<p class="text-3xl font-bold text-blue-600">###</p>
			<p class="text-sm text-gray-500">vehicles detected</p>
		</div>

		<div class="rounded-lg bg-white p-6 shadow">
			<h2 class="text-lg font-semibold text-gray-900">Max Speed</h2>
			<p class="text-3xl font-bold text-green-600">## mph</p>
			<p class="text-sm text-gray-500">last 24 hours</p>
		</div>
	</main>
</div>
