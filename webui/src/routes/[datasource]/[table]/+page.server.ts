import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { getMetadata, previewTable } from '$lib/server/polyglot';

const PAGE_SIZE = 50;

export const load: PageServerLoad = async ({ params }) => {
	const metadata = await getMetadata();
	const datasource = metadata.datasources.find((ds) => ds.name === params.datasource);
	if (!datasource) {
		throw error(404, `no such datasource: ${params.datasource}`);
	}
	const table = metadata.tables.find(
		(t) => t.datasource === params.datasource && t.name === params.table
	);
	if (!table) {
		throw error(404, `no such table: ${params.table}`);
	}

	const initial = await previewTable(datasource.name, table.name, { limit: PAGE_SIZE, offset: 0 });
	return { datasource, table, initial };
};
