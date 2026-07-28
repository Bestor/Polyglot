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
		padding: 2rem 1rem;
	}
	.empty {
		color: var(--muted-fg, #6b7280);
	}
	.query-list {
		border-top: 1px solid var(--border, #ddd);
	}
</style>
