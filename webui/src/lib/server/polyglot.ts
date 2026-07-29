import { config } from './config';
import type {
	ListQueriesResponse,
	QueryDetail,
	DatasourcesResponse,
	MetadataResponse,
	TableRows,
	Job
} from '$lib/types';

async function polyglotFetch(path: string): Promise<Response> {
	const res = await fetch(`${config.polyglotUrl}${path}`, {
		headers: { Authorization: `Bearer ${config.polyglotAuthToken}` }
	});
	if (!res.ok) {
		throw new Error(`polyglot ${path}: ${res.status} ${await res.text()}`);
	}
	return res;
}

// Thrown by runQuery/runWarm/getJobStatus instead of polyglotFetch's plain
// Error - callers (the api/warm and api/query/[datasource] routes) need
// the real status code to forward the caller's own request error (bad
// SQL, missing arg, ...) as the same 4xx it actually was, not a generic
// failure.
export class PolyglotError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

// polyglot's own error responses are PocketBase's ApiError JSON shape
// ({"status":...,"message":"...","data":{...}}) - pull out just the
// human-readable message so a re-thrown PolyglotError doesn't wrap JSON
// inside JSON by the time it reaches the browser.
function extractErrorMessage(text: string): string {
	try {
		const parsed = JSON.parse(text);
		if (parsed && typeof parsed.message === 'string') return parsed.message;
	} catch {
		// not JSON - the raw body is already the best available message
	}
	return text;
}

export const listQueries = (): Promise<ListQueriesResponse> =>
	polyglotFetch('/queries').then((r) => r.json());

export const getQueryDetail = (id: string): Promise<QueryDetail> =>
	polyglotFetch(`/queries/detail?id=${encodeURIComponent(id)}`).then((r) => r.json());

export const listDatasources = (): Promise<DatasourcesResponse> =>
	polyglotFetch('/datasources').then((r) => r.json());

export const getMetadata = (): Promise<MetadataResponse> =>
	polyglotFetch('/metadata').then((r) => r.json());

export interface PreviewOptions {
	limit: number;
	offset: number;
	sort?: string;
	dir?: 'asc' | 'desc';
}

// previewTable interpolates `datasource`/`table`/`sort` directly into SQL
// text (table/column identifiers can't go through GET /query's normal
// value-only parameter, since there's no SQL placeholder for an identifier)
// - it does NOT validate them itself. The caller (routes/api/explorer/
// [datasource]/[table]/+server.ts) is responsible for confirming all three
// are real, known values from a fresh getMetadata() call before calling
// this - see that route for the actual trust boundary.
export async function previewTable(
	datasource: string,
	table: string,
	{ limit, offset, sort, dir }: PreviewOptions
): Promise<TableRows> {
	let sql = `SELECT * FROM "${table}"`;
	if (sort) sql += ` ORDER BY "${sort}" ${dir === 'desc' ? 'DESC' : 'ASC'}`;
	// Fetch one row beyond the requested page size so Prev/Next can know
	// whether another page exists, without a separate COUNT query. This is
	// NOT the same thing as the response's own `truncated` field (that
	// reflects polyglot's server-side safety cap - internal/ai's
	// maxQueryRows/maxQueryResponseBytes - cutting off results; since our
	// own LIMIT is always far below that cap, `truncated` would come back
	// false on almost every page regardless of whether more rows exist,
	// confirmed live against real data during verification). We compute
	// our own truncated below instead of trusting the response's.
	sql += ` LIMIT ${limit + 1} OFFSET ${offset}`;

	const params = new URLSearchParams({ sql, datasource });
	const result: TableRows = await polyglotFetch(`/query?${params}`).then((r) => r.json());

	const truncated = result.rows.length > limit;
	const rows = truncated ? result.rows.slice(0, limit) : result.rows;
	return { rows, row_count: rows.length, truncated };
}

// runQuery sends a user's own arbitrary SQL text verbatim, unlike
// previewTable's identifier-only interpolation - safe to expose as-is
// since polyglot's own GET /query already enforces read-only execution
// (ai.RunReadOnlyQuery) and blocks its own bookkeeping tables
// (reservedTablePattern) server-side. Uses PolyglotError rather than
// polyglotFetch's plain Error so the caller (routes/api/query/
// [datasource]/+server.ts) can forward the real rejection reason (bad SQL,
// non-SELECT, unknown table, ...) with its real status instead of a
// generic failure - surfacing the actual reason is the point of a SQL box.
export async function runQuery(datasource: string, sql: string): Promise<TableRows> {
	const params = new URLSearchParams({ sql, datasource });
	const res = await fetch(`${config.polyglotUrl}/query?${params}`, {
		headers: { Authorization: `Bearer ${config.polyglotAuthToken}` }
	});
	const text = await res.text();
	if (!res.ok) throw new PolyglotError(res.status, extractErrorMessage(text));
	return JSON.parse(text);
}

// runWarm/getJobStatus proxy POST /warm and GET /jobs?id=&datasource=
// (see routes/api/warm/+server.ts) - the actual invoke-a-cache-warm-
// function and poll-its-status calls.
export async function runWarm(
	datasource: string,
	fn: string,
	args: Record<string, unknown>
): Promise<Job> {
	const res = await fetch(`${config.polyglotUrl}/warm`, {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${config.polyglotAuthToken}`,
			'Content-Type': 'application/json'
		},
		body: JSON.stringify({ datasource, function: fn, args })
	});
	const text = await res.text();
	if (!res.ok) throw new PolyglotError(res.status, extractErrorMessage(text));
	return JSON.parse(text);
}

export async function getJobStatus(datasource: string, id: string): Promise<Job> {
	const params = new URLSearchParams({ id, datasource });
	const res = await fetch(`${config.polyglotUrl}/jobs?${params}`, {
		headers: { Authorization: `Bearer ${config.polyglotAuthToken}` }
	});
	const text = await res.text();
	if (!res.ok) throw new PolyglotError(res.status, extractErrorMessage(text));
	return JSON.parse(text);
}
