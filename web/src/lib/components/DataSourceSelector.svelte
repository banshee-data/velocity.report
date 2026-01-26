<script lang="ts">
	import { ToggleGroup, ToggleOption } from 'svelte-ux';

	export let value: string = 'radar_objects';
</script>

<ToggleGroup bind:value vertical inset>
	<ToggleOption value="radar_objects">Objects</ToggleOption>
	<ToggleOption value="radar_data_transits">Transits</ToggleOption>
</ToggleGroup>
