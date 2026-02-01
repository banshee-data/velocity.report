/* eslint-disable @typescript-eslint/no-unused-vars */
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

	it('should download SVG map from OpenStreetMap', async () => {
		const mockSVG =
			'<svg xmlns="http://www.w3.org/2000/svg"><circle cx="50" cy="50" r="40"/></svg>';

		(global.fetch as jest.Mock).mockResolvedValueOnce({
			ok: true,
			text: async () => mockSVG
		});

		const { component } = render(MapEditorInteractive, {
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
		await fireEvent.click(downloadButton);

		await waitFor(() => {
			expect(global.fetch).toHaveBeenCalledWith(
				expect.stringContaining('render.openstreetmap.org')
			);
		});
	});

	it('should handle SVG download errors gracefully', async () => {
		(global.fetch as jest.Mock).mockResolvedValueOnce({
			ok: false,
			status: 500,
			statusText: 'Internal Server Error'
		});

		const { component } = render(MapEditorInteractive, {
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
		await fireEvent.click(downloadButton);

		await waitFor(() => {
			// Should show error message
			expect(screen.getByText(/Failed to download map/i)).toBeInTheDocument();
		});
	});
});
