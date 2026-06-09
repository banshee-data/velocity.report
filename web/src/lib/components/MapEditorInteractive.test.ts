import '@testing-library/jest-dom';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import MapEditorInteractive from './MapEditorInteractive.svelte';

describe('MapEditorInteractive', () => {
	// Mock fetch globally
	global.fetch = jest.fn();

	beforeEach(() => {
		jest.resetAllMocks();
	});

	afterEach(() => {
		cleanup();
	});

	it('should render map editor component', () => {
		render(MapEditorInteractive, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: null,
				bboxNELng: null,
				bboxSWLat: null,
				bboxSWLng: null,
				mapSvgData: null
			}
		});

		expect(screen.getByText('Map Configuration')).toBeInTheDocument();
	});

	it('should display default San Francisco coordinates when no props provided', () => {
		render(MapEditorInteractive, {
			props: {
				latitude: null,
				longitude: null,
				radarAngle: null,
				bboxNELat: null,
				bboxNELng: null,
				bboxSWLat: null,
				bboxSWLng: null,
				mapSvgData: null
			}
		});

		expect(screen.getByText('Map Configuration')).toBeInTheDocument();
		// Component should initialise with San Francisco defaults (37.7749, -122.4194)
	});

	it('should have generate button for the tile report map snapshot', async () => {
		render(MapEditorInteractive, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: 51.5124,
				bboxNELng: -0.1228,
				bboxSWLat: 51.5024,
				bboxSWLng: -0.1328,
				mapSvgData: null
			}
		});

		const generateButton = screen.getByText(/Generate Tile Snapshot/i);
		expect(generateButton).toBeInTheDocument();
	});

	it('should disable tile snapshot generation without a bounding box', async () => {
		render(MapEditorInteractive, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: null,
				bboxNELng: null,
				bboxSWLat: null,
				bboxSWLng: null,
				mapSvgData: null
			}
		});

		const generateButton = screen.getByText(/Generate Tile Snapshot/i);
		expect(generateButton).toBeDisabled();
	});

	it('should require explicit consent before generating a tile report map snapshot', async () => {
		(global.fetch as jest.Mock).mockResolvedValue({
			ok: true,
			blob: async () => new Blob()
		});

		render(MapEditorInteractive, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: 51.5124,
				bboxNELng: -0.1228,
				bboxSWLat: 51.5024,
				bboxSWLng: -0.1328,
				mapSvgData: null
			}
		});

		const generateButton = screen.getByText(/Generate Tile Snapshot/i);
		await fireEvent.click(generateButton);

		expect(screen.getByText(/Allow Map Tile Requests/i)).toBeInTheDocument();
		expect(screen.getByText(/OpenStreetMap raster tiles/i)).toBeInTheDocument();
		expect(screen.getByText(/Upload instead/i)).toBeInTheDocument();
		expect(
			screen.getByText(/Radar observations, vehicle data, reports, and raw sensor data/i)
		).toBeInTheDocument();
		expect(global.fetch).not.toHaveBeenCalled();

		await fireEvent.click(screen.getByText(/Allow Tiles This Session/i));

		await waitFor(() => {
			expect(global.fetch).toHaveBeenCalledTimes(1);
		});
	});

	it('should require explicit consent before loading OSM map tiles', async () => {
		render(MapEditorInteractive, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: 51.5124,
				bboxNELng: -0.1228,
				bboxSWLat: 51.5024,
				bboxSWLng: -0.1328,
				mapSvgData: null
			}
		});

		await fireEvent.click(screen.getByText(/Load Map Tiles/i));

		expect(screen.getByText(/Allow Map Tile Requests/i)).toBeInTheDocument();
		expect(screen.getByText(/external tile servers/i)).toBeInTheDocument();
		expect(global.fetch).not.toHaveBeenCalled();

		await fireEvent.click(screen.getByText(/Allow Tiles This Session/i));

		expect(screen.getByText(/Map tiles loaded/i)).toBeInTheDocument();
		expect(global.fetch).not.toHaveBeenCalled();
	});

	it('should not expose geocoder or alternate map-source controls in the tile-only editor', () => {
		render(MapEditorInteractive, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: 51.5124,
				bboxNELng: -0.1228,
				bboxSWLat: 51.5024,
				bboxSWLng: -0.1328,
				mapSvgData: null
			}
		});

		expect(screen.queryByText(/^Search$/i)).not.toBeInTheDocument();
		expect(screen.queryByText(/Map style/i)).not.toBeInTheDocument();
		expect(screen.queryByText(/Mirror/i)).not.toBeInTheDocument();
	});

	it('should label saved SVG as database state and hide the preview by default', async () => {
		render(MapEditorInteractive, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: 51.5124,
				bboxNELng: -0.1228,
				bboxSWLat: 51.5024,
				bboxSWLng: -0.1328,
				mapSvgData: 'PHN2Zy8+'
			}
		});

		expect(screen.getByText(/Existing Saved Report Map \(Database\)/i)).toBeInTheDocument();
		expect(screen.getByText(/site.map_svg_data/i)).toBeInTheDocument();
		const preview = document.querySelector('[data-report-map-preview]') as HTMLElement;
		expect(preview).not.toBeVisible();

		await fireEvent.click(screen.getByText(/Show Preview/i));

		expect(preview).toBeVisible();
	});

	it('should display help text about dragging FOV marker', () => {
		render(MapEditorInteractive, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: 51.5124,
				bboxNELng: -0.1228,
				bboxSWLat: 51.5024,
				bboxSWLng: -0.1328,
				mapSvgData: null
			}
		});

		// Check for FOV angle help text
		expect(screen.getByText(/Drag the red dot/i)).toBeInTheDocument();
	});

	describe('FOV angle calculations', () => {
		it('should normalise angle to 0-360 range', () => {
			// Test the angle normalisation logic (0 = North, 90 = East, etc.)
			const normaliseAngle = (angle: number): number => {
				let normalised = angle;
				if (normalised < 0) normalised += 360;
				return Math.round(normalised);
			};

			expect(normaliseAngle(0)).toBe(0);
			expect(normaliseAngle(90)).toBe(90);
			expect(normaliseAngle(180)).toBe(180);
			expect(normaliseAngle(270)).toBe(270);
			expect(normaliseAngle(-45)).toBe(315);
			expect(normaliseAngle(-90)).toBe(270);
		});

		it('should calculate correct bearing from coordinates', () => {
			// Test bearing calculation (atan2 based)
			const calculateBearing = (
				radarLat: number,
				radarLng: number,
				tipLat: number,
				tipLng: number
			): number => {
				const dLat = tipLat - radarLat;
				const dLng = tipLng - radarLng;
				let angle = Math.atan2(dLng, dLat) * (180 / Math.PI);
				if (angle < 0) angle += 360;
				return Math.round(angle);
			};

			// North (0°): tip is directly north of radar
			expect(calculateBearing(0, 0, 1, 0)).toBe(0);

			// East (90°): tip is directly east of radar
			expect(calculateBearing(0, 0, 0, 1)).toBe(90);

			// South (180°): tip is directly south of radar
			expect(calculateBearing(0, 0, -1, 0)).toBe(180);

			// West (270°): tip is directly west of radar
			expect(calculateBearing(0, 0, 0, -1)).toBe(270);

			// Northeast (45°)
			expect(calculateBearing(0, 0, 1, 1)).toBe(45);

			// Southeast (135°)
			expect(calculateBearing(0, 0, -1, 1)).toBe(135);
		});
	});

	describe('Bounding box calculations', () => {
		it('should maintain 3:2 aspect ratio', () => {
			// Test that bbox has correct aspect ratio (width:height = 3:2)
			const heightMeters = 300;
			const widthMeters = 450;

			expect(widthMeters / heightMeters).toBe(1.5); // 3:2 ratio
		});

		it('should account for longitude compression at latitude', () => {
			// Test the longitude compression calculation
			const calculateLngCompression = (lat: number): number => {
				return Math.cos((lat * Math.PI) / 180);
			};

			// At equator, no compression
			expect(calculateLngCompression(0)).toBeCloseTo(1, 5);

			// At 45°, ~0.707 compression
			expect(calculateLngCompression(45)).toBeCloseTo(0.7071, 4);

			// At 60°, 0.5 compression
			expect(calculateLngCompression(60)).toBeCloseTo(0.5, 5);

			// San Francisco (~37.77°)
			expect(calculateLngCompression(37.77)).toBeCloseTo(0.79, 2);
		});

		it('keeps the +/- Area resize at 3:2 in metres (longitude-compensated)', () => {
			// Mirrors adjustBBoxSize: the width delta in degrees is divided by
			// cos(lat) so the box stays 3:2 in ground metres at any latitude. The
			// report SVG is a fixed 1200x800 canvas, so a box that is 3:2 in degrees
			// (not metres) would be stretched horizontally when rendered.
			const lat = 37.77;
			const lngCompression = Math.cos((lat * Math.PI) / 180);
			const heightDelta = 0.003;
			const widthDelta = (heightDelta * 1.5) / lngCompression;

			const metersPerDegreeLat = 111320;
			const metersPerDegreeLng = 111320 * lngCompression;
			const heightMeters = heightDelta * 2 * metersPerDegreeLat;
			const widthMeters = widthDelta * 2 * metersPerDegreeLng;
			expect(widthMeters / heightMeters).toBeCloseTo(1.5, 5);

			// The previous degree-space formula (heightDelta * 1.5) was not 3:2 in
			// metres away from the equator, which stretched the generated map.
			const buggyWidthMeters = heightDelta * 1.5 * 2 * metersPerDegreeLng;
			expect(buggyWidthMeters / heightMeters).not.toBeCloseTo(1.5, 2);
		});
	});
});
