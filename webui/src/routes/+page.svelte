<script lang="ts">
	import QueryRow from '$lib/components/QueryRow.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>val-analyzer</title>
</svelte:head>

<main>
	<h1>Recent queries</h1>
	{#if data.queries.length === 0}
		<p class="empty">
			No recent queries. Either nothing has queried the backend yet, or tracing isn't configured
			(recent-queries history is read from Jaeger, not stored separately - see
			JAEGER_QUERY_URL).
		</p>
	{:else}
		<div class="query-list">
			{#each data.queries as query (query.id)}
				<QueryRow {query} />
			{/each}
		</div>
	{/if}
</main>

<style>
	main {
		max-width: 900px;
		margin: 0 auto;
		padding: 2.5rem 1rem;
	}
	h1 {
		color: var(--accent, #5f9a6f);
		letter-spacing: -0.01em;
	}
	.empty {
		color: var(--muted-fg, #8a7a54);
		background: var(--panel-bg, #fffdf8);
		border: 1px dashed var(--border, #d8c8a0);
		border-radius: 0.75rem;
		padding: 1.5rem;
	}
	.query-list {
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.75rem;
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(95, 154, 111, 0.22));
		padding: 0.5rem 0.75rem;
	}
</style>
