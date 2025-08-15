import layerstack from '@layerstack/tailwind/plugin';

/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}',
		'./node_modules/svelte-ux/**/*.{svelte,js}'
	],
	theme: {
		extend: {}
	},

	plugins: [layerstack]
};
