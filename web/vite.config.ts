import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { realpathSync } from 'node:fs';
import { resolve } from 'node:path';
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
		fs: {
			// In git worktrees, node_modules may be symlinked to the main
			// repo. Resolve the real path so Vite allows serving them.
			allow: [
				realpathSync(resolve('node_modules')),
				// The shared scene player lives outside web/, so the dev
				// server has to be allowed to read it.
				realpathSync(resolve('../public_html/src/js'))
			]
		},
		proxy: {
			'/api/lidar': 'http://localhost:8081',
			'/api': 'http://localhost:8080'
		}
	},
	plugins: [svelteVirtualCssFix(), tailwindcss(), sveltekit()],
	resolve: {
		alias: {
			// The public scenes' three.js player, imported rather than copied,
			// so the operator tools and the public site render through the
			// same modules. Types come from src/lib/scene/scene-reader.d.ts;
			// see svelte.config.js for why the alias is not a kit alias.
			$scene: resolve('../public_html/src/js')
		}
	},
	optimizeDeps: {
		exclude: ['svelte-ux', 'layerchart', '@layerstack/tailwind']
	},
	build: {
		emptyOutDir: true,
		chunkSizeWarningLimit: 2000,
		rollupOptions: {
			output: {
				// Force everything into a single chunk
				manualChunks: () => 'everything.js'
			}
		}
	}
});
