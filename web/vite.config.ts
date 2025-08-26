import { defineConfig } from 'vite';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	server: {
		proxy: {
			'/api': 'http://localhost:8080'
		}
	},
	plugins: [sveltekit(), tailwindcss()],
	optimizeDeps: {
		exclude: ['svelte-ux', 'layerchart', '@layerstack/tailwind']
	}
});
