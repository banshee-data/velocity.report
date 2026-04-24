import type { StoredReportSettings } from '$lib/reportSettings';

import {
	buildReportRequest,
	DEFAULT_REPORT_BOUNDARY_THRESHOLD,
	DEFAULT_REPORT_HISTOGRAM_BUCKET_SIZE,
	DEFAULT_REPORT_MIN_SPEED,
	resolveDashboardReportFilters
} from './reportRequests';

describe('report request parity', () => {
	it('builds identical single-report payloads for dashboard and reports page when settings are fresh', () => {
		const savedAt = new Date(Date.now() - 60 * 60 * 1000).toISOString();
		const settings: StoredReportSettings = {
			dateRange: { savedAt },
			group: '24h',
			selectedSource: 'radar_data_transits',
			minSpeed: 5,
			maxSpeedCutoff: 35,
			boundaryThreshold: 9
		};

		const base = {
			startDate: '2025-07-01',
			endDate: '2025-08-31',
			timezone: 'America/Los_Angeles',
			units: 'mph',
			group: '24h',
			source: 'radar_data_transits',
			siteId: 1,
			paperSize: 'letter' as const
		};

		const dashboardRequest = buildReportRequest(base, resolveDashboardReportFilters(settings));
		const reportsPageRequest = buildReportRequest(base, {
			minSpeed: 5,
			maxSpeedCutoff: 35,
			boundaryThreshold: 9
		});

		expect(dashboardRequest).toEqual(reportsPageRequest);
		expect(dashboardRequest).toEqual({
			start_date: '2025-07-01',
			end_date: '2025-08-31',
			timezone: 'America/Los_Angeles',
			units: 'mph',
			group: '24h',
			source: 'radar_data_transits',
			min_speed: 5,
			hist_max: 35,
			boundary_threshold: 9,
			histogram: true,
			hist_bucket_size: DEFAULT_REPORT_HISTOGRAM_BUCKET_SIZE,
			site_id: 1,
			paper_size: 'letter'
		});
	});

	it('falls back to dashboard defaults when shared settings are stale', () => {
		const staleSavedAt = new Date(Date.now() - 19 * 60 * 60 * 1000).toISOString();
		const settings: StoredReportSettings = {
			dateRange: { savedAt: staleSavedAt },
			minSpeed: 0,
			maxSpeedCutoff: 35,
			boundaryThreshold: 9
		};

		expect(resolveDashboardReportFilters(settings)).toEqual({
			minSpeed: DEFAULT_REPORT_MIN_SPEED,
			maxSpeedCutoff: null,
			boundaryThreshold: DEFAULT_REPORT_BOUNDARY_THRESHOLD
		});
	});

	it('adds comparison fields when a comparison period is requested', () => {
		const request = buildReportRequest(
			{
				startDate: '2025-07-01',
				endDate: '2025-08-31',
				timezone: 'America/Los_Angeles',
				units: 'mph',
				group: '24h',
				source: 'radar_data_transits',
				siteId: 1,
				paperSize: 'letter'
			},
			{
				minSpeed: 5,
				maxSpeedCutoff: 35,
				boundaryThreshold: 5
			},
			{
				compareStartDate: '2025-05-01',
				compareEndDate: '2025-06-30',
				compareSource: 'radar_data_transits'
			}
		);

		expect(request).toMatchObject({
			compare_start_date: '2025-05-01',
			compare_end_date: '2025-06-30',
			compare_source: 'radar_data_transits'
		});
	});
});
