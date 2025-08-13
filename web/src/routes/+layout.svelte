<script>
	import '../app.css';
	import { AppLayout, AppBar, ThemeInit, ThemeSelect } from 'svelte-ux';
	import favicon from '$lib/assets/favicon.svg';
	import NavMenu from './_NavMenu.svelte';

	/** @type {{ children?: import('svelte').Snippet }} */
	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<ThemeInit />

<AppLayout>
	{#snippet nav()}
		<NavMenu />
	{/snippet}

	<AppBar title="Velocity Report">
		<div class="absolute top-0 left-0 z-10 w-full px-2 py-1 text-right">
			<ThemeSelect />
		</div>
	</AppBar>

	<div class="p-6">
		{@render children?.()}
	</div>
</AppLayout>
