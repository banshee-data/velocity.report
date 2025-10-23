import {
	createSite,
	deleteReport,
	deleteSite,
	downloadReport,
	generateReport,
	getConfig,
	getEvents,
	getRadarStats,
	getRecentReports,
	getReport,
	getReportsForSite,
	getSite,
	getSites,
	updateSite,
	type Config,
	type Event,
	type Site,
	type SiteReport
} from './api';

// Mock fetch globally
global.fetch = jest.fn();

describe('api', () => {
	beforeEach(() => {
		jest.resetAllMocks();
	});

	describe('getEvents', () => {
		it('should fetch events without parameters', async () => {
			const mockEvents: Event[] = [
				{
					Speed: 25.5,
					Magnitude: 1.2,
					Uptime: 100
				}
			];

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockEvents
			});

			const result = await getEvents();

			const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
			expect(callUrl).toContain('/api/events');
			expect(result).toEqual(mockEvents);
		});

		it('should fetch events with units and timezone', async () => {
			const mockEvents: Event[] = [{ Speed: 30 }];

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockEvents
			});

			const result = await getEvents('mph', 'America/New_York');

			expect(global.fetch).toHaveBeenCalled();
			const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
			expect(callUrl).toContain('units=mph');
			expect(callUrl).toContain('timezone=America%2FNew_York');
			expect(result).toEqual(mockEvents);
		});

		it('should handle fetch errors', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500
			});

			await expect(getEvents()).rejects.toThrow('Failed to fetch events: 500');
		});
	});

	describe('getRadarStats', () => {
		it('should fetch radar stats and transform response', async () => {
			const serverResponse = {
				metrics: [
					{
						Classifier: 'car',
						StartTime: '2025-01-01T00:00:00Z',
						Count: 100,
						P50Speed: 50,
						P85Speed: 65,
						P98Speed: 75,
						MaxSpeed: 85
					}
				],
				histogram: { '0-10': 5, '10-20': 15 }
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => serverResponse
			});

			const result = await getRadarStats(1704067200, 1704153600);

			expect(result.metrics).toHaveLength(1);
			expect(result.metrics[0]).toEqual({
				classifier: 'car',
				date: new Date('2025-01-01T00:00:00Z'),
				count: 100,
				p50: 50,
				p85: 65,
				p98: 75,
				max: 85
			});
			expect(result.histogram).toEqual({ '0-10': 5, '10-20': 15 });
		});

		it('should handle all optional parameters', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => ({ metrics: [] })
			});

			await getRadarStats(1704067200, 1704153600, '1h', 'mph', 'America/New_York', 'radar_data');

			const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
			expect(callUrl).toContain('start=1704067200');
			expect(callUrl).toContain('end=1704153600');
			expect(callUrl).toContain('group=1h');
			expect(callUrl).toContain('units=mph');
			expect(callUrl).toContain('timezone=America%2FNew_York');
			expect(callUrl).toContain('source=radar_data');
		});

		it('should handle missing histogram in response', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => ({ metrics: [] })
			});

			const result = await getRadarStats(1704067200, 1704153600);

			expect(result.metrics).toEqual([]);
			expect(result.histogram).toBeUndefined();
		});

		it('should handle null histogram in response', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => ({ metrics: [], histogram: null })
			});

			const result = await getRadarStats(1704067200, 1704153600);

			expect(result.metrics).toEqual([]);
			expect(result.histogram).toBeUndefined();
		});

		it('should handle false histogram in response', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => ({ metrics: [], histogram: false })
			});

			const result = await getRadarStats(1704067200, 1704153600);

			expect(result.metrics).toEqual([]);
			expect(result.histogram).toBeUndefined();
		});

		it('should handle non-array metrics in response', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => ({ metrics: null })
			});

			const result = await getRadarStats(1704067200, 1704153600);

			expect(result.metrics).toEqual([]);
			expect(result.histogram).toBeUndefined();
		});

		it('should handle undefined histogram when payload is valid', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => ({ metrics: [], histogram: undefined })
			});

			const result = await getRadarStats(1704067200, 1704153600);

			expect(result.metrics).toEqual([]);
			expect(result.histogram).toBeUndefined();
		});

		it('should handle errors when fetching radar stats', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 503
			});

			await expect(getRadarStats(1704067200, 1704153600)).rejects.toThrow(
				'Failed to fetch radar stats: 503'
			);
		});
	});

	describe('getConfig', () => {
		it('should fetch configuration', async () => {
			const mockConfig: Config = {
				units: 'mph',
				timezone: 'America/New_York'
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockConfig
			});

			const result = await getConfig();

			expect(global.fetch).toHaveBeenCalled();
			expect(result).toEqual(mockConfig);
		});

		it('should handle errors when fetching config', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500
			});

			await expect(getConfig()).rejects.toThrow('Failed to fetch config: 500');
		});
	});

	describe('getSites', () => {
		it('should fetch sites list', async () => {
			const mockSites: Site[] = [
				{
					id: 1,
					name: 'Site A',
					location: 'Location A',
					cosine_error_angle: 5,
					speed_limit: 35,
					surveyor: 'John Doe',
					contact: 'john@example.com',
					include_map: false,
					created_at: '2025-01-01T00:00:00Z',
					updated_at: '2025-01-01T00:00:00Z'
				}
			];

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockSites
			});

			const result = await getSites();

			expect(global.fetch).toHaveBeenCalled();
			expect(result).toEqual(mockSites);
		});

		it('should handle errors when fetching sites', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500
			});

			await expect(getSites()).rejects.toThrow('Failed to fetch sites: 500');
		});
	});

	describe('generateReport', () => {
		it('should POST to generate report endpoint with all parameters', async () => {
			const mockResponse = { success: true, report_id: 123, message: 'Report generated' };
			const request = {
				start_date: '2025-01-01',
				end_date: '2025-01-31',
				timezone: 'UTC',
				units: 'mph'
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockResponse
			});

			const result = await generateReport(request);

			expect(global.fetch).toHaveBeenCalledWith(
				expect.stringContaining('/api/generate_report'),
				expect.objectContaining({
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(request)
				})
			);
			expect(result).toEqual(mockResponse);
		});

		it('should handle error responses', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 400,
				json: async () => ({ error: 'Invalid date range' })
			});

			await expect(
				generateReport({
					start_date: 'invalid',
					end_date: 'invalid',
					timezone: 'UTC',
					units: 'mph'
				})
			).rejects.toThrow('Invalid date range');
		});

		it('should handle error responses without JSON', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500,
				json: async () => {
					throw new Error('Not JSON');
				}
			});

			await expect(
				generateReport({
					start_date: '2025-01-01',
					end_date: '2025-01-31',
					timezone: 'UTC',
					units: 'mph'
				})
			).rejects.toThrow('HTTP 500');
		});
	});

	describe('getReport', () => {
		it('should fetch specific report by ID', async () => {
			const mockReport: SiteReport = {
				id: 123,
				site_id: 1,
				start_date: '2025-01-01',
				end_date: '2025-01-31',
				filepath: '/path/to/report.pdf',
				filename: 'report.pdf',
				run_id: 'run-123',
				timezone: 'UTC',
				units: 'mph',
				source: 'radar_data',
				created_at: '2025-01-01T00:00:00Z'
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockReport
			});

			const result = await getReport(123);

			expect(global.fetch).toHaveBeenCalled();
			expect(result).toEqual(mockReport);
		});

		it('should handle errors when fetching a report', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 404
			});

			await expect(getReport(999)).rejects.toThrow('Failed to fetch report: 404');
		});
	});

	describe('downloadReport', () => {
		it('should download PDF report and extract filename', async () => {
			const mockBlob = new Blob(['PDF content'], { type: 'application/pdf' });

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				headers: new Headers({
					'Content-Disposition': 'attachment; filename=test-report.pdf'
				}),
				blob: async () => mockBlob
			});

			const result = await downloadReport(123, 'pdf');

			expect(result.blob).toEqual(mockBlob);
			expect(result.filename).toBe('test-report.pdf');
		});

		it('should download PDF and trim whitespace from filename', async () => {
			const mockBlob = new Blob(['PDF content'], { type: 'application/pdf' });

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				headers: new Headers({
					'Content-Disposition': 'attachment; filename=  spaced-report.pdf  '
				}),
				blob: async () => mockBlob
			});

			const result = await downloadReport(123, 'pdf');

			expect(result.filename).toBe('spaced-report.pdf');
		});

		it('should download ZIP report', async () => {
			const mockBlob = new Blob(['ZIP content'], { type: 'application/zip' });

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				headers: new Headers({
					'Content-Disposition': 'attachment; filename=report-archive.zip'
				}),
				blob: async () => mockBlob
			});

			const result = await downloadReport(456, 'zip');

			expect(result.filename).toBe('report-archive.zip');
		});

		it('should use fallback filename when Content-Disposition is missing', async () => {
			const mockBlob = new Blob(['PDF content']);

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				headers: new Headers(),
				blob: async () => mockBlob
			});

			const result = await downloadReport(789, 'pdf');

			expect(result.filename).toBe('report.pdf');
		});

		it('should handle Content-Disposition without filename match', async () => {
			const mockBlob = new Blob(['PDF content']);

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				headers: new Headers({
					'Content-Disposition': 'attachment'
				}),
				blob: async () => mockBlob
			});

			const result = await downloadReport(789, 'pdf');

			expect(result.filename).toBe('report.pdf');
		});

		it('should handle download errors', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 404
			});

			await expect(downloadReport(999, 'pdf')).rejects.toThrow('Failed to download report: 404');
		});
	});

	describe('getRecentReports', () => {
		it('should fetch recent reports', async () => {
			const mockReports: SiteReport[] = [
				{
					id: 1,
					site_id: 1,
					start_date: '2025-01-01',
					end_date: '2025-01-31',
					filepath: '/path/to/report1.pdf',
					filename: 'report1.pdf',
					run_id: 'run-1',
					timezone: 'UTC',
					units: 'mph',
					source: 'radar_data',
					created_at: '2025-01-31T10:00:00Z'
				},
				{
					id: 2,
					site_id: 2,
					start_date: '2025-02-01',
					end_date: '2025-02-28',
					filepath: '/path/to/report2.pdf',
					filename: 'report2.pdf',
					run_id: 'run-2',
					timezone: 'America/New_York',
					units: 'kph',
					source: 'lidar_data',
					created_at: '2025-02-28T10:00:00Z'
				}
			];

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockReports
			});

			const result = await getRecentReports();

			expect(global.fetch).toHaveBeenCalledWith('/api/reports');
			expect(result).toEqual(mockReports);
		});

		it('should handle errors when fetching recent reports', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500
			});

			await expect(getRecentReports()).rejects.toThrow('Failed to fetch reports: 500');
		});
	});

	describe('getReportsForSite', () => {
		it('should fetch reports for a specific site', async () => {
			const mockReports: SiteReport[] = [
				{
					id: 1,
					site_id: 123,
					start_date: '2025-01-01',
					end_date: '2025-01-31',
					filepath: '/path/to/report.pdf',
					filename: 'site123-report.pdf',
					run_id: 'run-123',
					timezone: 'UTC',
					units: 'mph',
					source: 'radar_data',
					created_at: '2025-01-31T10:00:00Z'
				}
			];

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockReports
			});

			const result = await getReportsForSite(123);

			expect(global.fetch).toHaveBeenCalledWith('/api/reports/site/123');
			expect(result).toEqual(mockReports);
		});

		it('should handle errors when fetching site reports', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 404
			});

			await expect(getReportsForSite(999)).rejects.toThrow('Failed to fetch site reports: 404');
		});
	});

	describe('deleteReport', () => {
		it('should delete a report', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true
			});

			await deleteReport(123);

			expect(global.fetch).toHaveBeenCalledWith('/api/reports/123', { method: 'DELETE' });
		});

		it('should handle errors when deleting a report', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 403
			});

			await expect(deleteReport(123)).rejects.toThrow('Failed to delete report: 403');
		});
	});

	describe('getSite', () => {
		it('should fetch a specific site', async () => {
			const mockSite: Site = {
				id: 1,
				name: 'Test Site',
				location: 'Test Location',
				description: 'Test Description',
				cosine_error_angle: 15,
				speed_limit: 30,
				surveyor: 'John Doe',
				contact: 'john@example.com',
				address: '123 Main St',
				latitude: 51.5,
				longitude: -0.1,
				map_angle: 45,
				include_map: true,
				site_description: 'Site description',
				speed_limit_note: 'Note',
				created_at: '2025-01-01T00:00:00Z',
				updated_at: '2025-01-01T00:00:00Z'
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockSite
			});

			const result = await getSite(1);

			expect(global.fetch).toHaveBeenCalledWith('/api/sites/1');
			expect(result).toEqual(mockSite);
		});

		it('should handle errors when fetching a site', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 404
			});

			await expect(getSite(999)).rejects.toThrow('Failed to fetch site: 404');
		});
	});

	describe('createSite', () => {
		it('should create a new site', async () => {
			const newSite = {
				name: 'New Site',
				location: 'New Location',
				cosine_error_angle: 15,
				speed_limit: 30,
				surveyor: 'Jane Doe',
				contact: 'jane@example.com',
				include_map: false
			};
			const mockCreatedSite: Site = {
				id: 1,
				...newSite,
				description: null,
				address: null,
				latitude: null,
				longitude: null,
				map_angle: null,
				site_description: null,
				speed_limit_note: null,
				created_at: '2025-01-01T00:00:00Z',
				updated_at: '2025-01-01T00:00:00Z'
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockCreatedSite
			});

			const result = await createSite(newSite);

			expect(global.fetch).toHaveBeenCalledWith('/api/sites', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(newSite)
			});
			expect(result).toEqual(mockCreatedSite);
		});

		it('should handle errors when creating a site with error message', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 400,
				json: async () => ({ error: 'Invalid site data' })
			});

			await expect(createSite({ name: 'Bad Site' })).rejects.toThrow('Invalid site data');
		});

		it('should handle errors when creating a site without error message', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500,
				json: async () => {
					throw new Error('Not JSON');
				}
			});

			await expect(createSite({ name: 'Site' })).rejects.toThrow('HTTP 500');
		});

		it('should handle errors when creating a site with empty error field', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 400,
				json: async () => ({ error: '' })
			});

			await expect(createSite({ name: 'Site' })).rejects.toThrow('Failed to create site: 400');
		});

		it('should handle errors when creating a site with null error field', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 400,
				json: async () => ({ error: null })
			});

			await expect(createSite({ name: 'Site' })).rejects.toThrow('Failed to create site: 400');
		});
	});

	describe('updateSite', () => {
		it('should update an existing site', async () => {
			const updates = {
				name: 'Updated Site',
				speed_limit: 40
			};
			const mockUpdatedSite: Site = {
				id: 1,
				name: 'Updated Site',
				location: 'Location',
				description: null,
				cosine_error_angle: 15,
				speed_limit: 40,
				surveyor: 'John Doe',
				contact: 'john@example.com',
				address: null,
				latitude: null,
				longitude: null,
				map_angle: null,
				include_map: true,
				site_description: null,
				speed_limit_note: null,
				created_at: '2025-01-01T00:00:00Z',
				updated_at: '2025-01-02T00:00:00Z'
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockUpdatedSite
			});

			const result = await updateSite(1, updates);

			expect(global.fetch).toHaveBeenCalledWith('/api/sites/1', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(updates)
			});
			expect(result).toEqual(mockUpdatedSite);
		});

		it('should handle errors when updating a site with error message', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 404,
				json: async () => ({ error: 'Site not found' })
			});

			await expect(updateSite(999, { name: 'Updated' })).rejects.toThrow('Site not found');
		});

		it('should handle errors when updating a site without error message', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500,
				json: async () => {
					throw new Error('Not JSON');
				}
			});

			await expect(updateSite(1, { name: 'Updated' })).rejects.toThrow('HTTP 500');
		});

		it('should handle errors when updating a site with empty error field', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 404,
				json: async () => ({ error: '' })
			});

			await expect(updateSite(999, { name: 'Updated' })).rejects.toThrow(
				'Failed to update site: 404'
			);
		});
	});

	describe('deleteSite', () => {
		it('should delete a site', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true
			});

			await deleteSite(123);

			expect(global.fetch).toHaveBeenCalledWith('/api/sites/123', { method: 'DELETE' });
		});

		it('should handle errors when deleting a site with error message', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 409,
				json: async () => ({ error: 'Site has associated reports' })
			});

			await expect(deleteSite(123)).rejects.toThrow('Site has associated reports');
		});

		it('should handle errors when deleting a site without error message', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500,
				json: async () => {
					throw new Error('Not JSON');
				}
			});

			await expect(deleteSite(1)).rejects.toThrow('HTTP 500');
		});

		it('should handle errors when deleting a site with empty error field', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 409,
				json: async () => ({ error: '' })
			});

			await expect(deleteSite(123)).rejects.toThrow('Failed to delete site: 409');
		});
	});
});
