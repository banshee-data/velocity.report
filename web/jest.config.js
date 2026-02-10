import { createRequire } from 'module';
import { dirname, resolve } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(__dirname, '..');
const localRequire = createRequire(import.meta.url);

// Resolve full entry point paths for pnpm compatibility when rootDir != web/
const tsJestEntry = localRequire.resolve('ts-jest');
const svelteJesterEntry = localRequire.resolve('svelte-jester');

/** @type {import('jest').Config} */
export default {
	rootDir: repoRoot,
	testEnvironment: 'jsdom',
	extensionsToTreatAsEsm: ['.ts', '.svelte'],
	roots: ['<rootDir>/web/src', '<rootDir>/internal/lidar/monitor/assets'],
	coverageProvider: 'v8',
	moduleNameMapper: {
		'^\\$lib(.*)$': '<rootDir>/web/src/lib$1',
		'^\\$app(.*)$': '<rootDir>/web/src/mocks/$app$1',
		'^svelte/store$': '<rootDir>/web/src/__mocks__/svelte/store.ts',
		'^@testing-library/svelte$': '<rootDir>/web/src/__mocks__/@testing-library/svelte.ts',
		'^(.+)\\.svelte$': '<rootDir>/web/src/__mocks__/svelte-component.ts',
		'^@monitor/assets/(.*)$': '<rootDir>/internal/lidar/monitor/assets/$1'
	},
	transform: {
		'^.+\\.ts$': [
			tsJestEntry,
			{
				tsconfig: {
					target: 'es2022',
					module: 'esnext',
					moduleResolution: 'bundler',
					resolveJsonModule: true,
					allowJs: true,
					checkJs: true,
					esModuleInterop: true,
					isolatedModules: true,
					skipLibCheck: true,
					strict: true
				},
				useESM: true
			}
		],
		'^.+\\.svelte$': [
			svelteJesterEntry,
			{
				preprocess: true,
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
		'internal/lidar/monitor/assets/*.js',
		'!internal/lidar/monitor/assets/echarts.min.js'
	],
	coverageThreshold: {
		[resolve(repoRoot, 'web/src/lib/')]: {
			branches: 90,
			functions: 90,
			lines: 90,
			statements: 90
		},
		[resolve(repoRoot, 'internal/lidar/monitor/assets/')]: {
			branches: 90,
			functions: 90,
			lines: 90,
			statements: 90
		}
	},
	setupFilesAfterEnv: ['<rootDir>/web/src/setupTests.ts'],
	testMatch: ['**/__tests__/**/*.[jt]s', '**/?(*.)+(spec|test).[jt]s']
};
