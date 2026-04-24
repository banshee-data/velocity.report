import type { ReportRequest } from '$lib/api';
import { areStoredReportSettingsFresh, type StoredReportSettings } from '$lib/reportSettings';

export const DEFAULT_REPORT_MIN_SPEED = 5;
export const DEFAULT_REPORT_BOUNDARY_THRESHOLD = 5;
export const DEFAULT_REPORT_HISTOGRAM_BUCKET_SIZE = 5.0;

export type ReportFilters = {
	minSpeed: number;
	maxSpeedCutoff: number | null;
	boundaryThreshold: number;
};

export type ReportRequestBase = {
	siteId: number;
	startDate: string;
	endDate: string;
	timezone: string;
	units: string;
	group: string;
	source: string;
	paperSize?: ReportRequest['paper_size'];
};

export type ReportComparison = {
	compareStartDate: string;
	compareEndDate: string;
	compareSource: string;
};

export function resolveDashboardReportFilters(
	settings: StoredReportSettings | null | undefined
): ReportFilters {
	const freshSettings = areStoredReportSettingsFresh(settings) ? settings : null;

	return {
		minSpeed:
			typeof freshSettings?.minSpeed === 'number'
				? freshSettings.minSpeed
				: DEFAULT_REPORT_MIN_SPEED,
		maxSpeedCutoff:
			typeof freshSettings?.maxSpeedCutoff === 'number' || freshSettings?.maxSpeedCutoff === null
				? freshSettings.maxSpeedCutoff
				: null,
		boundaryThreshold:
			typeof freshSettings?.boundaryThreshold === 'number'
				? freshSettings.boundaryThreshold
				: DEFAULT_REPORT_BOUNDARY_THRESHOLD
	};
}

export function buildReportRequest(
	base: ReportRequestBase,
	filters: ReportFilters,
	comparison?: ReportComparison
): ReportRequest {
	const request: ReportRequest = {
		start_date: base.startDate,
		end_date: base.endDate,
		timezone: base.timezone,
		units: base.units,
		group: base.group,
		source: base.source,
		min_speed: filters.minSpeed,
		hist_max: filters.maxSpeedCutoff ?? undefined,
		boundary_threshold: filters.boundaryThreshold,
		histogram: true,
		hist_bucket_size: DEFAULT_REPORT_HISTOGRAM_BUCKET_SIZE,
		site_id: base.siteId,
		paper_size: base.paperSize
	};

	if (comparison) {
		request.compare_start_date = comparison.compareStartDate;
		request.compare_end_date = comparison.compareEndDate;
		request.compare_source = comparison.compareSource;
	}

	return request;
}
