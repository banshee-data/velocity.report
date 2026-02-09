import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig, type Plugin } from 'vite';

/**
 * Prevents @tailwindcss/vite from choking on virtual CSS modules that
 * vite-plugin-svelte fails to extract from node_modules components.
 * When extraction fails the raw Svelte source is mistakenly passed as CSS.
 */
function svelteVirtualCssFix(): Plugin {
	return {
		name: 'svelte-virtual-css-fix',
		enforce: 'pre',
		resolveId(id) {
			if (id.includes('node_modules') && id.includes('.svelte?svelte&type=style&lang.css')) {
				return id;
			}
		},
		load(id) {
			if (id.includes('node_modules') && id.includes('.svelte?svelte&type=style&lang.css')) {
				return '';
			}
		}
	};
}

export default defineConfig({
	server: {
		proxy: {
			'/api/lidar': 'http://localhost:8081',
			'/api': 'http://localhost:8080'
		}
	},
	plugins: [svelteVirtualCssFix(), tailwindcss(), sveltekit()],
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
