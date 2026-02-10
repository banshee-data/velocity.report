/* eslint-disable @typescript-eslint/no-require-imports -- CommonJS module requires require() */
const {
	regionColors,
	cellAtPixel,
	escapeHTML,
	loadRegions,
	updateInfoPanel,
	drawRegions,
	updateLegend,
	selectRegion,
	drawRegionOutline,
	autoRefresh,
	init
} = require('@monitor/assets/regions_dashboard.js') as {
	regionColors: string[];
	cellAtPixel: (
		x: number,
		y: number,
		w: number,
		h: number
	) => { ring: number; azBin: number; cellIdx: number } | null;
	escapeHTML: (str: unknown) => string;
	loadRegions: () => void;
	updateInfoPanel: (data: Record<string, unknown>) => void;
	drawRegions: (data: Record<string, unknown>) => void;
	updateLegend: (data: Record<string, unknown>) => void;
	selectRegion: (regionId: number) => void;
	drawRegionOutline: (regionId: number) => void;
	autoRefresh: () => void;
	init: () => void;
};
/* eslint-enable @typescript-eslint/no-require-imports */

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Drain the microtask queue so chained .then() callbacks can execute. */
async function flushPromises(): Promise<void> {
	for (let i = 0; i < 10; i++) await Promise.resolve();
}

/** Create a fresh mock 2-D canvas context. */
function createMockCtx() {
	return {
		fillStyle: '' as string,
		strokeStyle: '' as string,
		lineWidth: 0,
		font: '',
		globalAlpha: 1,
		textAlign: '' as string,
		fillRect: jest.fn(),
		strokeRect: jest.fn(),
		clearRect: jest.fn(),
		fillText: jest.fn(),
		beginPath: jest.fn(),
		moveTo: jest.fn(),
		lineTo: jest.fn(),
		stroke: jest.fn(),
		save: jest.fn(),
		restore: jest.fn(),
		setLineDash: jest.fn()
	};
}

type MockCtx = ReturnType<typeof createMockCtx>;
let mockCtx: MockCtx;

/** Install the standard DOM fixture with all elements needed by the dashboard. */
function setupDOM(): void {
	document.head.innerHTML = '<meta name="sensor-id" content="test-sensor" />';
	document.body.innerHTML = [
		'<canvas id="regionCanvas" width="1200" height="600"></canvas>',
		'<div id="tooltip"></div>',
		'<div id="sensorId"></div>',
		'<div id="regionCount"></div>',
		'<div id="framesSampled"></div>',
		'<span id="status"></span>',
		'<div id="legendItems"></div>'
	].join('\n');

	mockCtx = createMockCtx();
	const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
	canvasEl.getContext = jest.fn().mockReturnValue(mockCtx);
}

/** Build a default API response object, merging optional overrides. */
function makeSampleData(overrides: Record<string, unknown> = {}): Record<string, unknown> {
	const gridMapping = new Array(72000).fill(-1);
	gridMapping[0] = 0; // ring 0, azBin 0
	gridMapping[1] = 0; // ring 0, azBin 1
	gridMapping[1800] = 1; // ring 1, azBin 0

	return {
		sensor_id: 'test-sensor',
		region_count: 2,
		frames_sampled: 500,
		identification_complete: true,
		grid_mapping: gridMapping,
		regions: [
			{
				mean_variance: 0.123,
				cell_count: 50,
				params: {
					noise_relative_fraction: 0.05,
					neighbor_confirmation_count: 3,
					settle_update_fraction: 0.1
				}
			},
			{
				mean_variance: 0.456,
				cell_count: 30,
				params: {
					noise_relative_fraction: 0.08,
					neighbor_confirmation_count: 2,
					settle_update_fraction: 0.2
				}
			}
		],
		...overrides
	};
}

/** Install a global.fetch mock that resolves with the supplied data. */
function mockFetchWith(data?: Record<string, unknown>): void {
	const d = data ?? makeSampleData();
	global.fetch = jest.fn().mockResolvedValue({
		ok: true,
		json: () => Promise.resolve(d)
	});
}

/** Convert hex colour (#rrggbb) to the rgb(r, g, b) string jsdom uses. */
function hexToRgb(hex: string): string {
	const r = parseInt(hex.slice(1, 3), 16);
	const g = parseInt(hex.slice(3, 5), 16);
	const b = parseInt(hex.slice(5, 7), 16);
	return `rgb(${r}, ${g}, ${b})`;
}

/** Clear call counts on every jest.fn() inside mockCtx. */
function clearCtxMocks(): void {
	for (const v of Object.values(mockCtx)) {
		if (typeof v === 'function' && 'mockClear' in v) {
			(v as jest.Mock).mockClear();
		}
	}
}

// ===========================================================================
// Pure function tests (no DOM required)
// ===========================================================================

describe('escapeHTML', () => {
	it('re-exports escapeHTML from dashboard_common', () => {
		expect(typeof escapeHTML).toBe('function');
		expect(escapeHTML('<b>bold</b>')).toBe('&lt;b&gt;bold&lt;/b&gt;');
	});
});

describe('regionColors', () => {
	it('is an array of 20 hex colour strings', () => {
		expect(Array.isArray(regionColors)).toBe(true);
		expect(regionColors).toHaveLength(20);
	});

	it('every entry is a valid hex colour', () => {
		const hexPattern = /^#[0-9a-fA-F]{6}$/;
		regionColors.forEach((color: string) => {
			expect(color).toMatch(hexPattern);
		});
	});
});

describe('cellAtPixel', () => {
	const W = 1200;
	const H = 600;

	it('returns ring=0, azBin=0 for top-left corner', () => {
		const cell = cellAtPixel(0, 0, W, H);
		expect(cell).not.toBeNull();
		expect(cell!.ring).toBe(0);
		expect(cell!.azBin).toBe(0);
		expect(cell!.cellIdx).toBe(0);
	});

	it('returns last ring and azBin for near-bottom-right corner', () => {
		const cell = cellAtPixel(W - 0.01, H - 0.01, W, H);
		expect(cell).not.toBeNull();
		expect(cell!.ring).toBe(39); // 40 rings, 0-indexed
		expect(cell!.azBin).toBe(1799); // 1800 bins, 0-indexed
	});

	it('computes correct cellIdx from ring and azBin', () => {
		const cell = cellAtPixel(W / 2, H / 2, W, H);
		expect(cell).not.toBeNull();
		expect(cell!.cellIdx).toBe(cell!.ring * 1800 + cell!.azBin);
	});

	it('returns null for negative coordinates', () => {
		expect(cellAtPixel(-1, 0, W, H)).toBeNull();
		expect(cellAtPixel(0, -1, W, H)).toBeNull();
	});

	it('returns null for coordinates beyond canvas', () => {
		expect(cellAtPixel(W + 1, 0, W, H)).toBeNull();
		expect(cellAtPixel(0, H + 1, W, H)).toBeNull();
	});

	it('maps centre of canvas to expected ring and azBin', () => {
		const cell = cellAtPixel(W / 2, H / 2, W, H);
		expect(cell).not.toBeNull();
		expect(cell!.azBin).toBe(900); // middle of 1800 bins
		expect(cell!.ring).toBe(20); // middle of 40 rings
	});
});

// ===========================================================================
// DOM-only tests (no canvas / module state required)
// ===========================================================================

describe('updateInfoPanel', () => {
	beforeEach(setupDOM);

	it('sets sensorId text content', () => {
		updateInfoPanel({
			sensor_id: 'lidar-42',
			region_count: 0,
			frames_sampled: 0,
			identification_complete: false
		});
		expect(document.getElementById('sensorId')!.textContent).toBe('lidar-42');
	});

	it('sets regionCount text content', () => {
		updateInfoPanel({
			sensor_id: '',
			region_count: 7,
			frames_sampled: 0,
			identification_complete: false
		});
		expect(document.getElementById('regionCount')!.textContent).toBe('7');
	});

	it('sets framesSampled text content', () => {
		updateInfoPanel({
			sensor_id: '',
			region_count: 0,
			frames_sampled: 250,
			identification_complete: false
		});
		expect(document.getElementById('framesSampled')!.textContent).toBe('250');
	});

	it('shows "Complete" when identification_complete is true', () => {
		updateInfoPanel({
			sensor_id: '',
			region_count: 0,
			frames_sampled: 0,
			identification_complete: true
		});
		const status = document.getElementById('status')!;
		expect(status.textContent).toBe('Complete');
		expect(status.className).toBe('status status-complete');
	});

	it('shows "Collecting..." when identification_complete is false', () => {
		updateInfoPanel({
			sensor_id: '',
			region_count: 0,
			frames_sampled: 0,
			identification_complete: false
		});
		const status = document.getElementById('status')!;
		expect(status.textContent).toBe('Collecting...');
		expect(status.className).toBe('status status-pending');
	});
});

describe('updateLegend', () => {
	beforeEach(setupDOM);

	it('shows placeholder when regions is undefined', () => {
		updateLegend({});
		expect(document.getElementById('legendItems')!.innerHTML).toBe(
			'<div>No regions identified yet</div>'
		);
	});

	it('shows placeholder when regions array is empty', () => {
		updateLegend({ regions: [] });
		expect(document.getElementById('legendItems')!.innerHTML).toBe(
			'<div>No regions identified yet</div>'
		);
	});

	it('creates one legend-item per region', () => {
		updateLegend(makeSampleData());
		const items = document.querySelectorAll('#legendItems .legend-item');
		expect(items).toHaveLength(2);
	});

	it('displays variance, noise, and cell count in label', () => {
		updateLegend(makeSampleData());
		const span = document.querySelector('#legendItems .legend-item span')!;
		expect(span.textContent).toContain('Region 0');
		expect(span.textContent).toContain('var=0.123');
		expect(span.textContent).toContain('noise=0.050');
		expect(span.textContent).toContain('50 cells');
	});

	it('displays second region with correct values', () => {
		updateLegend(makeSampleData());
		const spans = document.querySelectorAll('#legendItems .legend-item span');
		expect(spans[1].textContent).toContain('Region 1');
		expect(spans[1].textContent).toContain('var=0.456');
		expect(spans[1].textContent).toContain('noise=0.080');
		expect(spans[1].textContent).toContain('30 cells');
	});

	it('sets correct background colour on the colour swatch', () => {
		updateLegend(makeSampleData());
		const boxes = document.querySelectorAll(
			'#legendItems .legend-color'
		) as NodeListOf<HTMLElement>;
		expect(boxes[0].style.backgroundColor).toBe(hexToRgb(regionColors[0]));
		expect(boxes[1].style.backgroundColor).toBe(hexToRgb(regionColors[1]));
	});

	it('handles region with missing params gracefully', () => {
		updateLegend({
			regions: [{ mean_variance: null, cell_count: 0 }]
		});
		const span = document.querySelector('#legendItems .legend-item span')!;
		expect(span.textContent).toContain('var=0.000');
		expect(span.textContent).toContain('noise=0.000');
		expect(span.textContent).toContain('0 cells');
	});

	it('wraps colour index for more than 20 regions', () => {
		const regions = Array.from({ length: 21 }, (_, i) => ({
			mean_variance: i * 0.01,
			cell_count: i,
			params: { noise_relative_fraction: 0 }
		}));
		updateLegend({ regions });
		const boxes = document.querySelectorAll(
			'#legendItems .legend-color'
		) as NodeListOf<HTMLElement>;
		// The 21st region (index 20) wraps back to regionColors[0]
		expect(boxes[20].style.backgroundColor).toBe(hexToRgb(regionColors[0]));
	});

	it('legend items have cursor pointer style', () => {
		updateLegend(makeSampleData());
		const item = document.querySelector('#legendItems .legend-item') as HTMLElement;
		expect(item.style.cursor).toBe('pointer');
	});
});

// ===========================================================================
// Canvas / module-state tests
// ===========================================================================

describe('drawRegions', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		// Use never-resolving fetch so autoRefresh's loadRegions doesn't
		// trigger an extra drawRegions call via the promise chain.
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init(); // sets module-level canvas, ctx (regionData stays null)
		clearCtxMocks();
	});

	afterEach(() => {
		jest.useRealTimers();
	});

	it('shows placeholder text when grid_mapping is null', () => {
		drawRegions({ grid_mapping: null, regions: [] });
		expect(mockCtx.clearRect).toHaveBeenCalledWith(0, 0, 1200, 600);
		expect(mockCtx.fillText).toHaveBeenCalledWith('No region data available', 600, 300);
	});

	it('shows placeholder text when grid_mapping is empty', () => {
		drawRegions({ grid_mapping: [], regions: [] });
		expect(mockCtx.clearRect).toHaveBeenCalledWith(0, 0, 1200, 600);
		expect(mockCtx.fillText).toHaveBeenCalledWith('No region data available', 600, 300);
	});

	it('sets correct font and style for the empty-data message', () => {
		drawRegions({ grid_mapping: null, regions: [] });
		expect(mockCtx.fillStyle).toBe('#666');
		expect(mockCtx.font).toBe('16px sans-serif');
		expect(mockCtx.textAlign).toBe('center');
	});

	it('does not draw grid lines for empty data', () => {
		drawRegions({ grid_mapping: [], regions: [] });
		expect(mockCtx.beginPath).not.toHaveBeenCalled();
	});

	it('draws a fillRect for each cell with regionId >= 0', () => {
		drawRegions(makeSampleData());
		// sample data has 3 cells with regionId >= 0
		expect(mockCtx.fillRect).toHaveBeenCalledTimes(3);
	});

	it('uses the correct colour from regionColors for each cell', () => {
		const data = makeSampleData();
		drawRegions(data);
		// The last fillRect call should be for region 1 (gridMapping[1800] = 1)
		// After drawing all 3 cells, the last fillStyle set is regionColors[1]
		// We can check the calls to fillRect correspond to correct positions
		const calls = mockCtx.fillRect.mock.calls;
		// Cell 0: ring=0, azBin=0 -> x=0, y=0
		expect(calls[0][0]).toBeCloseTo(0);
		expect(calls[0][1]).toBeCloseTo(0);
		// Cell 1: ring=0, azBin=1 -> x=cellWidth, y=0
		expect(calls[1][0]).toBeCloseTo(1200 / 1800);
		expect(calls[1][1]).toBeCloseTo(0);
	});

	it('draws horizontal grid lines every 5 rings', () => {
		drawRegions(makeSampleData());
		// rings 0, 5, 10, 15, 20, 25, 30, 35, 40 => 9 lines
		expect(mockCtx.beginPath).toHaveBeenCalledTimes(9);
		expect(mockCtx.stroke).toHaveBeenCalledTimes(9);
	});

	it('sets grid-line stroke style', () => {
		drawRegions(makeSampleData());
		expect(mockCtx.strokeStyle).toBe('rgba(0, 0, 0, 0.05)');
		expect(mockCtx.lineWidth).toBe(0.5);
	});

	it('populates the legend via updateLegend', () => {
		drawRegions(makeSampleData());
		const items = document.querySelectorAll('#legendItems .legend-item');
		expect(items).toHaveLength(2);
	});

	it('returns early after placeholder without drawing grid lines', () => {
		drawRegions({ grid_mapping: undefined, regions: [] });
		expect(mockCtx.beginPath).not.toHaveBeenCalled();
		expect(mockCtx.moveTo).not.toHaveBeenCalled();
	});
});

describe('drawRegionOutline', () => {
	afterEach(() => {
		jest.useRealTimers();
	});

	it('returns early when regionData is not loaded', () => {
		jest.useFakeTimers();
		setupDOM();
		// Use a never-resolving fetch so regionData stays null
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		clearCtxMocks();
		drawRegionOutline(0);
		expect(mockCtx.strokeRect).not.toHaveBeenCalled();
	});

	it('draws strokeRect for cells matching the regionId', async () => {
		jest.useFakeTimers();
		setupDOM();
		mockFetchWith();
		init();
		await flushPromises(); // regionData now populated
		clearCtxMocks();
		drawRegionOutline(0);
		// sample data has 2 cells with regionId 0
		expect(mockCtx.strokeRect).toHaveBeenCalledTimes(2);
		expect(mockCtx.strokeStyle).toBe('#FFD700');
		expect(mockCtx.lineWidth).toBe(3);
	});

	it('draws strokeRect for regionId 1', async () => {
		jest.useFakeTimers();
		setupDOM();
		mockFetchWith();
		init();
		await flushPromises();
		clearCtxMocks();
		drawRegionOutline(1);
		// sample data has 1 cell with regionId 1
		expect(mockCtx.strokeRect).toHaveBeenCalledTimes(1);
	});

	it('does not draw anything for a regionId with no matching cells', async () => {
		jest.useFakeTimers();
		setupDOM();
		mockFetchWith();
		init();
		await flushPromises();
		clearCtxMocks();
		drawRegionOutline(99);
		expect(mockCtx.strokeRect).not.toHaveBeenCalled();
	});
});

describe('selectRegion', () => {
	beforeEach(async () => {
		jest.useFakeTimers();
		setupDOM();
		mockFetchWith();
		init();
		await flushPromises(); // populate regionData
		clearCtxMocks();
	});

	afterEach(() => {
		jest.useRealTimers();
	});

	it('selects a region and redraws with outline', () => {
		selectRegion(0);
		// drawRegions is called internally, which clears and redraws
		expect(mockCtx.clearRect).toHaveBeenCalled();
		// selectedRegionId is now 0, so drawRegionOutline(0) is called
		expect(mockCtx.strokeRect).toHaveBeenCalled();
	});

	it('deselects the same region on second call (toggle)', () => {
		selectRegion(0); // select
		clearCtxMocks();
		selectRegion(0); // deselect (toggle off)
		expect(mockCtx.clearRect).toHaveBeenCalled();
		// No region selected, so no strokeRect for outline
		expect(mockCtx.strokeRect).not.toHaveBeenCalled();
	});

	it('switches between different regions', () => {
		selectRegion(0); // select region 0
		clearCtxMocks();
		selectRegion(1); // switch to region 1 (0 !== 1 so set to 1)
		expect(mockCtx.clearRect).toHaveBeenCalled();
		// Region 1 outline should be drawn
		expect(mockCtx.strokeRect).toHaveBeenCalled();
	});

	it('clicking legend item triggers selectRegion', async () => {
		// updateLegend was called as part of the data load; re-draw to be sure
		drawRegions(makeSampleData());
		clearCtxMocks();

		const legendItem = document.querySelector('#legendItems .legend-item') as HTMLElement;
		legendItem.click();

		// selectRegion(0) was called via onclick, which calls drawRegions
		expect(mockCtx.clearRect).toHaveBeenCalled();
	});
});

// ===========================================================================
// loadRegions (async + fetch)
// ===========================================================================

describe('loadRegions', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		mockFetchWith();
		init(); // sets module-level sensorId from meta tag
	});

	afterEach(() => {
		jest.useRealTimers();
	});

	it('fetches from the correct URL with encoded sensor id', () => {
		(global.fetch as jest.Mock).mockClear();
		loadRegions();
		expect(global.fetch).toHaveBeenCalledWith(
			'/debug/lidar/background/regions?sensor_id=test-sensor'
		);
	});

	it('updates DOM elements on successful fetch', async () => {
		const data = makeSampleData({
			sensor_id: 'loaded-sensor',
			region_count: 9,
			frames_sampled: 777,
			identification_complete: true
		});
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve(data)
		});

		loadRegions();
		await flushPromises();

		expect(document.getElementById('sensorId')!.textContent).toBe('loaded-sensor');
		expect(document.getElementById('regionCount')!.textContent).toBe('9');
		expect(document.getElementById('framesSampled')!.textContent).toBe('777');
		expect(document.getElementById('status')!.textContent).toBe('Complete');
	});

	it('calls drawRegions after successful fetch', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve(makeSampleData())
		});
		clearCtxMocks();

		loadRegions();
		await flushPromises();

		// drawRegions clears the canvas
		expect(mockCtx.clearRect).toHaveBeenCalled();
	});

	it('handles fetch rejection gracefully', async () => {
		const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
		global.fetch = jest.fn().mockRejectedValue(new Error('Network error'));

		loadRegions();
		await flushPromises();

		const status = document.getElementById('status')!;
		expect(status.textContent).toBe('Error loading');
		expect(status.className).toBe('status status-pending');
		expect(consoleSpy).toHaveBeenCalledWith('Error loading regions:', expect.any(Error));

		consoleSpy.mockRestore();
	});

	it('handles JSON parse failure gracefully', async () => {
		const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.reject(new Error('Invalid JSON'))
		});

		loadRegions();
		await flushPromises();

		const status = document.getElementById('status')!;
		expect(status.textContent).toBe('Error loading');
		expect(status.className).toBe('status status-pending');

		consoleSpy.mockRestore();
	});

	it('encodes special characters in sensor id', () => {
		// Re-init with a sensor id that has special chars
		document.head.innerHTML = '<meta name="sensor-id" content="sensor with spaces" />';
		init();
		(global.fetch as jest.Mock).mockClear();

		loadRegions();
		expect(global.fetch).toHaveBeenCalledWith(
			'/debug/lidar/background/regions?sensor_id=sensor%20with%20spaces'
		);
	});
});

// ===========================================================================
// autoRefresh (timers)
// ===========================================================================

describe('autoRefresh', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		mockFetchWith();
	});

	afterEach(() => {
		jest.useRealTimers();
	});

	it('calls loadRegions immediately', () => {
		init(); // sets sensorId
		(global.fetch as jest.Mock).mockClear();
		autoRefresh();
		expect(global.fetch).toHaveBeenCalledTimes(1);
	});

	it('schedules another call after 5s when regionData is null', () => {
		init();
		(global.fetch as jest.Mock).mockClear();

		// The init() autoRefresh scheduled setTimeout(5000) because
		// regionData was null at call time. Advance to fire it.
		jest.advanceTimersByTime(5000);
		expect(global.fetch).toHaveBeenCalledTimes(1);
	});

	it('keeps calling every 5s while not complete', () => {
		mockFetchWith(makeSampleData({ identification_complete: false }));
		init();
		(global.fetch as jest.Mock).mockClear();

		jest.advanceTimersByTime(5000);
		expect(global.fetch).toHaveBeenCalledTimes(1);

		(global.fetch as jest.Mock).mockClear();
		jest.advanceTimersByTime(5000);
		expect(global.fetch).toHaveBeenCalledTimes(1);
	});

	it('uses 30s interval after regionData is complete', async () => {
		mockFetchWith(makeSampleData({ identification_complete: true }));
		init();
		await flushPromises(); // regionData now set with identification_complete: true

		(global.fetch as jest.Mock).mockClear();

		// The init's autoRefresh scheduled setTimeout(5000) (regionData was null
		// at call time). After 5s the callback fires; regionData is now complete
		// so it schedules setTimeout(30000).
		jest.advanceTimersByTime(5000);
		expect(global.fetch).toHaveBeenCalledTimes(1);

		(global.fetch as jest.Mock).mockClear();
		// At 29999ms nothing should happen
		jest.advanceTimersByTime(29999);
		expect(global.fetch).not.toHaveBeenCalled();

		// At 30000ms the next refresh should fire
		jest.advanceTimersByTime(1);
		expect(global.fetch).toHaveBeenCalledTimes(1);
	});
});

// ===========================================================================
// init (integration)
// ===========================================================================

describe('init', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		mockFetchWith();
	});

	afterEach(() => {
		jest.useRealTimers();
	});

	it('obtains a 2d context from the canvas', () => {
		init();
		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		expect(canvasEl.getContext).toHaveBeenCalledWith('2d');
	});

	it('reads sensor-id from meta tag and uses it in fetch URL', () => {
		init(); // triggers autoRefresh -> loadRegions -> fetch
		expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining('sensor_id=test-sensor'));
	});

	it('starts auto-refresh which calls fetch', () => {
		init();
		expect(global.fetch).toHaveBeenCalled();
	});

	// -- mouseleave handler -------------------------------------------------

	it('hides tooltip on mouseleave', () => {
		init();
		const canvasEl = document.getElementById('regionCanvas')!;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;
		tooltipEl.style.display = 'block';

		canvasEl.dispatchEvent(new Event('mouseleave'));
		expect(tooltipEl.style.display).toBe('none');
	});

	// -- mousemove handler --------------------------------------------------

	it('does nothing on mousemove when regionData is not loaded', () => {
		init(); // regionData still null (promises not flushed)
		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		canvasEl.dispatchEvent(new MouseEvent('mousemove', { clientX: 5, clientY: 5, bubbles: true }));
		expect(tooltipEl.style.display).not.toBe('block');
	});

	it('shows tooltip when hovering over a valid region cell', async () => {
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		// Cell [ring=0, azBin=0] => gridMapping[0] = regionId 0
		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', {
				clientX: 0.1,
				clientY: 0.1,
				bubbles: true
			})
		);

		expect(tooltipEl.style.display).toBe('block');
		expect(tooltipEl.innerHTML).toContain('Region 0');
		expect(tooltipEl.innerHTML).toContain('Ring: 0');
		expect(tooltipEl.innerHTML).toContain('Variance: 0.123');
		expect(tooltipEl.innerHTML).toContain('Noise Rel: 0.050');
		expect(tooltipEl.innerHTML).toContain('Neighbors: 3');
		expect(tooltipEl.innerHTML).toContain('Alpha: 0.100');
	});

	it('positions tooltip relative to cursor', async () => {
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', {
				clientX: 0.1,
				clientY: 0.1,
				bubbles: true
			})
		);

		// tooltip.style.left = clientX + 10 + "px"
		expect(tooltipEl.style.left).toBe('10.1px');
		expect(tooltipEl.style.top).toBe('10.1px');
	});

	it('hides tooltip when hovering over a cell with no region', async () => {
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		// Cell at centre has regionId -1
		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', {
				clientX: 600,
				clientY: 300,
				bubbles: true
			})
		);
		expect(tooltipEl.style.display).toBe('none');
	});

	it('does not update tooltip when mouse is outside grid bounds', async () => {
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;
		tooltipEl.style.display = 'none';

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		// clientX = 1200 maps to canvasX = 1200 => azBin = 1800 => out of bounds
		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', {
				clientX: 1200,
				clientY: 0,
				bubbles: true
			})
		);
		// tooltip should remain unchanged (cell is null, handler does nothing)
		expect(tooltipEl.style.display).toBe('none');
	});

	it('applies canvas scaling when rendered size differs from pixel size', async () => {
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		// Canvas is 1200x600 pixels but rendered at 600x300 CSS pixels
		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 600,
			height: 300,
			right: 600,
			bottom: 300
		});

		// clientX=0.05 at 2x scale => canvasX=0.1. Should hit cell [0,0] = region 0
		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', {
				clientX: 0.05,
				clientY: 0.05,
				bubbles: true
			})
		);

		expect(tooltipEl.style.display).toBe('block');
		expect(tooltipEl.innerHTML).toContain('Region 0');
	});

	it('shows tooltip data for region 1', async () => {
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		// gridMapping[1800] = 1 (ring 1, azBin 0)
		// azBin 0 => clientX near 0, ring 1 => clientY near cellHeight = 15
		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', {
				clientX: 0.1,
				clientY: 15.1,
				bubbles: true
			})
		);

		expect(tooltipEl.style.display).toBe('block');
		expect(tooltipEl.innerHTML).toContain('Region 1');
		expect(tooltipEl.innerHTML).toContain('Variance: 0.456');
		expect(tooltipEl.innerHTML).toContain('Noise Rel: 0.080');
		expect(tooltipEl.innerHTML).toContain('Neighbors: 2');
		expect(tooltipEl.innerHTML).toContain('Alpha: 0.200');
	});

	it('includes azimuth in tooltip', async () => {
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		// Cell [ring=0, azBin=0] -> azDegrees = (0 * 360 / 1800).toFixed(1) = "0.0"
		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', {
				clientX: 0.1,
				clientY: 0.1,
				bubbles: true
			})
		);

		expect(tooltipEl.innerHTML).toContain('Azimuth: 0.0');
	});

	it('includes colour swatch in tooltip HTML', async () => {
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', {
				clientX: 0.1,
				clientY: 0.1,
				bubbles: true
			})
		);

		// The tooltip HTML includes the region colour as a background
		expect(tooltipEl.innerHTML).toContain(regionColors[0]);
	});

	it('shows tooltip with zero fallbacks when region has no params', async () => {
		// Override fetch to return region without params object and null mean_variance
		const noParamsData = makeSampleData({
			regions: [
				{ mean_variance: null, cell_count: 10 },
				{
					mean_variance: 0.6,
					cell_count: 20,
					params: {
						noise_relative_fraction: 0.08,
						neighbor_confirmation_count: 2,
						settle_update_fraction: 0.2
					}
				}
			]
		});
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve(noParamsData)
		});
		init();
		await flushPromises();

		const canvasEl = document.getElementById('regionCanvas') as HTMLCanvasElement;
		const tooltipEl = document.getElementById('tooltip') as HTMLElement;

		canvasEl.getBoundingClientRect = jest.fn().mockReturnValue({
			left: 0,
			top: 0,
			width: 1200,
			height: 600,
			right: 1200,
			bottom: 600
		});

		// Hover over cell [0,0] = region 0 (which has no params)
		canvasEl.dispatchEvent(
			new MouseEvent('mousemove', { clientX: 0.1, clientY: 0.1, bubbles: true })
		);

		expect(tooltipEl.style.display).toBe('block');
		expect(tooltipEl.innerHTML).toContain('Region 0');
		expect(tooltipEl.innerHTML).toContain('Variance: 0.000');
		expect(tooltipEl.innerHTML).toContain('Noise Rel: 0.000');
		expect(tooltipEl.innerHTML).toContain('Neighbors: 0');
		expect(tooltipEl.innerHTML).toContain('Alpha: 0.000');
	});
});
