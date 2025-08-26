<script lang="ts">
	import './app.css';

	import { NavItem, AppLayout, Tooltip, Button, AppBar, ThemeSelect, ThemeInit } from 'svelte-ux';
	import { mdiGithub, mdiHome, mdiCog } from '@mdi/js';

	import { discord } from '$lib/icons';
	import { page } from '$app/state';

	let { children } = $props();
</script>

<ThemeInit />

<AppLayout>
	<nav slot="nav">
		<NavItem text="Dashboard" icon={mdiHome} path="/" currentUrl={page.url} />
		<NavItem text="Settings" icon={mdiCog} path="/settings" currentUrl={page.url} />
	</nav>

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
