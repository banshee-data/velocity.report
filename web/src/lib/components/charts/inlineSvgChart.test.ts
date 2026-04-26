import { buildInlineSvgChartRequestUrl, isInlineSvgContentType } from './inlineSvgChart';

describe('inlineSvgChart helpers', () => {
	it('accepts same-origin relative chart URLs and appends a cache-busting timestamp', () => {
		const url = buildInlineSvgChartRequestUrl(
			'/api/charts/report.svg?group=4h',
			'http://localhost:5173',
			12345
		);

		expect(url.origin).toBe('http://localhost:5173');
		expect(url.pathname).toBe('/api/charts/report.svg');
		expect(url.searchParams.get('group')).toBe('4h');
		expect(url.searchParams.get('_ts')).toBe('12345');
	});

	it('rejects cross-origin chart URLs', () => {
		expect(() =>
			buildInlineSvgChartRequestUrl(
				'https://evil.example/chart.svg',
				'http://localhost:5173',
				12345
			)
		).toThrow('Could not load chart preview.');
	});

	it('accepts SVG content types with parameters', () => {
		expect(isInlineSvgContentType('image/svg+xml')).toBe(true);
		expect(isInlineSvgContentType('image/svg+xml; charset=utf-8')).toBe(true);
	});

	it('rejects missing or non-SVG content types', () => {
		expect(isInlineSvgContentType(null)).toBe(false);
		expect(isInlineSvgContentType('text/html')).toBe(false);
	});
});
