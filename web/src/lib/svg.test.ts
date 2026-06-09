import { TextDecoder, TextEncoder } from 'util';

// jsdom does not provide TextEncoder/TextDecoder globally.
global.TextEncoder = TextEncoder as typeof global.TextEncoder;
global.TextDecoder = TextDecoder as typeof global.TextDecoder;

import { svgToBase64 } from './svg';

describe('svgToBase64', () => {
	it('round-trips a simple SVG through base64', () => {
		const svg = '<svg xmlns="http://www.w3.org/2000/svg"></svg>';
		const encoded = svgToBase64(svg);
		expect(typeof encoded).toBe('string');
		expect(Buffer.from(encoded, 'base64').toString('utf-8')).toBe(svg);
	});

	it('handles multi-byte (UTF-8) characters', () => {
		const svg = '<svg><text>© OpenStreetMap — café</text></svg>';
		const decoded = Buffer.from(svgToBase64(svg), 'base64').toString('utf-8');
		expect(decoded).toBe(svg);
	});

	it('handles documents larger than the chunk size', () => {
		const svg = `<svg>${'x'.repeat(20000)}</svg>`;
		const decoded = Buffer.from(svgToBase64(svg), 'base64').toString('utf-8');
		expect(decoded).toBe(svg);
	});
});
