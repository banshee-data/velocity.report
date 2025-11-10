import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	server: {
		proxy: {
			'/api': 'http://localhost:8080'
		}
	},
	plugins: [sveltekit()],
	optimizeDeps: {
		exclude: ['svelte-ux', 'layerchart', '@layerstack/tailwind']
	},
	build: {
		emptyOutDir: true,
		rollupOptions: {
			output: {
				// Force everything into a single chunk
				manualChunks: () => 'everything.js'
			}
		}
	}
});
