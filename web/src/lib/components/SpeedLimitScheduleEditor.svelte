<script lang="ts">
	import { mdiPlus, mdiDelete } from '@mdi/js';
	import { Button, Card } from 'svelte-ux';
	import type { SpeedLimitSchedule } from '$lib/api';

	export let siteId: number;
	export let schedules: SpeedLimitSchedule[] = [];
	export let onSchedulesChange: (schedules: SpeedLimitSchedule[]) => void;

	const daysOfWeek = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

	// Generate time options in 5-minute increments
	function generateTimeOptions(): string[] {
		const options: string[] = [];
		for (let hour = 0; hour < 24; hour++) {
			for (let minute = 0; minute < 60; minute += 5) {
				const timeStr = `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`;
				options.push(timeStr);
			}
		}
		return options;
	}

	const timeOptions = generateTimeOptions();

	function addSchedule() {
		const newSchedule: Partial<SpeedLimitSchedule> = {
			site_id: siteId,
			day_of_week: 1, // Default to Monday
			start_time: '06:00',
			end_time: '07:05',
			speed_limit: 15,
			id: -(schedules.length + 1), // Temporary negative ID for new schedules
			created_at: new Date().toISOString(),
			updated_at: new Date().toISOString()
		};
		schedules = [...schedules, newSchedule as SpeedLimitSchedule];
		onSchedulesChange(schedules);
	}

	function removeSchedule(index: number) {
		schedules = schedules.filter((_, i) => i !== index);
		onSchedulesChange(schedules);
	}

	function updateSchedule(index: number, field: keyof SpeedLimitSchedule, value: any) {
		schedules[index] = { ...schedules[index], [field]: value };
		schedules = [...schedules]; // Trigger reactivity
		onSchedulesChange(schedules);
	}
</script>

<Card>
	<div class="space-y-4 p-6">
		<div class="flex items-center justify-between">
			<h3 class="text-lg font-semibold">Speed Limit Schedules</h3>
			<Button on:click={addSchedule} icon={mdiPlus} variant="fill" size="sm">
				Add Schedule
			</Button>
		</div>

		{#if schedules.length === 0}
			<p class="text-sm text-gray-500">
				No speed limit schedules defined. Click "Add Schedule" to create time-based speed limits.
			</p>
		{:else}
			<div class="space-y-3">
				{#each schedules as schedule, index}
					<div class="flex items-center gap-2 rounded border p-3 border-gray-200">
						<div class="grid flex-1 grid-cols-4 gap-2">
							<!-- Day of Week -->
							<div>
								<label for={`day-${index}`} class="text-xs text-gray-600 block">Day</label>
								<select
									id={`day-${index}`}
									bind:value={schedule.day_of_week}
									on:change={() => updateSchedule(index, 'day_of_week', schedule.day_of_week)}
									class="w-full rounded border px-2 py-1 text-sm border-gray-300"
								>
									{#each daysOfWeek as day, dayIndex}
										<option value={dayIndex}>{day}</option>
									{/each}
								</select>
							</div>

							<!-- Start Time -->
							<div>
								<label for={`start-${index}`} class="text-xs text-gray-600 block">Start Time</label>
								<select
									id={`start-${index}`}
									bind:value={schedule.start_time}
									on:change={() => updateSchedule(index, 'start_time', schedule.start_time)}
									class="w-full rounded border px-2 py-1 text-sm border-gray-300"
								>
									{#each timeOptions as time}
										<option value={time}>{time}</option>
									{/each}
								</select>
							</div>

							<!-- End Time -->
							<div>
								<label for={`end-${index}`} class="text-xs text-gray-600 block">End Time</label>
								<select
									id={`end-${index}`}
									bind:value={schedule.end_time}
									on:change={() => updateSchedule(index, 'end_time', schedule.end_time)}
									class="w-full rounded border px-2 py-1 text-sm border-gray-300"
								>
									{#each timeOptions as time}
										<option value={time}>{time}</option>
									{/each}
								</select>
							</div>

							<!-- Speed Limit -->
							<div>
								<label for={`speed-${index}`} class="text-xs text-gray-600 block">Speed Limit</label>
								<input
									id={`speed-${index}`}
									type="number"
									bind:value={schedule.speed_limit}
									on:input={() => updateSchedule(index, 'speed_limit', schedule.speed_limit)}
									min="5"
									max="100"
									class="w-full rounded border px-2 py-1 text-sm border-gray-300"
								/>
							</div>
						</div>

						<!-- Delete Button -->
						<Button
							on:click={() => removeSchedule(index)}
							icon={mdiDelete}
							variant="outline"
							size="sm"
							color="danger"
							title="Delete schedule"
						/>
					</div>
				{/each}
			</div>
		{/if}

		<div class="text-xs text-gray-500 border-t pt-3 border-gray-200">
			<p>
				Define different speed limits for different times of day and days of the week. This is
				useful for school zones or areas with variable speed limits.
			</p>
		</div>
	</div>
</Card>
