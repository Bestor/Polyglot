import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { getMetadata } from '$lib/server/polyglot';

export const load: PageServerLoad = async ({ params }) => {
	const metadata = await getMetadata();
	const datasource = metadata.datasources.find((ds) => ds.name === params.datasource);
	if (!datasource) {
		throw error(404, `no such datasource: ${params.datasource}`);
	}
	const tables = metadata.tables.filter((t) => t.datasource === params.datasource);
	const functions = metadata.functions.filter((f) => f.datasource === params.datasource);
	return { datasource, tables, functions };
};
