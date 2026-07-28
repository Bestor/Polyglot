import { config } from './config';
import type { ListQueriesResponse, QueryDetail } from '$lib/types';

async function polyglotFetch(path: string): Promise<Response> {
	const res = await fetch(`${config.polyglotUrl}${path}`, {
		headers: { Authorization: `Bearer ${config.polyglotAuthToken}` }
	});
	if (!res.ok) {
		throw new Error(`polyglot ${path}: ${res.status} ${await res.text()}`);
	}
	return res;
}

export const listQueries = (): Promise<ListQueriesResponse> =>
	polyglotFetch('/queries').then((r) => r.json());

export const getQueryDetail = (id: string): Promise<QueryDetail> =>
	polyglotFetch(`/queries/detail?id=${encodeURIComponent(id)}`).then((r) => r.json());
