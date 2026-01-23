<script lang="ts">
	let { children } = $props();
</script>

<main>
	<div class="mx-auto max-w-[780px]">
		{@render children?.()}
	</div>
</main>
