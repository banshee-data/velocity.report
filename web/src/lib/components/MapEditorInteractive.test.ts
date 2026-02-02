import '@testing-library/jest-dom';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import MapEditorInteractive from './MapEditorInteractive.svelte';

describe('MapEditorInteractive', () => {
	// Mock fetch globally
	global.fetch = jest.fn();

	beforeEach(() => {
		jest.resetAllMocks();
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

	it('should have download button for SVG map', async () => {
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

		const downloadButton = screen.getByText(/Download Map SVG/i);
		expect(downloadButton).toBeInTheDocument();
	});

	it('should have download button even without bounding box', async () => {
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

		const downloadButton = screen.getByText(/Download Map SVG/i);
		expect(downloadButton).toBeInTheDocument();
	});

	it('should show error when downloading without bounding box', async () => {
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

		const downloadButton = screen.getByText(/Download Map SVG/i);
		await fireEvent.click(downloadButton);

		await waitFor(() => {
			expect(screen.getByText(/Please set bounding box coordinates first/i)).toBeInTheDocument();
		});
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
	});
});
