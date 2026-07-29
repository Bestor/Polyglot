import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { getMetadata, previewTable } from '$lib/server/polyglot';

const DEFAULT_LIMIT = 50;
const MAX_LIMIT = 200;

// previewTable() interpolates table/column names directly into SQL text
// (there's no placeholder for a SQL identifier the way there is for a
// value) - this route is the actual trust boundary: datasource/table/sort
// are all checked against a fresh GET /metadata response before ever
// reaching previewTable(), so nothing arbitrary from the URL gets built
// into a query. limit/offset are parsed as integers and clamped for the
// same reason, even though they're not identifiers - an unbounded LIMIT
// isn't a SQL-injection risk in the identifier sense, but it's still a
// caller-controlled number heading straight into SQL text, not worth
// trusting blindly.
export const GET: RequestHandler = async ({ params, url }) => {
	const metadata = await getMetadata();
	const datasource = metadata.datasources.find((ds) => ds.name === params.datasource);
	if (!datasource) {
		throw error(400, `unknown datasource: ${params.datasource}`);
	}
	const table = metadata.tables.find(
		(t) => t.datasource === params.datasource && t.name === params.table
	);
	if (!table) {
		throw error(400, `unknown table: ${params.table}`);
	}

	const sortParam = url.searchParams.get('sort');
	let sort: string | undefined;
	if (sortParam) {
		if (!table.columns.some((c) => c.name === sortParam)) {
			throw error(400, `unknown column: ${sortParam}`);
		}
		sort = sortParam;
	}

	const dirParam = url.searchParams.get('dir');
	const dir = dirParam === 'desc' ? 'desc' : 'asc';

	const limit = clampInt(url.searchParams.get('limit'), DEFAULT_LIMIT, 1, MAX_LIMIT);
	const offset = clampInt(url.searchParams.get('offset'), 0, 0, Number.MAX_SAFE_INTEGER);

	try {
		return json(await previewTable(datasource.name, table.name, { limit, offset, sort, dir }));
	} catch {
		throw error(502, 'failed to load table data from polyglot');
	}
};

function clampInt(raw: string | null, fallback: number, min: number, max: number): number {
	const n = raw === null ? NaN : Number(raw);
	if (!Number.isInteger(n)) return fallback;
	return Math.min(max, Math.max(min, n));
}
