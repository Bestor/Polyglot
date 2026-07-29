import type { PageServerLoad } from './$types';
import { getMetadata } from '$lib/server/polyglot';

export const load: PageServerLoad = async () => {
	const metadata = await getMetadata();
	const datasources = metadata.datasources.map((ds) => ({
		...ds,
		tableCount: metadata.tables.filter((t) => t.datasource === ds.name).length
	}));
	return { datasources };
};
