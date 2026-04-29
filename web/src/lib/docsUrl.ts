export function offlineDocsUrl(locationLike: Pick<Location, 'href'>): string {
	const url = new URL(locationLike.href);
	url.pathname = '/docs/';
	url.search = '';
	url.hash = '';
	return url.toString();
}
