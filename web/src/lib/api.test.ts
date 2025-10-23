import {
  downloadReport,
  generateReport,
  getConfig,
  getEvents,
  getRadarStats,
  getReport,
  getSites,
  type Config,
  type Event
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

      expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining('/api/events'));
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
	});
});
