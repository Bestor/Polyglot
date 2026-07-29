import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { getMetadata, runQuery, PolyglotError } from '$lib/server/polyglot';

// Backs the Data Explorer's ad hoc SQL query box. Unlike routes/api/
// explorer/[datasource]/[table]/+server.ts, there's no identifier
// interpolation here to police - the user's sql text is sent to polyglot's
// GET /query verbatim, and polyglot itself already enforces read-only
// execution (ai.RunReadOnlyQuery) and blocks its own bookkeeping tables
// (query.go's reservedTablePattern). Only `datasource` needs checking, so
// a typo'd or made-up name fails clearly here rather than as a bare 400
// from polyglot.
const MAX_SQL_LENGTH = 10_000;

export const POST: RequestHandler = async ({ params, request }) => {
	const metadata = await getMetadata();
	const datasource = metadata.datasources.find((ds) => ds.name === params.datasource);
	if (!datasource) {
		throw error(400, `unknown datasource: ${params.datasource}`);
	}

	const body = await request.json().catch(() => null);
	const sql = typeof body?.sql === 'string' ? body.sql.trim() : '';
	if (!sql) {
		throw error(400, 'sql is required');
	}
	if (sql.length > MAX_SQL_LENGTH) {
		throw error(400, 'sql is too long');
	}

	try {
		return json(await runQuery(datasource.name, sql));
	} catch (err) {
		if (err instanceof PolyglotError) throw error(err.status, err.message);
		throw error(502, 'failed to reach polyglot');
	}
};
