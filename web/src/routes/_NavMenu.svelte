<script lang="ts">
	import { NavItem } from 'svelte-ux';
	import { mdiHome, mdiCog } from '@mdi/js';

	import { page } from '$app/state';
</script>

<NavItem text="Dashboard" icon={mdiHome} path="/" currentUrl={page.url} />

<NavItem text="Settings" icon={mdiCog} path="/settings" currentUrl={page.url} />
