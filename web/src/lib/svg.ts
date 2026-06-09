// Small SVG helpers shared by the map editor.

/**
 * Encode an SVG document string to base64 for use in a `data:image/svg+xml`
 * URL. Handles multi-byte characters (UTF-8) and large documents by chunking
 * the byte array before `btoa`, which only accepts a binary string.
 */
export function svgToBase64(svgText: string): string {
	const encoder = new TextEncoder();
	const bytes = encoder.encode(svgText);

	const chunkSize = 8192;
	let binaryString = '';
	for (let i = 0; i < bytes.length; i += chunkSize) {
		const chunk = bytes.slice(i, i + chunkSize);
		binaryString += String.fromCharCode(...chunk);
	}
	return btoa(binaryString);
}
