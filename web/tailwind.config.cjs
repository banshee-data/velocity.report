/** @type {import('tailwindcss').Config}*/
const config = {
	content: [
		'./src/**/*.{html,svelte,js}',
		'./node_modules/svelte-ux/**/*.{svelte,js}',
		'./node_modules/layerchart/**/*.{svelte,js}'
	],
	theme: {},
	plugins: []
};

module.exports = config;
