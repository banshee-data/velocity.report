import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';

/**
 * The Segments page is the Windows page (T1 of the LiDAR workflow design).
 * Old links, with their query, land on it.
 */
export function load({ url }: { url: URL }) {
	redirect(308, `${resolve('/lidar/windows')}${url.search}`);
}
