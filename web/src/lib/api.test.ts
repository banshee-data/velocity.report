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
        hourly_avg_speed: 15.5,
        daily_avg_speed: 14.2,
        total_detections: 1000
      };

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => serverResponse
      });

      const result = await getRadarStats('2025-01-01', '2025-01-02', 'mph');

      expect(result).toEqual({
        hourlyAvgSpeed: 15.5,
        dailyAvgSpeed: 14.2,
        totalDetections: 1000
      });
    });

    it('should use correct API endpoint with parameters', async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          hourly_avg_speed: 0,
          daily_avg_speed: 0,
          total_detections: 0
        })
      });

      await getRadarStats('2025-01-01', '2025-01-31', 'kmph');

      expect(global.fetch).toHaveBeenCalledWith(
        'http://localhost:3000/api/radar/stats?start=2025-01-01&end=2025-01-31&units=kmph'
      );
    });

    it('should handle missing properties in server response', async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => ({})
      });

      const result = await getRadarStats('2025-01-01', '2025-01-02', 'mps');

      expect(result).toEqual({
        hourlyAvgSpeed: undefined,
        dailyAvgSpeed: undefined,
        totalDetections: undefined
      });
    });
  });

  describe('getConfig', () => {
    it('should fetch configuration', async () => {
      const mockConfig: Config = {
        radarEnabled: true,
        lidarEnabled: false,
        detectionThreshold: 10.0
      };

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => mockConfig
      });

      const result = await getConfig();

      expect(global.fetch).toHaveBeenCalledWith('http://localhost:3000/api/config');
      expect(result).toEqual(mockConfig);
    });
  });

  describe('getSites', () => {
    it('should fetch sites list', async () => {
      const mockSites = [
        { id: 1, name: 'Site A', location: 'Location A' },
        { id: 2, name: 'Site B', location: 'Location B' }
      ];

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => mockSites
      });

      const result = await getSites();

      expect(global.fetch).toHaveBeenCalledWith('http://localhost:3000/api/sites');
      expect(result).toEqual(mockSites);
    });
  });

  describe('generateReport', () => {
    it('should POST to generate report endpoint', async () => {
      const mockResponse = { id: '123', status: 'processing' };

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse
      });

      const result = await generateReport('2025-01-01', '2025-01-31');

      expect(global.fetch).toHaveBeenCalledWith('http://localhost:3000/api/reports/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ start: '2025-01-01', end: '2025-01-31' })
      });
      expect(result).toEqual(mockResponse);
    });

    it('should handle POST errors', async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        statusText: 'Bad Request'
      });

      await expect(generateReport('invalid', 'dates')).rejects.toThrow();
    });
  });

  describe('getReport', () => {
    it('should fetch specific report by ID', async () => {
      const mockReport = {
        id: '123',
        status: 'complete',
        data: { events: 100 }
      };

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => mockReport
      });

      const result = await getReport('123');

      expect(global.fetch).toHaveBeenCalledWith('http://localhost:3000/api/reports/123');
      expect(result).toEqual(mockReport);
    });
  });

  describe('downloadReport', () => {
    it('should return download URL with filename', () => {
      const url = downloadReport('456', 'test-report.pdf');

      expect(url).toBe('http://localhost:3000/api/reports/456/download/test-report.pdf');
    });

    it('should handle reports without filename extension', () => {
      const url = downloadReport('789', 'report');

      expect(url).toBe('http://localhost:3000/api/reports/789/download/report');
    });

    it('should encode special characters in filename', () => {
      const url = downloadReport('999', 'report with spaces.pdf');

      expect(url).toContain('report%20with%20spaces.pdf');
    });
  });
});
