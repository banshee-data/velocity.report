// Disable SSR — this page uses browser APIs (fetch to local backend)
// that are not available during server-side rendering.
// prerender is inherited from the root layout (+layout.js) so the static
// adapter still emits an HTML shell that bootstraps the client app.
export const ssr = false;
