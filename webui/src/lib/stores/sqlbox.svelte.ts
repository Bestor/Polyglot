import type { TableRows } from '$lib/types';

// Svelte 5 runes store, module-scoped like chat.svelte.ts - the SQL query
// box floats as a global overlay (mounted once in +layout.svelte, see
// SqlQueryBox.svelte) so the query/results survive navigating between a
// datasource's tables instead of resetting on every page load, the same
// way chat.svelte.ts's history survives navigation. `datasource` is kept
// in sync with the current route's [datasource] param by Sidebar.svelte
// (always mounted, unlike the panel itself, which only exists in the DOM
// while open) - see setSqlboxDatasource.
export const sqlbox = $state<{
	open: boolean;
	datasource: string | null;
	sql: string;
	loading: boolean;
	error: string | null;
	result: TableRows | null;
}>({
	open: false,
	datasource: null,
	sql: '',
	loading: false,
	error: null,
	result: null
});

// Resets the query/results only when the datasource actually changed, not
// on every within-datasource table navigation - browsing
// /valorant/players -> /valorant/matches keeps whatever's in the box,
// since staying usable across that exact kind of navigation is the point.
export function setSqlboxDatasource(name: string | null) {
	if (name === sqlbox.datasource) return;
	sqlbox.datasource = name;
	sqlbox.sql = '';
	sqlbox.result = null;
	sqlbox.error = null;
}

export async function runSqlQuery() {
	const datasource = sqlbox.datasource;
	const q = sqlbox.sql.trim();
	if (!datasource || !q || sqlbox.loading) return;
	sqlbox.loading = true;
	sqlbox.error = null;
	try {
		const res = await fetch(`/api/query/${encodeURIComponent(datasource)}`, {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ sql: q })
		});
		if (!res.ok) throw new Error(await res.text());
		sqlbox.result = await res.json();
	} catch (err) {
		sqlbox.error = err instanceof Error ? err.message : String(err);
		sqlbox.result = null;
	} finally {
		sqlbox.loading = false;
	}
}
