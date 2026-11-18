export interface FollowingAnalysisResult {
	metadata: { binding: { siteID: string }; evidence_sha256: string };
	tracks: number;
	without_time_gap: number;
	histogram: { label: string; count: number }[];
	population: string;
}

/** The backend selects immutable evidence by recording ID, never client paths. */
export async function analyseFollowing(
	runID: string,
	signal: AbortSignal
): Promise<FollowingAnalysisResult> {
	const response = await fetch(
		`/api/lidar/runs/${encodeURIComponent(runID)}/following-review/analysis`,
		{ method: 'POST', signal }
	);
	const body = await response.json();
	if (!response.ok)
		throw new Error(body.error || 'Following evidence is unavailable for this recording.');
	if (
		body.schema !== 'following_analysis_v1' ||
		!Array.isArray(body.histogram) ||
		body.histogram.length !== 5
	)
		throw new Error('Unsupported following analysis response.');
	return body;
}
