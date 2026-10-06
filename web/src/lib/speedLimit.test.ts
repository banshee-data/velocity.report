import {
	defaultSpeedLimitUnit,
	formatSpeedLimit,
	kphToPosted,
	MAX_SPEED_LIMIT_KPH,
	MIN_SPEED_LIMIT_KPH,
	postedToKph,
	speedLimitError
} from './speedLimit';

describe('speedLimit', () => {
	it('stores a posted limit in km/h', () => {
		expect(postedToKph(30, 'kph')).toBe(30);
		expect(postedToKph(25, 'mph')).toBeCloseTo(40.2336, 10);
	});

	it('reads a stored limit back in the unit it is signed in', () => {
		expect(kphToPosted(postedToKph(25, 'mph'), 'mph')).toBe(25);
		expect(kphToPosted(40.2336, 'mph')).toBe(25);
		expect(kphToPosted(30, 'kph')).toBe(30);
		expect(kphToPosted(postedToKph(15, 'mph'), 'mph')).toBe(15);
	});

	it('formats a limit as signed, or a dash for none', () => {
		expect(formatSpeedLimit({ speed_limit_kph: 40.2336, speed_limit_unit: 'mph' })).toBe('25 mph');
		expect(formatSpeedLimit({ speed_limit_kph: 30, speed_limit_unit: 'kph' })).toBe('30 km/h');
		expect(formatSpeedLimit({ speed_limit_kph: null, speed_limit_unit: null })).toBe('—');
		expect(formatSpeedLimit({})).toBe('—');
		expect(formatSpeedLimit({ speed_limit_kph: 30, speed_limit_unit: 'km/h' })).toBe('—');
	});

	it('starts a new period in the display units', () => {
		expect(defaultSpeedLimitUnit('mph')).toBe('mph');
		expect(defaultSpeedLimitUnit('kmph')).toBe('kph');
		expect(defaultSpeedLimitUnit('kph')).toBe('kph');
		expect(defaultSpeedLimitUnit('mps')).toBe('kph');
	});

	it('accepts no limit and refuses what the server would', () => {
		expect(speedLimitError('', 'mph')).toBeNull();
		expect(speedLimitError('  ', 'kph')).toBeNull();
		expect(speedLimitError('25', 'mph')).toBeNull();
		expect(speedLimitError(String(MAX_SPEED_LIMIT_KPH), 'kph')).toBeNull();
		expect(speedLimitError('0', 'kph')).toMatch(/positive/);
		expect(speedLimitError('-5', 'mph')).toMatch(/positive/);
		expect(speedLimitError('fast', 'mph')).toMatch(/positive/);
		expect(speedLimitError('201', 'kph')).toBe('Speed limit must be at most 200 km/h');
		expect(speedLimitError('130', 'mph')).toBe('Speed limit must be at most 124.2 mph');
	});

	it('states limits the server accepts', () => {
		// 124.3 mph is 200.04 km/h, over the limit; the message must not offer it.
		expect(speedLimitError('124.2', 'mph')).toBeNull();
		expect(speedLimitError('124.3', 'mph')).toBe('Speed limit must be at most 124.2 mph');
		expect(speedLimitError('0.5', 'kph')).toBe('Speed limit must be at least 1 km/h');
		expect(speedLimitError('0.5', 'mph')).toBe('Speed limit must be at least 0.7 mph');
		expect(speedLimitError('0.7', 'mph')).toBeNull();
		expect(speedLimitError(String(MIN_SPEED_LIMIT_KPH), 'kph')).toBeNull();
	});
});
