/**
 * The scene player's clock, as the Tracks page needs it.
 *
 * The player counts seconds from its first frame; the page's timeline and
 * track list work in absolute milliseconds. A part's header carries the
 * absolute start in nanoseconds as a string, because the value is past a
 * double's integer precision; dividing it as a BigInt keeps the millisecond
 * exact.
 */

/** startMsOf converts a part header's start_ns to Unix milliseconds. */
export function startMsOf(startNs: string | null | undefined): number {
	if (!startNs || !/^-?\d+$/.test(startNs)) return 0;
	return Number(BigInt(startNs) / 1_000_000n);
}

export interface ClockReading {
	seconds: number;
	playing: boolean;
	rate: number;
	duration: number;
}

export interface PageClock {
	/** Absolute Unix milliseconds of the shown frame. */
	timeMs: number;
	playing: boolean;
	rate: number;
}

/**
 * clockFollower turns the player's per-frame readings into page updates at
 * most every intervalMs, since the page recomputes its visible tracks and
 * redraws its timeline on each one. A change of playing or rate is passed on
 * at once, and so is a jump (a seek), so the page never lags a control.
 */
export function clockFollower(
	startMs: number,
	emit: (clock: PageClock) => void,
	intervalMs = 100,
	now: () => number = () => Date.now()
): (reading: ClockReading) => void {
	let lastAt = -Infinity;
	let last: ClockReading | null = null;
	return (reading) => {
		const t = now();
		const changed =
			last === null ||
			reading.playing !== last.playing ||
			reading.rate !== last.rate ||
			Math.abs(reading.seconds - last.seconds) > 1 + (reading.rate * (t - lastAt)) / 1000;
		if (!changed && t - lastAt < intervalMs) return;
		lastAt = t;
		last = reading;
		emit({
			timeMs: startMs + reading.seconds * 1000,
			playing: reading.playing,
			rate: reading.rate
		});
	};
}
