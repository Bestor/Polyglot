<script lang="ts">
	import type { QuerySummary, QueryDetail } from '$lib/types';

	let { query }: { query: QuerySummary } = $props();

	let expanded = $state(false);
	let detail = $state<QueryDetail | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);

	// Fetched lazily on first expand, then cached in `detail` for the rest
	// of the row's lifetime - re-collapsing/re-expanding doesn't refetch.
	async function toggle() {
		expanded = !expanded;
		if (expanded && !detail && !loading) {
			loading = true;
			error = null;
			try {
				const res = await fetch(`/api/queries/${encodeURIComponent(query.id)}`);
				if (!res.ok) throw new Error(await res.text());
				detail = await res.json();
			} catch (err) {
				error = err instanceof Error ? err.message : String(err);
			} finally {
				loading = false;
			}
		}
	}

	// Best effort: `response` is the query tool's raw result text, which is
	// usually JSON (see internal/polyglot's QueryResponse shape) but isn't
	// guaranteed to be - an error path or a non-query tool call could leave
	// something else here. Falls back to the raw string unchanged rather
	// than showing nothing if it doesn't parse.
	function formatResponse(response: string): string {
		try {
			return JSON.stringify(JSON.parse(response), null, 2);
		} catch {
			return response;
		}
	}
</script>

<div class="query-row" class:expanded>
	<button class="query-summary" onclick={toggle}>
		<span class="query-status query-status-{query.status}">{query.status}</span>
		<code class="query-sql">{query.sql}</code>
		<span class="query-timestamp">{new Date(query.timestamp).toLocaleString()}</span>
		<span class="query-duration">{query.duration_ms}ms</span>
	</button>

	{#if query.question}
		<div class="query-question">"{query.question}"</div>
	{/if}

	{#if expanded}
		<div class="query-detail">
			<h4>SQL</h4>
			<pre>{query.sql}</pre>

			{#if loading}
				<p>Loading…</p>
			{:else if error}
				<p class="query-error">{error}</p>
			{:else if detail}
				<h4>Response</h4>
				<pre>{formatResponse(detail.response)}</pre>
				<h4>Spans</h4>
				<table>
					<thead>
						<tr>
							<th>Service</th>
							<th>Operation</th>
							<th>Duration</th>
						</tr>
					</thead>
					<tbody>
						{#each detail.spans as span, i (i)}
							<tr>
								<td>{span.service}</td>
								<td>{span.operation_name}</td>
								<td>{span.duration_ms}ms</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	{/if}
</div>

<style>
	.query-row {
		border-bottom: 1px solid var(--border, #d8c8a0);
		border-radius: 0.5rem;
		padding: 0 0.5rem;
		margin: 0.15rem 0;
		transition:
			background 0.15s ease,
			box-shadow 0.15s ease;
	}
	.query-row:hover {
		background: color-mix(in srgb, var(--accent-bg, #dcefdd) 50%, transparent);
	}
	.query-row:last-child {
		border-bottom: none;
	}
	/* Shade the box once it's expanded ("selected") - a clear, persistent
	highlight distinct from the lighter hover state above. */
	.query-row.expanded {
		background: var(--accent-bg, #dcefdd);
		box-shadow: var(--shadow, 0 2px 10px rgba(95, 154, 111, 0.18));
	}
	.query-summary {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		width: 100%;
		padding: 0.6rem 0;
		background: none;
		border: none;
		text-align: left;
		cursor: pointer;
		font: inherit;
	}
	.query-status {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.1rem 0.5rem;
		border-radius: 999px;
	}
	.query-status-success {
		background: var(--success-bg, #dbeedb);
		color: var(--success-fg, #2f6b3f);
	}
	.query-status-error {
		background: var(--error-bg, #f7e3d6);
		color: var(--error-fg, #a1502b);
	}
	.query-sql {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 0.85rem;
	}
	.query-timestamp,
	.query-duration {
		font-size: 0.8rem;
		color: var(--muted-fg, #8a7a54);
		white-space: nowrap;
	}
	.query-question {
		font-size: 0.85rem;
		font-style: italic;
		color: var(--muted-fg, #8a7a54);
		padding-bottom: 0.5rem;
	}
	.query-detail {
		padding: 0.5rem 0 1rem;
	}
	.query-detail h4 {
		color: var(--accent, #5f9a6f);
		margin-bottom: 0.35rem;
	}
	.query-detail pre {
		white-space: pre-wrap;
		word-break: break-word;
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		padding: 0.5rem;
		border-radius: 0.5rem;
		max-height: 300px;
		overflow-y: auto;
	}
	.query-detail table {
		border-collapse: collapse;
		width: 100%;
		font-size: 0.85rem;
		background: var(--panel-bg, #fffdf8);
		border-radius: 0.5rem;
		overflow: hidden;
	}
	.query-detail th,
	.query-detail td {
		border: 1px solid var(--border, #d8c8a0);
		padding: 0.25rem 0.5rem;
		text-align: left;
	}
	.query-detail th {
		background: var(--muted-bg, #f1e6cd);
	}
	.query-error {
		color: var(--error-fg, #a1502b);
		background: var(--error-bg, #f7e3d6);
		border-radius: 0.5rem;
		padding: 0.5rem 0.75rem;
	}
</style>
