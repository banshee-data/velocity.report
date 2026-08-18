function localSiteUrl(locationLike: Pick<Location, 'href'>, pathname: string): string {
	const url = new URL(locationLike.href);
	url.pathname = pathname;
	url.search = '';
	url.hash = '';
	return url.toString();
}

export function offlinePublicHTMLUrl(locationLike: Pick<Location, 'href'>): string {
	return localSiteUrl(locationLike, '/public_html/');
}

export function gitRepoDocsUrl(locationLike: Pick<Location, 'href'>): string {
	return localSiteUrl(locationLike, '/docs/');
}
