<script lang="ts">
	/**
	 * What the motion strip's colours mean. Static is the deliverable half;
	 * motion is reserved for the SLAM pipeline and is not replay material.
	 */
	export let compact = false;
</script>

<div class="text-surface-content/60 flex items-center gap-4 text-xs">
	<span class="flex items-center gap-1.5">
		<span class="inline-block h-2 w-3 rounded-sm bg-emerald-500"></span>
		static{#if !compact}<span class="text-surface-content/40"> — replay material</span>{/if}
	</span>
	<span class="flex items-center gap-1.5">
		<span class="inline-block h-2 w-3 rounded-sm bg-amber-500"></span>
		motion{#if !compact}<span class="text-surface-content/40">
				— sensor moving, parked for SLAM</span
			>{/if}
	</span>
	{#if !compact}
		<span class="flex items-center gap-1.5">
			<span class="inline-block h-3 w-px bg-black/30"></span>
			capture boundary
		</span>
	{/if}
</div>
