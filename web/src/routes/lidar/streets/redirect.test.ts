// Kit's redirect throws; this one throws what it was given, so the test can read it.
jest.mock(
	'@sveltejs/kit',
	() => ({
		redirect: (status: number, location: string) => {
			throw { status, location };
		}
	}),
	{ virtual: true }
);

import { load } from './+page';

function redirectOf(href: string): unknown {
	try {
		load({ url: new URL(href) });
	} catch (e) {
		return e;
	}
	return null;
}

describe('the old street editor route', () => {
	it('redirects to the site viewer, permanently', () => {
		expect(redirectOf('http://localhost:8080/app/lidar/streets')).toEqual({
			status: 308,
			location: '/app/lidar/sites'
		});
	});

	it('keeps the site and anything else asked for', () => {
		expect(redirectOf('http://localhost:8080/app/lidar/streets?site=808580f3f&view=3d')).toEqual({
			status: 308,
			location: '/app/lidar/sites?site=808580f3f&view=3d'
		});
	});
});
