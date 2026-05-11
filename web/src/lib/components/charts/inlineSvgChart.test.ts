import {
	buildInlineSvgChartRequestUrl,
	isInlineSvgContentType,
	resolveInlineSvgChartTheme,
	transformInlineSvgChartSvg,
	type InlineSvgChartDarkColours
} from './inlineSvgChart';

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

	it('detects dark and light backgrounds from resolved RGB colours', () => {
		expect(resolveInlineSvgChartTheme('rgb(12, 12, 12)')).toBe('dark');
		expect(resolveInlineSvgChartTheme('rgb(248, 250, 252)')).toBe('light');
	});

	it('rewrites dashboard SVG colours for dark theme without changing the source mode', () => {
		const svg = [
			'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 40">',
			'<g class="x-axis"><line stroke="black" x1="0" y1="10" x2="10" y2="10"/></g>',
			'<g class="gap-dividers"><line stroke="#999" stroke-dasharray="3 3" x1="20" y1="0" x2="20" y2="40"/></g>',
			'<g class="max-reference"><line stroke="#2d1e2f" stroke-dasharray="1 3" x1="0" y1="20" x2="100" y2="20"/></g>',
			'<g class="series-p50"><polyline fill="none" stroke="#fbd92f" points="0,30 100,10"/></g>',
			'<rect fill="white" stroke="#ccc" x="1" y="1" width="98" height="38"/>',
			'<text x="10" y="35">Axis label</text>',
			'</svg>'
		].join('');

		const darkSvg = transformInlineSvgChartSvg(svg, 'dark');
		const documentSvg = new DOMParser().parseFromString(darkSvg, 'image/svg+xml');
		const root = documentSvg.documentElement;

		expect(root.querySelector('.x-axis line')?.getAttribute('stroke')).toBe('#ffffff');
		expect(root.querySelector('.gap-dividers line')?.getAttribute('stroke')).toBe('#ffffff');
		expect(root.querySelector('.max-reference line')?.getAttribute('stroke')).toBe('#ffffff');
		expect(root.querySelector('.series-p50 polyline')?.getAttribute('stroke')).toBe('#ffe36b');
		expect(root.querySelector('text')?.getAttribute('fill')).toBe('#ffffff');
		expect(root.querySelector('rect[stroke="#ffffff"]')?.getAttribute('fill')).toBe('#000000');
		expect(transformInlineSvgChartSvg(svg, 'light')).toBe(svg);
	});

	it('honours injected dark-theme colours so frame colours match surface tokens', () => {
		const svg = [
			'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 40">',
			'<rect fill="white" stroke="#ccc" x="1" y="1" width="98" height="38"/>',
			'<text x="10" y="35">Axis label</text>',
			'</svg>'
		].join('');

		const colours: InlineSvgChartDarkColours = {
			background: 'rgb(39, 39, 42)',
			text: 'rgb(244, 244, 245)'
		};

		const darkSvg = transformInlineSvgChartSvg(svg, 'dark', colours);
		const documentSvg = new DOMParser().parseFromString(darkSvg, 'image/svg+xml');
		const root = documentSvg.documentElement;

		expect(root.querySelector('text')?.getAttribute('fill')).toBe('rgb(244, 244, 245)');
		expect(root.querySelector('rect')?.getAttribute('fill')).toBe('rgb(39, 39, 42)');
		expect(root.querySelector('rect')?.getAttribute('stroke')).toBe('rgb(244, 244, 245)');
	});
});
