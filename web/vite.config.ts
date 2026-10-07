import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
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
	plugins: [
		svelteVirtualCssFix(),
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter(),
			// $scene is deliberately aliased in resolve.alias below rather
			// than here. A kit alias also lands in the generated tsconfig
			// paths, which would make svelte-check follow checkJs into
			// public_html's player — code that has never been type-checked
			// and is not ours to retrofit. The hand-written declaration in
			// src/lib/scene is the typed contract instead; Vite resolves the
			// real modules at build time.
			paths: {
				base: '/app',
				relative: false
			},
			prerender: {
				handleMissingId: 'warn',
				handleUnseenRoutes: 'warn'
			},
			// One JS bundle and one CSS file for the whole app, which is what
			// the old rollupOptions.output.manualChunks asked for (kit 3 sets
			// codeSplitting itself and ignores manualChunks). It keeps the
			// shipped shape of the kit 2 build. Kit's split default would also
			// serve correctly: the server embeds all of web/build, so a chunk
			// whose hash begins with "_" is no longer dropped from the binary.
			output: {
				bundleStrategy: 'single'
			}
		})
	],
	resolve: {
		alias: {
			// The public scenes' three.js player, imported rather than copied,
			// so the operator tools and the public site render through the
			// same modules. Types come from src/lib/scene/scene-reader.d.ts;
			// see the sveltekit() options above for why the alias is not a
			// kit alias.
			$scene: resolve('../public_html/src/js'),
			// The shared player imports the bare specifier "three". Because it
			// lives under public_html, Node resolution would look for it in
			// that directory's node_modules, which the web build has no reason
			// to have installed: CI builds web on its own, and the build fails
			// there while passing locally only because a developer happens to
			// have installed public_html too. Pinning it to web's own declared
			// copy keeps this build self-contained.
			three: resolve('node_modules/three')
		}
	},
	optimizeDeps: {
		exclude: ['svelte-ux', 'layerchart', '@layerstack/tailwind']
	},
	build: {
		chunkSizeWarningLimit: 2000
	}
});
