import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		// Using Static adapter for static site generation 
		// https://svelte.dev/docs/kit/adapter-static
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: undefined, // @TODO: set 'index.html' for SPA fallback
			precompress: false,
			strict: true
		})
	}
};

export default config;
