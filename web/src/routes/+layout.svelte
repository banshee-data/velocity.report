<script lang="ts">
	import {
		mdiCog,
		mdiFileDocument,
		mdiGithub,
		mdiHome,
		mdiMapMarker,
		mdiMapMarkerPath,
		mdiMovieOpen
	} from '@mdi/js';
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
					aside:
						'border-r border-surface-300/80 top-[var(--headerHeight)] h-[calc(100%-var(--headerHeight))] md:top-0',
					nav: 'bg-surface-100'
				}
			},
			AppBar: {
				classes:
					'bg-primary text-primary-content shadow-md z-[60] \
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

<a
	href="#main-content"
	class="focus:bg-primary focus:text-primary-content sr-only focus:not-sr-only focus:absolute focus:z-50 focus:p-4"
>
	Skip to main content
</a>

<AppLayout>
	<nav slot="nav">
		<NavItem text="Dashboard" icon={mdiHome} path="/app/" currentUrl={page.url} />
		<NavItem text="Sites" icon={mdiMapMarker} path="/app/site" currentUrl={page.url} />
		<NavItem text="Reports" icon={mdiFileDocument} path="/app/reports" currentUrl={page.url} />
		<NavItem
			text="Lidar Tracks"
			icon={mdiMapMarkerPath}
			path="/app/lidar/tracks"
			currentUrl={page.url}
		/>
		<NavItem
			text="Lidar Scenes"
			icon={mdiMovieOpen}
			path="/app/lidar/scenes"
			currentUrl={page.url}
		/>
		<NavItem text="Settings" icon={mdiCog} path="/app/settings" currentUrl={page.url} />
	</nav>

	<AppBar title="velocity.report">
		<div slot="actions" class="flex items-center gap-2">
			<div class="border-primary-content/40 border-r pr-2">
				<ThemeSelect keyboardShortcuts />
			</div>

			<Tooltip title="Discord" placement="left" offset={2}>
				<Button
					icon={discord}
					href="https://discord.gg/XXh6jXVFkt"
					class="p-2"
					target="_blank"
					rel="noopener noreferrer"
				/>
			</Tooltip>

			<Tooltip title="View repo" placement="left" offset={2}>
				<Button
					icon={mdiGithub}
					href="https://github.com/banshee-data/velocity.report"
					class="p-2"
					target="_blank"
					rel="noopener noreferrer"
				/>
			</Tooltip>
		</div>
	</AppBar>

	{@render children?.()}
</AppLayout>
