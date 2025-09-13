<script lang="ts">
	import { mdiCog, mdiGithub, mdiHome } from '@mdi/js';
	import {
		AppBar,
		AppLayout,
		Button,
		NavItem,
		ThemeInit,
		ThemeSelect,
		Tooltip,
		settings
	} from 'svelte-ux';

	import { page } from '$app/state';
	import { discord } from '$lib/icons';

	import './app.css';

	let { children } = $props();

	settings({
		components: {
			AppLayout: {
				classes: {
					aside: 'border-r border-surface-300/80',
					nav: 'bg-surface-300/30'
				}
			},
			AppBar: {
				classes:
					'bg-primary text-primary-content shadow-md \
					[text-shadow:1px_1px_2px_var(--color-primary-400)]'
			},
			NavItem: {
				classes: {
					root: 'text-sm text-surface-content/70 pl-6 py-2 \
								hover:bg-surface-100/70 relative',
					active:
						'text-primary bg-surface-100 font-medium shadow-sm z-10\
						before:absolute before:bg-primary before:rounded-full \
						before:w-1 before:h-2/3 before:left-[6px]'
				}
			},
			Table: {
				classes: {
					table: 'w-full',
					thead: '',
					tbody: '',
					th: 'p-3 text-left font-medium text-surface-content/70',
					td: 'p-3'
				}
			}
		}
	});
</script>

<ThemeInit />

<AppLayout>
	<nav slot="nav">
		<NavItem text="Dashboard" icon={mdiHome} path="/app/" currentUrl={page.url} />
		<NavItem text="Settings" icon={mdiCog} path="/app/settings" currentUrl={page.url} />
	</nav>

	<AppBar title="velocity.report">
		<div slot="actions" class="flex items-center gap-2">
			<div class="border-primary-content/40 border-r pr-2">
				<ThemeSelect keyboardShortcuts />
			</div>

			<Tooltip title="Discord" placement="left" offset={2}>
				<Button icon={discord} href="https://discord.gg/XXh6jXVFkt" class="p-2" target="_blank" />
			</Tooltip>

			<Tooltip title="View repo" placement="left" offset={2}>
				<Button
					icon={mdiGithub}
					href="https://github.com/banshee-data/velocity.report"
					class="p-2"
					target="_blank"
				/>
			</Tooltip>
		</div>
	</AppBar>

	{@render children?.()}
</AppLayout>
