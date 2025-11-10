export default (ctx) => {
	// Only apply Tailwind PostCSS plugin to our app files, not node_modules
	const isNodeModules = ctx.file && ctx.file.includes('node_modules');

	return {
		plugins: isNodeModules
			? {}
			: {
					'@tailwindcss/postcss': {}
				}
	};
};
