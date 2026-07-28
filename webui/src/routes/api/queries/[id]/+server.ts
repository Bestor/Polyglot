import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { getQueryDetail } from '$lib/server/polyglot';

export const GET: RequestHandler = async ({ params }) => {
	try {
		return json(await getQueryDetail(params.id));
	} catch {
		throw error(502, 'failed to load query detail from polyglot');
	}
};
