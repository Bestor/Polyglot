<script lang="ts">
	import type { ExampleQuery } from '$lib/types';
	import { sqlbox, runSqlQuery } from '$lib/stores/sqlbox.svelte';

	let { queries }: { queries: ExampleQuery[] } = $props();

	// Populates and runs the floating SQL query box - sqlbox.datasource is
	// kept in sync with the current route by Sidebar.svelte's own $effect,
	// so this only needs to supply the SQL text.
	function tryQuery(sql: string) {
		sqlbox.sql = sql;
		sqlbox.open = true;
		runSqlQuery();
	}
</script>

<div class="example-query-list">
	{#each queries as ex, i (i)}
		<div class="example-query">
			<div class="example-header">
				<p class="example-question">{ex.question}</p>
				<button class="try-btn" onclick={() => tryQuery(ex.sql)}>Try it</button>
			</div>
			<pre class="example-sql">{ex.sql}</pre>
		</div>
	{/each}
</div>

<style>
	.example-query-list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.example-query {
		background: var(--muted-bg, #f1e6cd);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.6rem;
		padding: 0.6rem 0.75rem;
	}
	.example-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
	}
	.example-question {
		margin: 0;
		font-weight: 600;
		font-size: 0.9rem;
	}
	.example-sql {
		margin: 0.5rem 0 0;
		font-family: monospace;
		font-size: 0.8rem;
		white-space: pre-wrap;
		word-break: break-word;
		color: var(--muted-fg, #8a7a54);
	}
	.try-btn {
		flex-shrink: 0;
		padding: 0.25rem 0.7rem;
		border: none;
		border-radius: 0.4rem;
		background: var(--accent, #5f9a6f);
		color: #fff;
		font-weight: 600;
		font-size: 0.75rem;
		cursor: pointer;
		transition: background 0.15s ease;
	}
	.try-btn:hover {
		background: var(--accent-hover, #4f8a5f);
	}
</style>
