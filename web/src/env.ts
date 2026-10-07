import { defineEnvVars } from '@sveltejs/kit/env';

// Build provenance for the meta tags in app.html. SvelteKit 3 fills
// %sveltekit.env.NAME% only for variables declared here; an undeclared one
// renders as an empty string. scripts/set-build-env.js sets all three for
// every `pnpm run build` and `pnpm run dev`, so a build that bypasses it
// fails here instead of shipping a page that cannot say what made it.
export const variables = defineEnvVars({
	PUBLIC_GIT_SHA: {
		public: true,
		static: true,
		description: 'Short git SHA of the commit the web app was built from'
	},
	PUBLIC_BUILD_TIME: {
		public: true,
		static: true,
		description: 'ISO build timestamp, or DEV_MODE under the dev server'
	},
	PUBLIC_BUILD_VERSION: {
		public: true,
		static: true,
		description: 'Release version from the Makefile VERSION, or "unknown"'
	}
});
