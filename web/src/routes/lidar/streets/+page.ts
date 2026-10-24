import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';

/**
 * The street editor became the read-only site viewer; editing moved to the
 * macOS app. Old links, with their `?site=` and anything else, land on it.
 */
export function load({ url }: { url: URL }) {
	redirect(308, `${resolve('/lidar/sites')}${url.search}`);
}
