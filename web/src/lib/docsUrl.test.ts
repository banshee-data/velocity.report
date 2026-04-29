import { offlineDocsUrl } from './docsUrl';

describe('offlineDocsUrl', () => {
	it('uses the current hostname with the offline docs port', () => {
		expect(offlineDocsUrl({ href: 'http://pi.local:8080/app/' })).toBe('http://pi.local:8083/');
	});

	it('clears path, search, and hash', () => {
		expect(offlineDocsUrl({ href: 'http://127.0.0.1:8080/app/reports?x=1#top' })).toBe(
			'http://127.0.0.1:8083/'
		);
	});

	it('preserves https for proxied local deployments', () => {
		expect(offlineDocsUrl({ href: 'https://velocity.local/app/' })).toBe(
			'https://velocity.local:8083/'
		);
	});
});
