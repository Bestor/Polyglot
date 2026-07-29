import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { getMetadata, runWarm, getJobStatus, PolyglotError } from '$lib/server/polyglot';

// POST invokes a cache-warm function; GET polls a previously started job's
// status (?id=&datasource=). Grouped in one route since both proxy the
// same underlying feature and exist for the same reason: the browser never
// gets polyglot's own bearer token (POLYGLOT_AUTH_TOKEN lives only in
// $lib/server/config), so every polyglot call has to go through a
// server-side route like this one.

export const POST: RequestHandler = async ({ request }) => {
	const body = await request.json().catch(() => null);
	const datasource = typeof body?.datasource === 'string' ? body.datasource : '';
	const fn = typeof body?.function === 'string' ? body.function : '';
	const args = body?.args && typeof body.args === 'object' ? body.args : {};

	// Re-validate against a fresh GET /metadata before proxying, mirroring
	// routes/api/explorer/[datasource]/[table]/+server.ts's own trust
	// boundary - polyglot's own Registry.RunFunction already rejects an
	// unknown datasource/function too, but failing fast here with a clear
	// message is cheap and consistent with that established pattern.
	const metadata = await getMetadata();
	const known = metadata.functions.some((f) => f.datasource === datasource && f.name === fn);
	if (!known) {
		throw error(400, `unknown function "${fn}" on datasource "${datasource}"`);
	}

	try {
		const job = await runWarm(datasource, fn, args);
		return json(job, { status: 202 });
	} catch (err) {
		if (err instanceof PolyglotError) throw error(err.status, err.message);
		throw error(502, 'failed to reach polyglot');
	}
};

export const GET: RequestHandler = async ({ url }) => {
	const id = url.searchParams.get('id') ?? '';
	const datasource = url.searchParams.get('datasource') ?? '';
	if (!id || !datasource) {
		throw error(400, 'id and datasource are required');
	}

	try {
		return json(await getJobStatus(datasource, id));
	} catch (err) {
		if (err instanceof PolyglotError) throw error(err.status, err.message);
		throw error(502, 'failed to reach polyglot');
	}
};
