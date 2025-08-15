<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { AppLayout, ThemeInit, AppBar, ThemeSelect } from 'svelte-ux';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<ThemeInit />

<AppLayout>
	<svelte:fragment slot="nav">
		<!-- Nav menu -->
	</svelte:fragment>

	<AppBar title="Example">
		<div slot="actions">
			<!-- App actions -->
		</div>
		<ThemeSelect />
	</AppBar>

	<main>
		{@render children?.()}
	</main>
</AppLayout>
