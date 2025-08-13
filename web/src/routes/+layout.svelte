<script>
	import '../app.css';
	import { AppLayout } from 'svelte-ux';
	import favicon from '$lib/assets/favicon.svg';

	/** @type {{ children?: import('svelte').Snippet }} */
	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<AppLayout>
	{@render children?.()}
</AppLayout>
