<script lang="ts">
	import './app.css';

	import { discord } from '$lib/icons';
	import { mdiGithub } from '@mdi/js';

	import { AppLayout, Tooltip, Button, AppBar, ThemeSelect } from 'svelte-ux';
	import NavMenu from './_NavMenu.svelte';

	let { children } = $props();
</script>

<AppLayout>
	{#snippet nav()}
		<NavMenu />
	{/snippet}

	<AppBar title="velocity.report">
		<div slot="actions" class="flex items-center gap-2">
			<Tooltip title="View repository on Github" placement="left">
				<Button
					icon={mdiGithub}
					href="https://github.com/banshee-data/velocity.report"
					class="p-2"
					target="_blank"
				/>
			</Tooltip>

			<Tooltip title="Chat with us on Discord" placement="left">
				<Button icon={discord} href="https://discord.gg/XXh6jXVFkt" class="p-2" target="_blank" />
			</Tooltip>

			<div class="border-primary-content/20 pr-2">
				<ThemeSelect keyboardShortcuts />
			</div>
		</div>
	</AppBar>

	<main>
		{@render children?.()}
	</main>
</AppLayout>
