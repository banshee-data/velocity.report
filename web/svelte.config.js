import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),

	kit: {
		adapter: adapter(),
		// $scene is deliberately aliased in vite.config.ts rather than here.
		// A kit alias also lands in the generated tsconfig paths, which would
		// make svelte-check follow checkJs into public_html's player — code
		// that has never been type-checked and is not ours to retrofit. The
		// hand-written declaration in src/lib/scene is the typed contract
		// instead; Vite resolves the real modules at build time.
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
