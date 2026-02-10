import {
	createSite,
	deleteReport,
	deleteSite,
	downloadReport,
	generateReport,
	getActiveTracks,
	getBackgroundGrid,
	getConfig,
	getEvents,
	getRadarStats,
	getRecentReports,
	getReport,
	getReportsForSite,
	getSite,
	getSites,
	getTimeline,
	getTrackById,
	getTrackHistory,
	getTrackObservations,
	getTrackObservationsRange,
	getTrackSummary,
	getTransitWorkerState,
	listSiteConfigPeriods,
	updateSite,
	updateTransitWorker,
	upsertSiteConfigPeriod,
	type Config,
	type Event,
	type Site,
	type SiteConfigPeriod,
	type SiteReport,
	type TransitWorkerState,
	type TransitWorkerUpdateResponse
} from './api';
import type {
	BackgroundGrid,
	Track,
	TrackHistoryResponse,
	TrackListResponse,
	TrackObservation,
	TrackSummaryResponse
} from './types/lidar';

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

			await getRadarStats(
				1704067200,
				1704153600,
				'1h',
				'mph',
				'America/New_York',
				'radar_data',
				42
			);

			const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
			expect(callUrl).toContain('start=1704067200');
			expect(callUrl).toContain('end=1704153600');
			expect(callUrl).toContain('group=1h');
			expect(callUrl).toContain('units=mph');
			expect(callUrl).toContain('timezone=America%2FNew_York');
			expect(callUrl).toContain('source=radar_data');
			expect(callUrl).toContain('site_id=42');
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

	describe('site config periods', () => {
		it('should list site config periods', async () => {
			const mockPeriods: SiteConfigPeriod[] = [
				{
					id: 1,
					site_id: 42,
					effective_start_unix: 0,
					effective_end_unix: null,
					is_active: true,
					notes: 'Initial',
					cosine_error_angle: 10
				}
			];

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockPeriods
			});

			const result = await listSiteConfigPeriods(42);

			expect(global.fetch).toHaveBeenCalled();
			const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
			expect(callUrl).toContain('/api/site_config_periods');
			expect(callUrl).toContain('site_id=42');
			expect(result).toEqual(mockPeriods);
		});

		it('should create or update site config period', async () => {
			const mockPeriod: SiteConfigPeriod = {
				site_id: 7,
				effective_start_unix: 1000,
				effective_end_unix: null,
				is_active: true,
				notes: null,
				cosine_error_angle: 12
			};
			const response = { ...mockPeriod, id: 99 };

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => response
			});

			const result = await upsertSiteConfigPeriod(mockPeriod);

			expect(global.fetch).toHaveBeenCalledWith(
				'/api/site_config_periods',
				expect.objectContaining({
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(mockPeriod)
				})
			);
			expect(result).toEqual(response);
		});

		it('should handle upsertSiteConfigPeriod error with JSON error message', async () => {
			const mockPeriod: SiteConfigPeriod = {
				site_id: 7,
				effective_start_unix: 1000,
				effective_end_unix: null,
				is_active: true,
				notes: null,
				cosine_error_angle: 12
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 400,
				json: async () => ({ error: 'Invalid period configuration' })
			});

			await expect(upsertSiteConfigPeriod(mockPeriod)).rejects.toThrow(
				'Invalid period configuration'
			);
		});

		it('should handle upsertSiteConfigPeriod error when JSON parsing fails', async () => {
			const mockPeriod: SiteConfigPeriod = {
				site_id: 7,
				effective_start_unix: 1000,
				effective_end_unix: null,
				is_active: true,
				notes: null,
				cosine_error_angle: 12
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500,
				json: async () => {
					throw new Error('Invalid JSON');
				}
			});

			await expect(upsertSiteConfigPeriod(mockPeriod)).rejects.toThrow('HTTP 500');
		});

		it('should fetch timeline', async () => {
			const mockTimeline = {
				site_id: 5,
				data_range: { start_unix: 100, end_unix: 200 },
				config_periods: [],
				unconfigured_periods: []
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockTimeline
			});

			const result = await getTimeline(5);

			const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
			expect(callUrl).toContain('/api/timeline');
			expect(callUrl).toContain('site_id=5');
			expect(result).toEqual(mockTimeline);
		});

		it('should handle errors when fetching timeline', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 404
			});

			await expect(getTimeline(999)).rejects.toThrow('Failed to fetch timeline: 404');
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

		it('should include min_speed parameter when provided', async () => {
			const mockResponse = { success: true, report_id: 456, message: 'Report generated' };
			const request = {
				start_date: '2025-01-01',
				end_date: '2025-01-31',
				timezone: 'UTC',
				units: 'mph',
				min_speed: 5.0
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

		it('should include boundary_threshold parameter when provided', async () => {
			const mockResponse = { success: true, report_id: 789, message: 'Report generated' };
			const request = {
				start_date: '2025-01-01',
				end_date: '2025-01-31',
				timezone: 'UTC',
				units: 'mph',
				boundary_threshold: 5
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

		it('should include both min_speed and boundary_threshold when provided', async () => {
			const mockResponse = { success: true, report_id: 999, message: 'Report generated' };
			const request = {
				start_date: '2025-01-01',
				end_date: '2025-01-31',
				timezone: 'UTC',
				units: 'mph',
				min_speed: 5.0,
				boundary_threshold: 5,
				site_id: 1,
				histogram: true,
				hist_bucket_size: 5.0
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

		it('should handle zero values for min_speed', async () => {
			const mockResponse = { success: true, report_id: 111, message: 'Report generated' };
			const request = {
				start_date: '2025-01-01',
				end_date: '2025-01-31',
				timezone: 'UTC',
				units: 'mph',
				min_speed: 0
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
					body: expect.stringContaining('"min_speed":0')
				})
			);
			expect(result).toEqual(mockResponse);
		});

		it('should handle zero values for boundary_threshold', async () => {
			const mockResponse = { success: true, report_id: 222, message: 'Report generated' };
			const request = {
				start_date: '2025-01-01',
				end_date: '2025-01-31',
				timezone: 'UTC',
				units: 'mph',
				boundary_threshold: 0
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
					body: expect.stringContaining('"boundary_threshold":0')
				})
			);
			expect(result).toEqual(mockResponse);
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

	describe('site map fields', () => {
		it('should create site with map fields', async () => {
			const testSVGData =
				'PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciPjxjaXJjbGUgY3g9IjUwIiBjeT0iNTAiIHI9IjQwIi8+PC9zdmc+'; // base64 SVG
			const newSite = {
				name: 'Map Site',
				location: 'Location with Map',
				surveyor: 'Test Surveyor',
				contact: 'test@example.com',
				latitude: 51.5074,
				longitude: -0.1278,
				map_angle: 45,
				bbox_ne_lat: 51.5124,
				bbox_ne_lng: -0.1228,
				bbox_sw_lat: 51.5024,
				bbox_sw_lng: -0.1328,
				map_svg_data: testSVGData,
				include_map: false
			};
			const mockCreatedSite: Site = {
				id: 1,
				...newSite,
				description: null,
				address: null,
				site_description: null,
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
			expect(result.bbox_ne_lat).toBe(51.5124);
			expect(result.bbox_ne_lng).toBe(-0.1228);
			expect(result.bbox_sw_lat).toBe(51.5024);
			expect(result.bbox_sw_lng).toBe(-0.1328);
			expect(result.map_svg_data).toBe(testSVGData);
		});

		it('should update site with map fields', async () => {
			const testSVGData =
				'PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciPjxyZWN0IHdpZHRoPSIxMDAiIGhlaWdodD0iMTAwIi8+PC9zdmc+';
			const updates = {
				bbox_ne_lat: 52.0,
				bbox_ne_lng: -1.0,
				bbox_sw_lat: 51.0,
				bbox_sw_lng: -2.0,
				map_svg_data: testSVGData
			};
			const mockUpdatedSite: Site = {
				id: 1,
				name: 'Site Name',
				location: 'Location',
				description: null,
				surveyor: 'John Doe',
				contact: 'john@example.com',
				address: null,
				latitude: 51.5,
				longitude: -0.1,
				map_angle: 45,
				include_map: true,
				site_description: null,
				bbox_ne_lat: 52.0,
				bbox_ne_lng: -1.0,
				bbox_sw_lat: 51.0,
				bbox_sw_lng: -2.0,
				map_svg_data: testSVGData,
				created_at: '2025-01-01T00:00:00Z',
				updated_at: '2025-01-02T00:00:00Z'
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockUpdatedSite
			});

			const result = await updateSite(1, updates);

			expect(result.bbox_ne_lat).toBe(52.0);
			expect(result.map_svg_data).toBe(testSVGData);
		});

		it('should retrieve site with null map fields', async () => {
			const mockSite: Site = {
				id: 1,
				name: 'Site Without Map',
				location: 'Test Location',
				description: null,
				surveyor: 'John Doe',
				contact: 'john@example.com',
				address: null,
				latitude: null,
				longitude: null,
				map_angle: null,
				include_map: false,
				site_description: null,
				bbox_ne_lat: null,
				bbox_ne_lng: null,
				bbox_sw_lat: null,
				bbox_sw_lng: null,
				map_svg_data: null,
				created_at: '2025-01-01T00:00:00Z',
				updated_at: '2025-01-01T00:00:00Z'
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockSite
			});

			const result = await getSite(1);

			expect(result.bbox_ne_lat).toBeNull();
			expect(result.bbox_ne_lng).toBeNull();
			expect(result.bbox_sw_lat).toBeNull();
			expect(result.bbox_sw_lng).toBeNull();
			expect(result.map_svg_data).toBeNull();
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

	describe('getTransitWorkerState', () => {
		it('should fetch transit worker state', async () => {
			const mockState: TransitWorkerState = {
				enabled: true,
				last_run_at: '2024-01-01T12:00:00Z',
				current_run: {
					started_at: '2024-01-01T12:30:00Z',
					trigger: 'manual'
				},
				last_run: {
					started_at: '2024-01-01T12:00:00Z',
					finished_at: '2024-01-01T12:01:00Z',
					duration_ms: 60000,
					trigger: 'periodic'
				},
				run_count: 5,
				is_healthy: true
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockState
			});

			const result = await getTransitWorkerState();

			expect(global.fetch).toHaveBeenCalledWith('/api/transit_worker');
			expect(result).toEqual(mockState);
		});

		it('should handle errors when fetching transit worker state', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 503
			});

			await expect(getTransitWorkerState()).rejects.toThrow(
				'Failed to fetch transit worker state: 503'
			);
		});
	});

	describe('updateTransitWorker', () => {
		it('should update transit worker with enabled only', async () => {
			const mockResponse: TransitWorkerUpdateResponse = {
				enabled: true,
				last_run_at: '2024-01-01T12:00:00Z',
				run_count: 6,
				is_healthy: true
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockResponse
			});

			const result = await updateTransitWorker({ enabled: true });

			expect(global.fetch).toHaveBeenCalledWith('/api/transit_worker', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ enabled: true })
			});
			expect(result).toEqual(mockResponse);
		});

		it('should update transit worker with trigger only', async () => {
			const mockResponse: TransitWorkerUpdateResponse = {
				enabled: true,
				last_run_at: '2024-01-01T12:01:00Z',
				run_count: 7,
				is_healthy: true
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockResponse
			});

			const result = await updateTransitWorker({ trigger: true });

			expect(global.fetch).toHaveBeenCalledWith('/api/transit_worker', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ trigger: true })
			});
			expect(result).toEqual(mockResponse);
		});

		it('should update transit worker with both enabled and trigger', async () => {
			const mockResponse: TransitWorkerUpdateResponse = {
				enabled: true,
				last_run_at: '2024-01-01T12:02:00Z',
				run_count: 8,
				is_healthy: true
			};

			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: true,
				json: async () => mockResponse
			});

			const result = await updateTransitWorker({ enabled: true, trigger: true });

			expect(global.fetch).toHaveBeenCalledWith('/api/transit_worker', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ enabled: true, trigger: true })
			});
			expect(result).toEqual(mockResponse);
		});

		it('should handle errors with error message', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 400,
				json: async () => ({ error: 'Invalid request' })
			});

			await expect(updateTransitWorker({ enabled: false })).rejects.toThrow('Invalid request');
		});

		it('should handle errors without error message', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 500,
				json: async () => {
					throw new Error('Not JSON');
				}
			});

			await expect(updateTransitWorker({ enabled: true })).rejects.toThrow('HTTP 500');
		});

		it('should handle errors with empty error field', async () => {
			(global.fetch as jest.Mock).mockResolvedValueOnce({
				ok: false,
				status: 503,
				json: async () => ({ error: '' })
			});

			await expect(updateTransitWorker({ trigger: true })).rejects.toThrow(
				'Failed to update transit worker: 503'
			);
		});
	});

	describe('LiDAR API', () => {
		describe('getActiveTracks', () => {
			it('should fetch active tracks without state filter', async () => {
				const mockResponse: TrackListResponse = {
					tracks: [
						{
							track_id: 'track-123',
							sensor_id: 'hesai-pandar40p',
							state: 'confirmed',
							position: { x: 10.5, y: 5.2, z: 0.5 },
							velocity: { vx: 2.5, vy: 1.2 },
							speed_mps: 2.77,
							heading_rad: 0.42,
							object_class: 'car',
							object_confidence: 0.95,
							observation_count: 15,
							age_seconds: 3.5,
							avg_speed_mps: 2.5,
							peak_speed_mps: 3.2,
							bounding_box: {
								length_avg: 4.5,
								width_avg: 2.0,
								height_avg: 1.5
							},
							first_seen: '2025-12-09T10:00:00Z',
							last_seen: '2025-12-09T10:00:03Z'
						}
					],
					count: 1,
					timestamp: '2025-12-09T10:00:03Z'
				};

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});

				const result = await getActiveTracks('hesai-pandar40p');

				expect(global.fetch).toHaveBeenCalled();
				const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
				expect(callUrl).toContain('/api/lidar/tracks/active');
				expect(callUrl).toContain('sensor_id=hesai-pandar40p');
				expect(callUrl).not.toContain('state=');
				expect(result).toEqual(mockResponse);
			});

			it('should update transit worker with full history trigger', async () => {
				const mockResponse: TransitWorkerUpdateResponse = {
					enabled: true,
					last_run_at: '2024-01-01T12:10:00Z',
					run_count: 9,
					is_healthy: true,
					current_run: {
						started_at: '2024-01-01T12:10:00Z',
						trigger: 'full-history'
					}
				};

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});

				const result = await updateTransitWorker({ trigger_full_history: true });

				expect(global.fetch).toHaveBeenCalledWith('/api/transit_worker', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ trigger_full_history: true })
				});
				expect(result).toEqual(mockResponse);
			});

			it('should fetch active tracks with state filter', async () => {
				const mockResponse: TrackListResponse = {
					tracks: [],
					count: 0,
					timestamp: '2025-12-09T10:00:00Z'
				};

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});

				const result = await getActiveTracks('hesai-pandar40p', 'confirmed');

				expect(global.fetch).toHaveBeenCalled();
				const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
				expect(callUrl).toContain('sensor_id=hesai-pandar40p');
				expect(callUrl).toContain('state=confirmed');
				expect(result).toEqual(mockResponse);
			});

			it('should handle errors when fetching active tracks', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: false,
					status: 500
				});

				await expect(getActiveTracks('hesai-pandar40p')).rejects.toThrow(
					'Failed to fetch active tracks: 500'
				);
			});
		});

		describe('getTrackById', () => {
			it('should fetch a specific track by ID', async () => {
				const mockTrack: Track = {
					track_id: 'track-456',
					sensor_id: 'hesai-pandar40p',
					state: 'confirmed',
					position: { x: 15.0, y: 8.0, z: 0.6 },
					velocity: { vx: 3.0, vy: 0.5 },
					speed_mps: 3.04,
					heading_rad: 0.16,
					object_class: 'pedestrian',
					object_confidence: 0.88,
					observation_count: 20,
					age_seconds: 5.0,
					avg_speed_mps: 2.8,
					peak_speed_mps: 3.5,
					bounding_box: {
						length_avg: 0.6,
						width_avg: 0.4,
						height_avg: 1.7
					},
					first_seen: '2025-12-09T10:00:00Z',
					last_seen: '2025-12-09T10:00:05Z'
				};

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockTrack
				});

				const result = await getTrackById('track-456');

				expect(global.fetch).toHaveBeenCalledWith('/api/lidar/tracks/track-456');
				expect(result).toEqual(mockTrack);
			});

			it('should handle errors when fetching a track', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: false,
					status: 404
				});

				await expect(getTrackById('nonexistent')).rejects.toThrow('Failed to fetch track: 404');
			});
		});

		describe('getTrackObservations', () => {
			it('should fetch observations for a track', async () => {
				const mockObservations: TrackObservation[] = [
					{
						track_id: 'track-789',
						timestamp: '2025-12-09T10:00:00Z',
						position: { x: 10.0, y: 5.0, z: 0.5 },
						velocity: { vx: 2.0, vy: 1.0 },
						speed_mps: 2.24,
						heading_rad: 0.46,
						bounding_box: {
							length: 4.5,
							width: 2.0,
							height: 1.5
						}
					},
					{
						track_id: 'track-789',
						timestamp: '2025-12-09T10:00:01Z',
						position: { x: 12.0, y: 6.0, z: 0.5 },
						velocity: { vx: 2.0, vy: 1.0 },
						speed_mps: 2.24,
						heading_rad: 0.46,
						bounding_box: {
							length: 4.5,
							width: 2.0,
							height: 1.5
						}
					}
				];

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockObservations
				});

				const result = await getTrackObservations('track-789');

				expect(global.fetch).toHaveBeenCalledWith('/api/lidar/tracks/track-789/observations');
				expect(result).toEqual(mockObservations);
			});

			it('should handle errors when fetching track observations', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: false,
					status: 404
				});

				await expect(getTrackObservations('nonexistent')).rejects.toThrow(
					'Failed to fetch track observations: 404'
				);
			});
		});

		describe('getTrackObservationsRange', () => {
			it('should fetch observations for a sensor within time range', async () => {
				const mockResponse = {
					observations: [
						{
							track_id: 'track-789',
							timestamp: '2025-12-09T10:00:00Z',
							position: { x: 5.0, y: 3.0, z: 0.5 },
							velocity: { vx: 0.8, vy: 0.3 },
							speed_mps: 0.85,
							heading_rad: 0.36,
							bounding_box: { length: 0.5, width: 0.3, height: 1.7 }
						}
					],
					count: 1,
					timestamp: '2025-12-09T10:00:00Z'
				};

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});

				const result = await getTrackObservationsRange('hesai-pandar40p', 1000000, 2000000);

				expect(global.fetch).toHaveBeenCalled();
				const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
				expect(callUrl).toContain('/api/lidar/observations');
				expect(callUrl).toContain('sensor_id=hesai-pandar40p');
				expect(callUrl).toContain('start_time=1000000');
				expect(callUrl).toContain('end_time=2000000');
				expect(callUrl).toContain('limit=2000');
				expect(result).toEqual(mockResponse);
			});

			it('should include optional track_id parameter when provided', async () => {
				const mockResponse = { observations: [], count: 0, timestamp: '2025-12-09T10:00:00Z' };

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});

				await getTrackObservationsRange('hesai-pandar40p', 1000000, 2000000, 500, 'track-123');

				const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
				expect(callUrl).toContain('track_id=track-123');
				expect(callUrl).toContain('limit=500');
			});

			it('should handle errors when fetching track observations range', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: false,
					status: 500
				});

				await expect(
					getTrackObservationsRange('hesai-pandar40p', 1000000, 2000000)
				).rejects.toThrow('Failed to fetch track observations range: 500');
			});
		});

		describe('getTrackHistory', () => {
			it('should fetch historical tracks with correct URL parameters', async () => {
				const mockResponse: TrackHistoryResponse = {
					tracks: [
						{
							track_id: 'track-101',
							sensor_id: 'hesai-pandar40p',
							state: 'confirmed',
							position: { x: 20.0, y: 10.0, z: 0.7 },
							velocity: { vx: 1.5, vy: 0.8 },
							speed_mps: 1.7,
							heading_rad: 0.49,
							object_class: 'car',
							object_confidence: 0.92,
							observation_count: 25,
							age_seconds: 6.0,
							avg_speed_mps: 1.6,
							peak_speed_mps: 2.0,
							bounding_box: {
								length_avg: 4.5,
								width_avg: 2.0,
								height_avg: 1.5
							},
							first_seen: '2025-12-09T09:00:00Z',
							last_seen: '2025-12-09T09:00:06Z'
						}
					],
					observations: {
						'track-101': [
							{
								track_id: 'track-101',
								timestamp: '2025-12-09T09:00:00Z',
								position: { x: 20.0, y: 10.0, z: 0.7 },
								velocity: { vx: 1.5, vy: 0.8 },
								speed_mps: 1.7,
								heading_rad: 0.49,
								bounding_box: {
									length: 4.5,
									width: 2.0,
									height: 1.5
								}
							}
						]
					}
				};

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});

				const startTime = 1733745600000000000; // Unix nanoseconds
				const endTime = 1733832000000000000;
				const result = await getTrackHistory('hesai-pandar40p', startTime, endTime);

				expect(global.fetch).toHaveBeenCalled();
				const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
				expect(callUrl).toContain('/api/lidar/tracks/history');
				expect(callUrl).toContain('sensor_id=hesai-pandar40p');
				expect(callUrl).toContain(`start_time=${startTime}`);
				expect(callUrl).toContain(`end_time=${endTime}`);
				expect(result).toEqual(mockResponse);
			});

			it('should handle errors when fetching track history', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: false,
					status: 400
				});

				await expect(getTrackHistory('hesai-pandar40p', 0, 1000000)).rejects.toThrow(
					'Failed to fetch track history: 400'
				);
			});
		});

		describe('getTrackSummary', () => {
			it('should fetch track summary statistics', async () => {
				const mockSummary: TrackSummaryResponse = {
					by_class: {
						car: {
							count: 10,
							avg_speed_mps: 8.5,
							max_speed_mps: 15.0,
							avg_duration_seconds: 5.5
						},
						pedestrian: {
							count: 5,
							avg_speed_mps: 1.5,
							max_speed_mps: 2.5,
							avg_duration_seconds: 8.0
						}
					},
					by_state: {
						confirmed: {
							count: 12,
							avg_age_seconds: 6.0
						},
						tentative: {
							count: 3,
							avg_age_seconds: 1.5
						}
					},
					total_tracks: 15,
					timestamp: '2025-12-09T10:00:00Z'
				};

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockSummary
				});

				const result = await getTrackSummary('hesai-pandar40p');

				expect(global.fetch).toHaveBeenCalled();
				const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
				expect(callUrl).toContain('/api/lidar/tracks/summary');
				expect(callUrl).toContain('sensor_id=hesai-pandar40p');
				expect(result).toEqual(mockSummary);
			});

			it('should handle errors when fetching track summary', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: false,
					status: 503
				});

				await expect(getTrackSummary('hesai-pandar40p')).rejects.toThrow(
					'Failed to fetch track summary: 503'
				);
			});
		});

		describe('getBackgroundGrid', () => {
			it('should fetch background grid data', async () => {
				const mockGrid: BackgroundGrid = {
					sensor_id: 'hesai-pandar40p',
					timestamp: '2025-12-09T10:00:00Z',
					rings: 40,
					azimuth_bins: 1800,
					cells: [
						{
							x: 10.5,
							y: 5.2,
							range_spread_meters: 0.15,
							times_seen: 100
						},
						{
							x: 12.0,
							y: 6.0,
							range_spread_meters: 0.2,
							times_seen: 95
						}
					]
				};

				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockGrid
				});

				const result = await getBackgroundGrid('hesai-pandar40p');

				expect(global.fetch).toHaveBeenCalled();
				const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
				expect(callUrl).toContain('/api/lidar/background/grid');
				expect(callUrl).toContain('sensor_id=hesai-pandar40p');
				expect(result).toEqual(mockGrid);
			});

			it('should handle errors when fetching background grid', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: false,
					status: 500
				});

				await expect(getBackgroundGrid('hesai-pandar40p')).rejects.toThrow(
					'Failed to fetch background grid: 500'
				);
			});
		});
	});

	// Phase 3-6: LiDAR labelling and transit API tests
	describe('LiDAR labelling API', () => {
		describe('getLidarScenes', () => {
			it('should fetch scenes without filter', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => ({ scenes: [{ scene_id: 's1', sensor_id: 'sensor1' }] })
				});
				const { getLidarScenes } = await import('./api');
				const result = await getLidarScenes();
				expect(result).toEqual([{ scene_id: 's1', sensor_id: 'sensor1' }]);
			});

			it('should fetch scenes with sensor_id filter', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => ({ scenes: [] })
				});
				const { getLidarScenes } = await import('./api');
				const result = await getLidarScenes('hesai');
				expect(result).toEqual([]);
				const callUrl = (global.fetch as jest.Mock).mock.calls[0][0].toString();
				expect(callUrl).toContain('sensor_id=hesai');
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 500 });
				const { getLidarScenes } = await import('./api');
				await expect(getLidarScenes()).rejects.toThrow('Failed to fetch scenes: 500');
			});
		});

		describe('getLidarRuns', () => {
			it('should fetch runs with filters', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => ({ runs: [{ run_id: 'r1' }] })
				});
				const { getLidarRuns } = await import('./api');
				const result = await getLidarRuns({ sensor_id: 'hesai', status: 'complete', limit: 10 });
				expect(result).toEqual([{ run_id: 'r1' }]);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 500 });
				const { getLidarRuns } = await import('./api');
				await expect(getLidarRuns()).rejects.toThrow('Failed to fetch runs: 500');
			});
		});

		describe('getRunTracks', () => {
			it('should fetch run tracks', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => ({ tracks: [{ track_id: 't1', user_label: 'good_vehicle' }] })
				});
				const { getRunTracks } = await import('./api');
				const result = await getRunTracks('run-001');
				expect(result).toEqual([{ track_id: 't1', user_label: 'good_vehicle' }]);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 404 });
				const { getRunTracks } = await import('./api');
				await expect(getRunTracks('bad-id')).rejects.toThrow('Failed to fetch tracks: 404');
			});
		});

		describe('updateTrackLabel', () => {
			it('should update label via PUT', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: true });
				const { updateTrackLabel } = await import('./api');
				await updateTrackLabel('run-001', 'track-001', {
					user_label: 'good_vehicle',
					quality_label: 'perfect'
				});
				const call = (global.fetch as jest.Mock).mock.calls[0];
				expect(call[1].method).toBe('PUT');
				expect(JSON.parse(call[1].body)).toEqual({
					user_label: 'good_vehicle',
					quality_label: 'perfect'
				});
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 400 });
				const { updateTrackLabel } = await import('./api');
				await expect(updateTrackLabel('r', 't', { user_label: 'invalid' })).rejects.toThrow(
					'Failed to update label: 400'
				);
			});
		});

		describe('updateTrackFlags', () => {
			it('should update flags via PUT', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: true });
				const { updateTrackFlags } = await import('./api');
				await updateTrackFlags('run-001', 'track-001', {
					linked_track_ids: ['t2'],
					user_label: 'split'
				});
				const call = (global.fetch as jest.Mock).mock.calls[0];
				expect(call[1].method).toBe('PUT');
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 500 });
				const { updateTrackFlags } = await import('./api');
				await expect(updateTrackFlags('r', 't', { linked_track_ids: [] })).rejects.toThrow(
					'Failed to update flags: 500'
				);
			});
		});

		describe('getLabellingProgress', () => {
			it('should fetch labelling progress', async () => {
				const mockProgress = { total: 100, labelled: 50, progress_pct: 50.0, by_class: {} };
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockProgress
				});
				const { getLabellingProgress } = await import('./api');
				const result = await getLabellingProgress('run-001');
				expect(result).toEqual(mockProgress);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 404 });
				const { getLabellingProgress } = await import('./api');
				await expect(getLabellingProgress('bad-id')).rejects.toThrow(
					'Failed to fetch progress: 404'
				);
			});
		});
	});

	describe('LiDAR Scene Management', () => {
		describe('getLidarScene', () => {
			it('should fetch a single scene by ID', async () => {
				const mockScene = {
					scene_id: 'scene-001',
					sensor_id: 'hesai-pandar40p',
					pcap_file: 'test.pcap',
					description: 'Test scene',
					created_at_ns: 1000000000
				};
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockScene
				});
				const { getLidarScene } = await import('./api');
				const result = await getLidarScene('scene-001');
				expect(result).toEqual(mockScene);
				expect(global.fetch).toHaveBeenCalledWith('/api/lidar/scenes/scene-001');
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 404 });
				const { getLidarScene } = await import('./api');
				await expect(getLidarScene('bad-id')).rejects.toThrow('Failed to fetch scene: 404');
			});
		});

		describe('createLidarScene', () => {
			it('should create a new scene', async () => {
				const newScene = {
					sensor_id: 'hesai-pandar40p',
					pcap_file: 'new.pcap',
					description: 'New test scene'
				};
				const mockResponse = {
					...newScene,
					scene_id: 'scene-new',
					created_at_ns: 1000000000
				};
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});
				const { createLidarScene } = await import('./api');
				const result = await createLidarScene(newScene);
				expect(result).toEqual(mockResponse);
				expect(global.fetch).toHaveBeenCalledWith(
					'/api/lidar/scenes',
					expect.objectContaining({
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify(newScene)
					})
				);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 400 });
				const { createLidarScene } = await import('./api');
				await expect(
					createLidarScene({
						sensor_id: 'hesai-pandar40p',
						pcap_file: 'test.pcap'
					})
				).rejects.toThrow('Failed to create scene: 400');
			});
		});

		describe('updateLidarScene', () => {
			it('should update an existing scene', async () => {
				const update = {
					description: 'Updated description',
					reference_run_id: 'run-001'
				};
				const mockResponse = {
					scene_id: 'scene-001',
					sensor_id: 'hesai-pandar40p',
					pcap_file: 'test.pcap',
					...update,
					created_at_ns: 1000000000
				};
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});
				const { updateLidarScene } = await import('./api');
				const result = await updateLidarScene('scene-001', update);
				expect(result).toEqual(mockResponse);
				expect(global.fetch).toHaveBeenCalledWith(
					'/api/lidar/scenes/scene-001',
					expect.objectContaining({
						method: 'PUT',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify(update)
					})
				);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 500 });
				const { updateLidarScene } = await import('./api');
				await expect(updateLidarScene('scene-001', {})).rejects.toThrow(
					'Failed to update scene: 500'
				);
			});
		});

		describe('deleteLidarScene', () => {
			it('should delete a scene', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: true });
				const { deleteLidarScene } = await import('./api');
				await deleteLidarScene('scene-001');
				expect(global.fetch).toHaveBeenCalledWith(
					'/api/lidar/scenes/scene-001',
					expect.objectContaining({
						method: 'DELETE'
					})
				);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 404 });
				const { deleteLidarScene } = await import('./api');
				await expect(deleteLidarScene('scene-001')).rejects.toThrow('Failed to delete scene: 404');
			});
		});

		describe('scanPcapFiles', () => {
			it('should scan for PCAP files', async () => {
				const mockResponse = {
					pcap_dir: '/data/pcaps',
					files: [
						{ path: 'file1.pcap', size_bytes: 1024, modified_at: '2024-01-01', in_use: false },
						{ path: 'file2.pcap', size_bytes: 2048, modified_at: '2024-01-02', in_use: true }
					],
					count: 2
				};
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});
				const { scanPcapFiles } = await import('./api');
				const result = await scanPcapFiles();
				expect(result).toEqual(mockResponse);
				expect(global.fetch).toHaveBeenCalledWith('/api/lidar/pcap/files');
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 500 });
				const { scanPcapFiles } = await import('./api');
				await expect(scanPcapFiles()).rejects.toThrow('Failed to scan PCAP files: 500');
			});
		});
	});

	describe('LiDAR Missed Regions', () => {
		describe('getMissedRegions', () => {
			it('should fetch missed regions for a run', async () => {
				const mockRegions = [
					{
						region_id: 'region-001',
						run_id: 'run-001',
						center_x: 10.5,
						center_y: 20.3,
						radius_m: 5.0,
						time_start_ns: 1000000000,
						time_end_ns: 2000000000,
						expected_label: 'vehicle',
						notes: 'Missed detection'
					}
				];
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => ({ regions: mockRegions })
				});
				const { getMissedRegions } = await import('./api');
				const result = await getMissedRegions('run-001');
				expect(result).toEqual(mockRegions);
				expect(global.fetch).toHaveBeenCalledWith('/api/lidar/runs/run-001/missed-regions');
			});

			it('should return empty array if no regions key in response', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => ({})
				});
				const { getMissedRegions } = await import('./api');
				const result = await getMissedRegions('run-001');
				expect(result).toEqual([]);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 404 });
				const { getMissedRegions } = await import('./api');
				await expect(getMissedRegions('run-001')).rejects.toThrow(
					'Failed to fetch missed regions: 404'
				);
			});
		});

		describe('createMissedRegion', () => {
			it('should create a new missed region', async () => {
				const newRegion = {
					center_x: 10.5,
					center_y: 20.3,
					radius_m: 5.0,
					time_start_ns: 1000000000,
					time_end_ns: 2000000000,
					expected_label: 'vehicle',
					notes: 'Test missed region'
				};
				const mockResponse = {
					...newRegion,
					region_id: 'region-new',
					run_id: 'run-001'
				};
				(global.fetch as jest.Mock).mockResolvedValueOnce({
					ok: true,
					json: async () => mockResponse
				});
				const { createMissedRegion } = await import('./api');
				const result = await createMissedRegion('run-001', newRegion);
				expect(result).toEqual(mockResponse);
				expect(global.fetch).toHaveBeenCalledWith(
					'/api/lidar/runs/run-001/missed-regions',
					expect.objectContaining({
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify(newRegion)
					})
				);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 400 });
				const { createMissedRegion } = await import('./api');
				await expect(
					createMissedRegion('run-001', {
						center_x: 10.5,
						center_y: 20.3,
						time_start_ns: 1000000000,
						time_end_ns: 2000000000
					})
				).rejects.toThrow('Failed to create missed region: 400');
			});
		});

		describe('deleteMissedRegion', () => {
			it('should delete a missed region', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: true });
				const { deleteMissedRegion } = await import('./api');
				await deleteMissedRegion('run-001', 'region-001');
				expect(global.fetch).toHaveBeenCalledWith(
					'/api/lidar/runs/run-001/missed-regions/region-001',
					expect.objectContaining({
						method: 'DELETE'
					})
				);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 404 });
				const { deleteMissedRegion } = await import('./api');
				await expect(deleteMissedRegion('run-001', 'region-001')).rejects.toThrow(
					'Failed to delete missed region: 404'
				);
			});
		});

		describe('deleteAllRuns', () => {
			it('should delete all runs for a sensor', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: true });
				const { deleteAllRuns } = await import('./api');
				await deleteAllRuns('test-sensor');
				expect(global.fetch).toHaveBeenCalledWith(
					expect.objectContaining({
						href: expect.stringContaining('/api/lidar/runs/clear?sensor_id=test-sensor')
					}),
					expect.objectContaining({
						method: 'POST'
					})
				);
			});

			it('should delete all runs without sensor_id', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: true });
				const { deleteAllRuns } = await import('./api');
				await deleteAllRuns();
				expect(global.fetch).toHaveBeenCalledWith(
					expect.objectContaining({
						href: expect.stringContaining('/api/lidar/runs/clear')
					}),
					expect.objectContaining({
						method: 'POST'
					})
				);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 500 });
				const { deleteAllRuns } = await import('./api');
				await expect(deleteAllRuns('test-sensor')).rejects.toThrow(
					'Failed to delete all runs: 500'
				);
			});
		});

		describe('deleteRunTrack', () => {
			it('should delete a specific run track', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: true });
				const { deleteRunTrack } = await import('./api');
				await deleteRunTrack('run-001', 'track-001');
				expect(global.fetch).toHaveBeenCalledWith(
					'/api/lidar/runs/run-001/tracks/track-001',
					expect.objectContaining({
						method: 'DELETE'
					})
				);
			});

			it('should handle errors', async () => {
				(global.fetch as jest.Mock).mockResolvedValueOnce({ ok: false, status: 404 });
				const { deleteRunTrack } = await import('./api');
				await expect(deleteRunTrack('run-001', 'track-001')).rejects.toThrow(
					'Failed to delete run track: 404'
				);
			});
		});
	});
});
