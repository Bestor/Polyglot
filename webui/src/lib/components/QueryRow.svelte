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
</script>

<div class="query-row">
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
			{#if loading}
				<p>Loading…</p>
			{:else if error}
				<p class="query-error">{error}</p>
			{:else if detail}
				<h4>Response</h4>
				<pre>{detail.response}</pre>
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
		border-bottom: 1px solid var(--border, #ddd);
	}
	.query-summary {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		width: 100%;
		padding: 0.5rem 0;
		background: none;
		border: none;
		text-align: left;
		cursor: pointer;
		font: inherit;
	}
	.query-status {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.1rem 0.4rem;
		border-radius: 0.25rem;
	}
	.query-status-success {
		background: #dcfce7;
		color: #166534;
	}
	.query-status-error {
		background: #fee2e2;
		color: #991b1b;
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
		color: var(--muted-fg, #6b7280);
		white-space: nowrap;
	}
	.query-question {
		font-size: 0.85rem;
		font-style: italic;
		color: var(--muted-fg, #6b7280);
		padding-bottom: 0.5rem;
	}
	.query-detail {
		padding: 0.5rem 0 1rem;
	}
	.query-detail pre {
		white-space: pre-wrap;
		word-break: break-word;
		background: var(--muted-bg, #f3f4f6);
		padding: 0.5rem;
		border-radius: 0.25rem;
		max-height: 300px;
		overflow-y: auto;
	}
	.query-detail table {
		border-collapse: collapse;
		width: 100%;
		font-size: 0.85rem;
	}
	.query-detail th,
	.query-detail td {
		border: 1px solid var(--border, #ddd);
		padding: 0.25rem 0.5rem;
		text-align: left;
	}
	.query-error {
		color: #b91c1c;
	}
</style>
