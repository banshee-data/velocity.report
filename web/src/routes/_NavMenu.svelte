<script>
	import { NavItem } from 'svelte-ux';
	import { page } from '$app/state';
</script>

<NavItem text="Dashboard" path="/" currentUrl={page.url} />
<NavItem text="Settings" path="/settings" currentUrl={page.url} />
