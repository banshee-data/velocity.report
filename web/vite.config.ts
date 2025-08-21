import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	server: {
		proxy: {
			'/api': 'http://localhost:8080'
		}
	},
	plugins: [tailwindcss(), sveltekit()],
	optimizeDeps: {
		exclude: [
			'svelte-ux',
			'layerchart',
			'@layerstack/tailwind'
		]
	}
});
