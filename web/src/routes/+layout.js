export const prerender = true;

// Disable SSR for all pages — every page fetches from the local API
// server in onMount, so server-side rendering cannot produce meaningful
// content and may trigger lifecycle errors (onMount, onDestroy, getContext).
export const ssr = false;
