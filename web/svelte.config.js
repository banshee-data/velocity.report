import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),

	kit: {
		adapter: adapter(),
		paths: {
			base: '/app',
			relative: false
		},
		prerender: {
			handleMissingId: 'warn',
			handleUnseenRoutes: 'warn'
		}
	}
};

export default config;
