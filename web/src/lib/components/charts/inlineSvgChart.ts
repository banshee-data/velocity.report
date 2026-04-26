export function buildInlineSvgChartRequestUrl(
	nextUrl: string,
	origin: string,
	timestamp: number
): URL {
	const requestUrl = new URL(nextUrl, origin);
	if (requestUrl.origin !== origin) {
		throw new Error('Could not load chart preview.');
	}
	requestUrl.searchParams.set('_ts', String(timestamp));
	return requestUrl;
}

export function isInlineSvgContentType(contentType: string | null): boolean {
	if (!contentType) {
		return false;
	}

	return contentType.toLowerCase().includes('image/svg+xml');
}
