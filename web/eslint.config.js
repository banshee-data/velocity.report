import { includeIgnoreFile } from '@eslint/compat';
import js from '@eslint/js';
import prettier from 'eslint-config-prettier';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import { fileURLToPath } from 'node:url';
import ts from 'typescript-eslint';

const gitignorePath = fileURLToPath(new URL('./.gitignore', import.meta.url));
const commonJsDisabledRules = Object.fromEntries(
	[...new Set(svelte.configs.recommended.flatMap((config) => Object.keys(config.rules ?? {})))].map(
		(ruleName) => [ruleName, 'off']
	)
);

export default ts.config(
	includeIgnoreFile(gitignorePath),
	js.configs.recommended,
	...ts.configs.recommended,
	...svelte.configs.recommended,
	prettier,
	...svelte.configs.prettier,
	{
		languageOptions: {
			globals: { ...globals.browser, ...globals.node }
		},
		rules: {
			// typescript-eslint strongly recommend that you do not use the no-undef lint rule on TypeScript projects.
			// see: https://typescript-eslint.io/troubleshooting/faqs/eslint/#i-get-errors-from-the-no-undef-rule-about-global-variables-not-being-defined-even-though-there-are-no-typescript-errors
			'no-undef': 'off'
		}
	},
	{
		files: ['**/*.svelte', '**/*.svelte.ts', '**/*.svelte.js'],
		languageOptions: {
			parserOptions: {
				projectService: true,
				extraFileExtensions: ['.svelte'],
				parser: ts.parser
			}
		},
		rules: {
			// The core rule does not understand Svelte reactive assignments
			// ($: x = …) and flags them as useless. Disable for .svelte files.
			'no-useless-assignment': 'off'
		}
	},
	{
		files: ['**/*.cjs'],
		languageOptions: {
			sourceType: 'commonjs'
		},
		rules: {
			...commonJsDisabledRules,
			'@typescript-eslint/no-require-imports': 'off'
		}
	}
);
