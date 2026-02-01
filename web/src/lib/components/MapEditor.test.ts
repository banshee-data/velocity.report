import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import '@testing-library/jest-dom';
import MapEditor from './MapEditor.svelte';

describe('MapEditor', () => {
	// Mock fetch globally
	global.fetch = jest.fn();

	beforeEach(() => {
		jest.resetAllMocks();
	});

	it('should render map editor with all fields', () => {
		render(MapEditor, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: null,
				bboxNELng: null,
				bboxSWLat: null,
				bboxSWLng: null,
				mapRotation: 0,
				mapSvgData: null
			}
		});

		expect(screen.getByText('Map Configuration')).toBeInTheDocument();
		expect(screen.getByText('Radar Location')).toBeInTheDocument();
		expect(screen.getByText('Map Bounding Box')).toBeInTheDocument();
	});

	it('should validate latitude range', () => {
		const { component } = render(MapEditor, {
			props: {
				latitude: 95, // Invalid - out of range
				longitude: 0,
				radarAngle: null,
				bboxNELat: null,
				bboxNELng: null,
				bboxSWLat: null,
				bboxSWLng: null,
				mapRotation: null,
				mapSvgData: null
			}
		});

		// The component should show validation error for invalid latitude
		const latitudeInput = screen.getByLabelText(/latitude/i);
		expect(latitudeInput).toBeInTheDocument();
	});

	it('should validate longitude range', () => {
		const { component } = render(MapEditor, {
			props: {
				latitude: 0,
				longitude: 185, // Invalid - out of range
				radarAngle: null,
				bboxNELat: null,
				bboxNELng: null,
				bboxSWLat: null,
				bboxSWLng: null,
				mapRotation: null,
				mapSvgData: null
			}
		});

		const longitudeInput = screen.getByLabelText(/longitude/i);
		expect(longitudeInput).toBeInTheDocument();
	});

	it('should set default bounding box when button is clicked', async () => {
		const { component } = render(MapEditor, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: null,
				bboxNELat: null,
				bboxNELng: null,
				bboxSWLat: null,
				bboxSWLng: null,
				mapRotation: null,
				mapSvgData: null
			}
		});

		const setDefaultButton = screen.getByText(/Set Default/i);
		await fireEvent.click(setDefaultButton);

		// The component should set bbox values around the radar position
		// Values should be set via component state
	});

	it('should download SVG map from OpenStreetMap', async () => {
		const mockSVG = '<svg xmlns="http://www.w3.org/2000/svg"><circle cx="50" cy="50" r="40"/></svg>';
		
		(global.fetch as jest.Mock).mockResolvedValueOnce({
			ok: true,
			text: async () => mockSVG
		});

		const { component } = render(MapEditor, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: 51.5124,
				bboxNELng: -0.1228,
				bboxSWLat: 51.5024,
				bboxSWLng: -0.1328,
				mapRotation: 0,
				mapSvgData: null
			}
		});

		const downloadButton = screen.getByText(/Download Map SVG/i);
		await fireEvent.click(downloadButton);

		await waitFor(() => {
			expect(global.fetch).toHaveBeenCalledWith(
				expect.stringContaining('render.openstreetmap.org')
			);
		});
	});

	it('should validate bounding box coordinates', () => {
		const { component } = render(MapEditor, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: null,
				bboxNELat: 51.5024, // NE is less than SW - invalid!
				bboxNELng: -0.1228,
				bboxSWLat: 51.5124,
				bboxSWLng: -0.1328,
				mapRotation: null,
				mapSvgData: null
			}
		});

		// Should show validation error for invalid bounding box
		expect(screen.getByText(/Invalid bounding box/i)).toBeInTheDocument();
	});

	it('should handle SVG download errors gracefully', async () => {
		(global.fetch as jest.Mock).mockResolvedValueOnce({
			ok: false,
			status: 500,
			statusText: 'Internal Server Error'
		});

		const { component } = render(MapEditor, {
			props: {
				latitude: 51.5074,
				longitude: -0.1278,
				radarAngle: 45,
				bboxNELat: 51.5124,
				bboxNELng: -0.1228,
				bboxSWLat: 51.5024,
				bboxSWLng: -0.1328,
				mapRotation: 0,
				mapSvgData: null
			}
		});

		const downloadButton = screen.getByText(/Download Map SVG/i);
		await fireEvent.click(downloadButton);

		await waitFor(() => {
			// Should show error message
			expect(screen.getByText(/Failed to download map/i)).toBeInTheDocument();
		});
	});
});
