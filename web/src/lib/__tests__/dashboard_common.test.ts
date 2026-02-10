/* eslint-disable @typescript-eslint/no-require-imports -- CommonJS module requires require() */
const { escapeHTML, parseDuration, formatDuration } =
	require('@monitor/assets/dashboard_common.js') as {
		escapeHTML: (str: unknown) => string;
		parseDuration: (s: string | null | undefined) => number;
		formatDuration: (secs: number) => string;
	};
/* eslint-enable @typescript-eslint/no-require-imports */

describe('escapeHTML', () => {
	it('returns empty string for null and undefined', () => {
		expect(escapeHTML(null)).toBe('');
		expect(escapeHTML(undefined)).toBe('');
	});

	it('converts numbers to strings unchanged', () => {
		expect(escapeHTML(42)).toBe('42');
		expect(escapeHTML(3.14)).toBe('3.14');
		expect(escapeHTML(0)).toBe('0');
	});

	it('passes plain text through unchanged', () => {
		expect(escapeHTML('hello world')).toBe('hello world');
	});

	it('escapes ampersands', () => {
		expect(escapeHTML('a & b')).toBe('a &amp; b');
	});

	it('escapes less-than signs', () => {
		expect(escapeHTML('a < b')).toBe('a &lt; b');
	});

	it('escapes greater-than signs', () => {
		expect(escapeHTML('a > b')).toBe('a &gt; b');
	});

	it('escapes double quotes', () => {
		expect(escapeHTML('a "b" c')).toBe('a &quot;b&quot; c');
	});

	it('escapes single quotes', () => {
		expect(escapeHTML("a 'b' c")).toBe('a &#39;b&#39; c');
	});

	it('escapes all special characters together', () => {
		expect(escapeHTML('<script>alert("xss")&</script>')).toBe(
			'&lt;script&gt;alert(&quot;xss&quot;)&amp;&lt;/script&gt;'
		);
	});

	it('handles strings with only special characters', () => {
		expect(escapeHTML('&<>"\'&')).toBe('&amp;&lt;&gt;&quot;&#39;&amp;');
	});

	it('handles empty string', () => {
		expect(escapeHTML('')).toBe('');
	});

	it('escapes ampersand before other entities (no double-escape on first pass)', () => {
		// &lt; should become &amp;lt; — the ampersand is escaped
		expect(escapeHTML('&lt;')).toBe('&amp;lt;');
	});
});

describe('parseDuration', () => {
	it('returns 0 for empty/null input', () => {
		expect(parseDuration('')).toBe(0);
		expect(parseDuration(null)).toBe(0);
		expect(parseDuration(undefined)).toBe(0);
	});

	it('parses seconds', () => {
		expect(parseDuration('5s')).toBe(5);
		expect(parseDuration('30s')).toBe(30);
		expect(parseDuration('0s')).toBe(0);
	});

	it('parses milliseconds', () => {
		expect(parseDuration('500ms')).toBe(0.5);
		expect(parseDuration('1000ms')).toBe(1);
		expect(parseDuration('100ms')).toBe(0.1);
	});

	it('parses minutes', () => {
		expect(parseDuration('2m')).toBe(120);
		expect(parseDuration('1m')).toBe(60);
	});

	it('parses hours', () => {
		expect(parseDuration('1h')).toBe(3600);
		expect(parseDuration('2h')).toBe(7200);
	});

	it('parses compound durations', () => {
		expect(parseDuration('1m30s')).toBe(90);
		expect(parseDuration('1h30m')).toBe(5400);
		expect(parseDuration('2h15m30s')).toBe(8130);
	});

	it('handles fractional values', () => {
		expect(parseDuration('1.5s')).toBe(1.5);
		expect(parseDuration('2.5m')).toBe(150);
	});
});

describe('formatDuration', () => {
	it('formats seconds (< 60)', () => {
		expect(formatDuration(5)).toBe('5s');
		expect(formatDuration(30)).toBe('30s');
		expect(formatDuration(0)).toBe('0s');
	});

	it('formats minutes (< 3600)', () => {
		expect(formatDuration(60)).toBe('1m');
		expect(formatDuration(90)).toBe('1m 30s');
		expect(formatDuration(120)).toBe('2m');
	});

	it('formats hours (>= 3600)', () => {
		expect(formatDuration(3600)).toBe('1h');
		expect(formatDuration(5400)).toBe('1h 30m');
		expect(formatDuration(7200)).toBe('2h');
	});

	it('rounds fractional seconds', () => {
		expect(formatDuration(5.4)).toBe('5s');
		expect(formatDuration(5.6)).toBe('6s');
	});
});
