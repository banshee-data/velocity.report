<script lang="ts">
	/**
	 * When there are captures, and what is in them.
	 *
	 * One row per day, each session positioned across the span that day's
	 * sessions occupy. The bar is filled with the session's motion timeline
	 * where one has been computed, so the answer to "what do we have?" and the
	 * answer to "how much of it is usable?" are the same picture.
	 */
	import type { CaptureSession, MotionPeriod } from '$lib/types/captures';
	import { coverageRows, formatClock } from '$lib/captures/timeline';
	import MotionStrip from './MotionStrip.svelte';

	export let sessions: CaptureSession[] = [];
	/** Motion periods by session id, for the sessions that have been passed. */
	export let periodsBySession: Record<string, MotionPeriod[]> = {};
	export let selectedSessionId: string | null = null;
	export let onSelect: (sessionId: string) => void = () => {};

	$: rows = coverageRows(sessions);
</script>

{#if rows.length === 0}
	<p class="text-surface-content/50 py-6 text-center text-sm">
		No sessions yet. Scan a capture volume to see what is on it.
	</p>
{:else}
	<div class="flex flex-col gap-3">
		{#each rows as row (row.day)}
			<div class="flex items-center gap-3">
				<span class="text-surface-content/60 w-16 shrink-0 text-right text-xs">{row.dayLabel}</span>
				<div class="bg-surface-100 relative h-7 flex-1 rounded">
					{#each row.bars as bar (bar.session.session_id)}
						<button
							type="button"
							class="absolute top-0.5 bottom-0.5 rounded {selectedSessionId ===
							bar.session.session_id
								? 'ring-primary ring-2'
								: 'hover:ring-surface-content/30 hover:ring-1'}"
							style="left: {bar.left}%; width: {bar.width}%"
							title="{bar.session.label || bar.session.session_id} · {formatClock(
								bar.session.start_ns
							)}–{formatClock(bar.session.end_ns)} · {bar.session.file_count} captures"
							on:click={() => onSelect(bar.session.session_id)}
						>
							<MotionStrip
								periods={periodsBySession[bar.session.session_id] ?? []}
								startNs={bar.session.start_ns}
								endNs={bar.session.end_ns}
								height={22}
							/>
						</button>
					{/each}
				</div>
				<span class="text-surface-content/50 w-28 shrink-0 text-xs">
					{formatClock(row.bars[0].session.start_ns)}–{formatClock(
						row.bars[row.bars.length - 1].session.end_ns
					)}
				</span>
			</div>
		{/each}
	</div>

	<p class="text-surface-content/40 mt-3 text-xs">
		Each row spans the period that day's sessions occupy, not the whole day. Bars are filled with
		the session's motion timeline once a motion pass has run.
	</p>
{/if}
