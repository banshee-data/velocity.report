import { clockFollower, startMsOf, type PageClock } from '../sceneClock';

describe('startMsOf', () => {
	it('keeps the millisecond of a start past double precision', () => {
		expect(startMsOf('1764973025928103400')).toBe(1764973025928);
	});

	it('is zero for a missing or malformed start', () => {
		expect(startMsOf(undefined)).toBe(0);
		expect(startMsOf('')).toBe(0);
		expect(startMsOf('12.5')).toBe(0);
	});
});

describe('clockFollower', () => {
	function follow() {
		let t = 0;
		const emitted: PageClock[] = [];
		const read = clockFollower(
			1_000_000,
			(c) => emitted.push(c),
			100,
			() => t
		);
		return {
			emitted,
			at(ms: number, seconds: number, playing = true, rate = 1) {
				t = ms;
				read({ seconds, playing, rate, duration: 60 });
			}
		};
	}

	it('passes the first reading on as absolute milliseconds', () => {
		const f = follow();
		f.at(0, 2.5);
		expect(f.emitted).toEqual([{ timeMs: 1_002_500, playing: true, rate: 1 }]);
	});

	it('holds per-frame readings to one every interval', () => {
		const f = follow();
		for (let ms = 0; ms <= 300; ms += 16) f.at(ms, ms / 1000);
		expect(f.emitted.length).toBe(3);
	});

	it('passes a pause, a rate change and a seek on at once', () => {
		const f = follow();
		f.at(0, 1);
		f.at(10, 1.01, false);
		f.at(20, 1.02, false, 4);
		f.at(30, 40, false, 4);
		expect(f.emitted.map((c) => [c.playing, c.rate, c.timeMs])).toEqual([
			[true, 1, 1_001_000],
			[false, 1, 1_001_010],
			[false, 4, 1_001_020],
			[false, 4, 1_040_000]
		]);
	});
});
