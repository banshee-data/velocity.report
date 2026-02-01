/** @type {import('jest').Config} */
export default {
	preset: 'ts-jest',
	testEnvironment: 'jsdom',
	extensionsToTreatAsEsm: ['.ts', '.svelte'],
	moduleNameMapper: {
		'^\\$lib(.*)$': '<rootDir>/src/lib$1',
		'^\\$app(.*)$': '<rootDir>/src/mocks/$app$1',
		'^svelte/store$': '<rootDir>/src/__mocks__/svelte/store.ts',
		'^@testing-library/svelte$': '<rootDir>/src/__mocks__/@testing-library/svelte.ts',
		'^(.+)\\.svelte$': '<rootDir>/src/__mocks__/svelte-component.ts'
	},
	transform: {
		'^.+\\.ts$': [
			'ts-jest',
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
			'svelte-jester',
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
		'src/lib/**/*.{ts,js}',
		'!src/lib/**/*.d.ts',
		'!src/lib/index.ts',
		'!src/lib/icons.ts',
		'!src/lib/assets/**'
	],
	coverageThreshold: {
		global: {
			branches: 90,
			functions: 90,
			lines: 90,
			statements: 90
		}
	},
	setupFilesAfterEnv: ['<rootDir>/src/setupTests.ts'],
	testMatch: ['**/__tests__/**/*.[jt]s', '**/?(*.)+(spec|test).[jt]s']
};
