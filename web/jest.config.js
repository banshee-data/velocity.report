import { createRequire } from 'module';
import { dirname, resolve } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, '..');
const localRequire = createRequire(import.meta.url);

// Resolve full entry point paths for pnpm compatibility when rootDir != web/
const svelteJesterEntry = localRequire.resolve('svelte-jester');
const esbuildTransformerEntry = resolve(__dirname, 'jest.esbuild-transformer.cjs');

/** @type {import('jest').Config} */
export default {
	rootDir: repoRoot,
	testEnvironment: 'jsdom',
	watchman: false,
	extensionsToTreatAsEsm: ['.ts', '.svelte'],
	roots: ['<rootDir>/web/src', '<rootDir>/internal/lidar/l9endpoints/l10clients/assets'],
	coverageProvider: 'v8',
	moduleNameMapper: {
		'^\\$lib(.*)$': '<rootDir>/web/src/lib$1',
		'^\\$app(.*)$': '<rootDir>/web/src/mocks/$app$1',
		'^svelte/store$': '<rootDir>/web/src/__mocks__/svelte/store.ts',
		'^@testing-library/svelte$': '<rootDir>/web/src/__mocks__/@testing-library/svelte.ts',
		'^(.+)\\.svelte$': '<rootDir>/web/src/__mocks__/svelte-component.ts',
		'^@monitor/assets/(.*)$': '<rootDir>/internal/lidar/l9endpoints/l10clients/assets/$1'
	},
	transform: {
		'^.+\\.ts$': [
			esbuildTransformerEntry,
			{
				target: 'es2022'
			}
		],
		'^.+\\.svelte$': [
			svelteJesterEntry,
			{
				preprocess: false,
				compilerOptions: {
					dev: true
				}
			}
		]
	},
	moduleFileExtensions: ['js', 'ts', 'svelte'],
	transformIgnorePatterns: ['node_modules/(?!(svelte|@testing-library))'],
	collectCoverageFrom: [
		'web/src/lib/**/*.{ts,js}',
		'!web/src/lib/**/*.d.ts',
		'!web/src/lib/index.ts',
		'!web/src/lib/icons.ts',
		'!web/src/lib/assets/**',
		'internal/lidar/l9endpoints/l10clients/assets/*.js',
		'!internal/lidar/l9endpoints/l10clients/assets/echarts.min.js'
	],
	coverageThreshold: {
		[resolve(repoRoot, 'web/src/lib/')]: {
			branches: 90,
			functions: 90,
			lines: 90,
			statements: 90
		},
		[resolve(repoRoot, 'internal/lidar/l9endpoints/l10clients/assets/')]: {
			branches: 90,
			functions: 90,
			lines: 90,
			statements: 90
		}
	},
	setupFilesAfterEnv: ['<rootDir>/web/src/setupTests.ts'],
	testMatch: ['**/__tests__/**/*.[jt]s', '**/?(*.)+(spec|test).[jt]s']
};
