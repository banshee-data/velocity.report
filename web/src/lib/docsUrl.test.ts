import { gitRepoDocsUrl, offlineHomepageUrl } from './docsUrl';

describe('gitRepoDocsUrl', () => {
	it('uses the current origin with the repository docs path', () => {
		expect(gitRepoDocsUrl({ href: 'http://pi.local:8080/app/' })).toBe(
			'http://pi.local:8080/docs/'
		);
	});

	it('clears path, search, and hash', () => {
		expect(gitRepoDocsUrl({ href: 'http://127.0.0.1:8080/app/reports?x=1#top' })).toBe(
			'http://127.0.0.1:8080/docs/'
		);
	});

	it('preserves https for proxied local deployments', () => {
		expect(gitRepoDocsUrl({ href: 'https://velocity.local/app/' })).toBe(
			'https://velocity.local/docs/'
		);
	});
});

describe('offlineHomepageUrl', () => {
	it('uses the current origin with the offline homepage path', () => {
		expect(offlineHomepageUrl({ href: 'https://velocity.local/app/reports' })).toBe(
			'https://velocity.local/homepage/'
		);
	});
});
