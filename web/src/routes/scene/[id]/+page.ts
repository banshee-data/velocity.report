import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => {
	redirect(308, `/survey/${encodeURIComponent(params.id)}`);
};

export const prerender = false;
