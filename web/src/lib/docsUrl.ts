export function offlineDocsUrl(locationLike: Pick<Location, 'href'>): string {
	const url = new URL(locationLike.href);
	url.port = '8083';
	url.pathname = '/';
	url.search = '';
	url.hash = '';
	return url.toString();
}
