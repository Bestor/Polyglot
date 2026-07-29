import type { PageServerLoad } from './$types';
import { listQueries } from '$lib/server/polyglot';

export const load: PageServerLoad = async () => ({ queries: (await listQueries()).queries });
