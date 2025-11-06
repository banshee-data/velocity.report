import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),

	kit: {
		adapter: adapter({
			fallback: 'index.html' // Enable SPA mode for dynamic routes
		}),
		paths: {
			base: '/app'
		},
		prerender: {
			handleMissingId: 'warn',
			handleUnseenRoutes: 'warn'
		}
	}
};

export default config;
