export type InlineSvgChartTheme = 'light' | 'dark';
export type InlineSvgChartThemeMode = 'source' | 'dashboard';

export interface InlineSvgChartDarkColours {
	background: string;
	text: string;
}

const defaultDarkColours: InlineSvgChartDarkColours = {
	background: '#000000',
	text: '#ffffff'
};

const inlineSvgDarkPalette = new Map<string, string>([
	['#fbd92f', '#ffe36b'],
	['#f7b32b', '#ffc85c'],
	['#f25f5c', '#ff8f8c'],
	['#2d1e2f', '#e5e7eb']
]);

const inlineSvgWhiteValues = new Set(['white', '#fff', '#ffffff']);
const inlineSvgBlackValues = new Set(['black', '#000', '#000000']);
const inlineSvgMidGreyValues = new Set(['#999', '#999999']);
const inlineSvgLightBorderValues = new Set(['#ccc', '#cccccc']);

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

export function buildInlineSvgChartBlobUrl(svg: string): string {
	return URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml;charset=utf-8' }));
}

export function resolveInlineSvgChartTheme(backgroundColor: string): InlineSvgChartTheme {
	const rgb = parseInlineSvgChartColor(backgroundColor);
	if (!rgb) {
		return 'light';
	}

	const luminance = rgb
		.map((channel) => channel / 255)
		.map((channel) =>
			channel <= 0.03928 ? channel / 12.92 : Math.pow((channel + 0.055) / 1.055, 2.4)
		)
		.reduce((sum, channel, index) => sum + channel * [0.2126, 0.7152, 0.0722][index], 0);

	return luminance < 0.45 ? 'dark' : 'light';
}

export function resolveInlineSvgChartDarkColours(
	host: HTMLElement | null
): InlineSvgChartDarkColours {
	if (typeof window === 'undefined' || !host) {
		return defaultDarkColours;
	}

	const probe = document.createElement('span');
	probe.setAttribute('aria-hidden', 'true');
	probe.style.position = 'absolute';
	probe.style.pointerEvents = 'none';
	probe.style.opacity = '0';
	probe.style.inlineSize = '0';
	probe.style.blockSize = '0';
	probe.style.overflow = 'hidden';
	probe.style.backgroundColor = 'var(--color-surface-100)';
	probe.style.color = 'var(--color-surface-content)';
	host.appendChild(probe);
	const computed = getComputedStyle(probe);
	const background = computed.backgroundColor || defaultDarkColours.background;
	const text = computed.color || defaultDarkColours.text;
	probe.remove();

	return { background, text };
}

export function transformInlineSvgChartSvg(
	sourceSvg: string,
	theme: InlineSvgChartTheme,
	colours: InlineSvgChartDarkColours = defaultDarkColours
): string {
	if (theme === 'light') {
		return sourceSvg;
	}

	const parser = new DOMParser();
	const documentSvg = parser.parseFromString(sourceSvg, 'image/svg+xml');
	const root = documentSvg.documentElement;
	if (!root || root.tagName.toLowerCase() !== 'svg') {
		return sourceSvg;
	}

	const parserError = documentSvg.querySelector('parsererror');
	if (parserError) {
		return sourceSvg;
	}

	const namespace = root.namespaceURI ?? 'http://www.w3.org/2000/svg';
	const background = documentSvg.createElementNS(namespace, 'rect');
	background.setAttribute('x', '0');
	background.setAttribute('y', '0');
	background.setAttribute('width', '100%');
	background.setAttribute('height', '100%');
	background.setAttribute('fill', colours.background);
	background.setAttribute('data-inline-svg-chart-background', 'true');
	root.insertBefore(background, root.firstChild);

	for (const element of Array.from(root.querySelectorAll('*'))) {
		if (element.getAttribute('data-inline-svg-chart-background') === 'true') {
			continue;
		}

		for (const attribute of ['fill', 'stroke'] as const) {
			const current = element.getAttribute(attribute);
			if (!current) {
				continue;
			}

			const replacement = resolveInlineSvgChartDarkColour(current, attribute, colours);
			if (replacement) {
				element.setAttribute(attribute, replacement);
			}
		}

		if (element.tagName.toLowerCase() === 'text') {
			element.setAttribute('fill', colours.text);
		}

		if (element.matches('.max-reference line')) {
			element.setAttribute('stroke', colours.text);
			element.setAttribute('opacity', '0.8');
		}

		if (element.matches('.gap-dividers line')) {
			element.setAttribute('stroke', colours.text);
			element.setAttribute('opacity', '0.35');
		}

		if (element.matches('.x-axis line, .y-axis line, .count-axis line')) {
			element.setAttribute('stroke', colours.text);
		}

		if (
			element.tagName.toLowerCase() === 'rect' &&
			isInlineSvgLightBorder(element.getAttribute('stroke'))
		) {
			element.setAttribute('stroke', colours.text);
			element.setAttribute('stroke-opacity', '0.35');
		}
	}

	return new XMLSerializer().serializeToString(root);
}

function resolveInlineSvgChartDarkColour(
	colour: string,
	attribute: 'fill' | 'stroke',
	colours: InlineSvgChartDarkColours
): string | null {
	const normalised = colour.trim().toLowerCase();
	if (normalised === 'none') {
		return null;
	}

	if (attribute === 'fill' && inlineSvgWhiteValues.has(normalised)) {
		return colours.background;
	}

	if (inlineSvgWhiteValues.has(normalised)) {
		return colours.text;
	}

	if (inlineSvgBlackValues.has(normalised)) {
		return colours.text;
	}

	if (inlineSvgMidGreyValues.has(normalised)) {
		return colours.text;
	}

	if (inlineSvgLightBorderValues.has(normalised)) {
		return colours.text;
	}

	return inlineSvgDarkPalette.get(normalised) ?? null;
}

function isInlineSvgLightBorder(colour: string | null): boolean {
	return colour !== null && inlineSvgLightBorderValues.has(colour.trim().toLowerCase());
}

function parseInlineSvgChartColor(value: string): [number, number, number] | null {
	const trimmed = value.trim().toLowerCase();
	const rgbMatch = trimmed.match(
		/^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})(?:\s*,\s*[0-9.]+)?\s*\)$/
	);
	if (rgbMatch) {
		return [Number(rgbMatch[1]), Number(rgbMatch[2]), Number(rgbMatch[3])];
	}

	const hexMatch = trimmed.match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/i);
	if (!hexMatch) {
		return null;
	}

	const hex =
		hexMatch[1].length === 3
			? hexMatch[1]
					.split('')
					.map((char) => `${char}${char}`)
					.join('')
			: hexMatch[1];

	return [
		parseInt(hex.slice(0, 2), 16),
		parseInt(hex.slice(2, 4), 16),
		parseInt(hex.slice(4, 6), 16)
	];
}
