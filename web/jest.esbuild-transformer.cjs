const { createRequire } = require('module');

const viteRequire = createRequire(require.resolve('vite'));
const { transformSync } = viteRequire('esbuild');

function lowerDynamicImports(code) {
	return code.replace(/\bimport\((["'][^"']+["'])\)/g, 'Promise.resolve().then(() => require($1))');
}

function hoistJestMocks(code) {
	const lines = code.split('\n');
	const mocks = [];
	const rest = [];

	for (let i = 0; i < lines.length; i++) {
		const line = lines[i];
		if (!line.trimStart().startsWith('jest.mock(')) {
			rest.push(line);
			continue;
		}

		const block = [line];
		let depth = (line.match(/\(/g) || []).length - (line.match(/\)/g) || []).length;

		while (depth > 0 && i + 1 < lines.length) {
			i++;
			const nextLine = lines[i];
			block.push(nextLine);
			depth += (nextLine.match(/\(/g) || []).length - (nextLine.match(/\)/g) || []).length;
		}

		mocks.push(...block);
	}

	if (mocks.length === 0) {
		return code;
	}

	const firstRequire = rest.findIndex((line) => /\brequire\(/.test(line));
	const insertAt = firstRequire === -1 ? 0 : firstRequire;
	rest.splice(insertAt, 0, ...mocks);

	return rest.join('\n');
}

module.exports = {
	process(source, filename) {
		const result = transformSync(source, {
			loader: 'ts',
			format: 'cjs',
			target: 'es2022',
			sourcemap: 'inline',
			sourcefile: filename
		});

		return {
			code: lowerDynamicImports(hoistJestMocks(result.code)),
			map: result.map || null
		};
	}
};
