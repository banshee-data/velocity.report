<script lang="ts">
	/**
	 * When there are captures, and what is in them.
	 *
	 * One group per day, each session positioned across the span that day's
	 * sessions occupy. A day has a lane per folder, so the original rolls and
	 * an export of the same hours sit one above the other on a shared axis
	 * rather than on top of each other, and more lanes where a folder's own
	 * sessions overlap. The bar is filled with the session's motion timeline
	 * where one has been computed, so the answer to "what do we have?" and the
	 * answer to "how much of it is usable?" are the same picture.
	 */
	import type { CaptureSession, MotionPeriod } from '#lib/types/captures.js';
	import { coverageRows, formatClock } from '#lib/captures/timeline.js';
	import MotionStrip from './MotionStrip.svelte';

	export let sessions: CaptureSession[] = [];
	/** Motion periods by session id, for the sessions that have been passed. */
	export let periodsBySession: Record<string, MotionPeriod[]> = {};
	export let selectedSessionId: string | null = null;
	export let onSelect: (sessionId: string) => void = () => {};
	/** The folder each session's captures sit in, within the volume; '' at its top level. */
	export let folderBySession: Record<string, string> = {};

	$: rows = coverageRows(sessions, (s) => folderBySession[s.session_id] ?? '');
	// A folder column only earns its width when there is more than one folder.
	$: showFolders = new Set(rows.flatMap((row) => row.lanes.map((lane) => lane.folder))).size > 1;
</script>

{#if rows.length === 0}
	<p class="text-surface-content/50 py-6 text-center text-sm">
		No sessions yet. Scan a capture volume to see what is on it.
	</p>
{:else}
	<div class="flex flex-col gap-3">
		{#each rows as row (row.day)}
			<div class="flex flex-col gap-1">
				{#each row.lanes as lane, i (lane.key)}
					<div class="flex items-center gap-3">
						<span class="text-surface-content/60 w-20 shrink-0 text-right text-xs"
							>{i === 0 ? row.dayLabel : ''}</span
						>
						{#if showFolders}
							<span
								class="text-surface-content/50 w-44 shrink-0 truncate font-mono text-[11px]"
								title={lane.folder || 'the volume’s top level'}
								>{lane.firstOfFolder ? lane.folder || '(top level)' : ''}</span
							>
						{/if}
						<div class="bg-surface-100 relative h-7 flex-1 rounded">
							{#each lane.bars as bar (bar.session.session_id)}
								<button
									type="button"
									class="absolute top-0.5 bottom-0.5 rounded {selectedSessionId ===
									bar.session.session_id
										? 'ring-primary ring-2'
										: 'hover:ring-surface-content/30 hover:ring-1'}"
									style="left: {bar.left}%; width: {bar.width}%"
									title="{bar.session.label || bar.session.session_id} · {lane.folder ||
										'(top level)'} · {formatClock(bar.session.start_ns)}–{formatClock(
										bar.session.end_ns
									)} · {bar.session.file_count} captures"
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
							{formatClock(lane.bars[0].session.start_ns)}–{formatClock(
								lane.bars[lane.bars.length - 1].session.end_ns
							)}
						</span>
					</div>
				{/each}
			</div>
		{/each}
	</div>

	<p class="text-surface-content/40 mt-3 text-xs">
		Each day spans the period its sessions occupy, not the whole day, with a lane per folder so
		copies of the same hours sit one above the other. Bars are filled with the session's motion
		timeline once a motion pass has run.
	</p>
{/if}
