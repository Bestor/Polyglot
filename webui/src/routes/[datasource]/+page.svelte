<script lang="ts">
	import type { PageProps } from './$types';
	import SqlQueryBox from '$lib/components/SqlQueryBox.svelte';
	import WarmFunctionCard from '$lib/components/WarmFunctionCard.svelte';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>{data.datasource.name} - Polyglot</title>
</svelte:head>

<main>
	<a class="back-link" href="/">&larr; All datasources</a>
	<h1>{data.datasource.name}</h1>
	{#if data.datasource.description}
		<p class="description">{data.datasource.description}</p>
	{/if}
	{#if data.datasource.query_guidance}
		<p class="guidance"><strong>Query guidance:</strong> {data.datasource.query_guidance}</p>
	{/if}

	<SqlQueryBox datasource={data.datasource.name} />

	<div class="explorer-layout">
		{#if data.tables.length === 0}
			<p class="empty">This datasource has no tables in polyglot's catalog yet.</p>
		{:else}
			<div class="table-list">
				{#each data.tables as table (table.id)}
					<a
						class="table-row"
						href="/{encodeURIComponent(data.datasource.name)}/{encodeURIComponent(table.name)}"
					>
						<span class="table-name">{table.name}</span>
						{#if table.description}
							<span class="table-description">{table.description}</span>
						{/if}
						<span class="column-count"
							>{table.columns.length} column{table.columns.length === 1 ? '' : 's'}</span
						>
					</a>
				{/each}
			</div>
		{/if}

		<aside class="functions-panel">
			<h2>Cache warm functions</h2>
			{#if data.functions.length === 0}
				<p class="empty-panel">This datasource exposes no warm functions.</p>
			{:else}
				<div class="function-list">
					{#each data.functions as fn (fn.id)}
						<WarmFunctionCard datasource={data.datasource.name} {fn} />
					{/each}
				</div>
			{/if}
		</aside>
	</div>
</main>

<style>
	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2.5rem 1rem;
	}
	.back-link {
		color: var(--muted-fg, #8a7a54);
		text-decoration: none;
		font-size: 0.9rem;
	}
	.back-link:hover {
		color: var(--accent, #5f9a6f);
	}
	h1 {
		color: var(--accent, #5f9a6f);
		letter-spacing: -0.01em;
		margin-top: 0.5rem;
	}
	.description {
		color: var(--muted-fg, #8a7a54);
	}
	.guidance {
		font-size: 0.9rem;
		background: var(--muted-bg, #f1e6cd);
		border-radius: 0.5rem;
		padding: 0.5rem 0.75rem;
	}
	.empty {
		color: var(--muted-fg, #8a7a54);
		background: var(--panel-bg, #fffdf8);
		border: 1px dashed var(--border, #d8c8a0);
		border-radius: 0.75rem;
		padding: 1.5rem;
	}
	.explorer-layout {
		display: flex;
		align-items: flex-start;
		gap: 1.5rem;
		margin-top: 1.5rem;
	}
	.table-list {
		flex: 2;
		min-width: 0;
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.75rem;
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(95, 154, 111, 0.22));
		padding: 0.5rem 0.75rem;
	}
	.table-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.6rem 0.5rem;
		border-bottom: 1px solid var(--border, #d8c8a0);
		text-decoration: none;
		color: inherit;
		border-radius: 0.5rem;
		transition: background 0.15s ease;
	}
	.table-row:last-child {
		border-bottom: none;
	}
	.table-row:hover {
		background: color-mix(in srgb, var(--accent-bg, #dcefdd) 50%, transparent);
	}
	.table-name {
		font-weight: 600;
		font-family: monospace;
	}
	.table-description {
		flex: 1;
		color: var(--muted-fg, #8a7a54);
		font-size: 0.85rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.column-count {
		font-size: 0.75rem;
		font-weight: 600;
		background: var(--muted-bg, #f1e6cd);
		color: var(--muted-fg, #8a7a54);
		padding: 0.1rem 0.5rem;
		border-radius: 999px;
		white-space: nowrap;
	}

	.functions-panel {
		flex: 1;
		min-width: 260px;
		max-width: 360px;
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.75rem;
		box-shadow: var(--shadow, 0 2px 10px rgba(95, 154, 111, 0.18));
		padding: 1rem;
	}
	.functions-panel h2 {
		margin: 0 0 0.75rem;
		font-size: 1rem;
		color: var(--accent, #5f9a6f);
	}
	.empty-panel {
		color: var(--muted-fg, #8a7a54);
		font-size: 0.85rem;
		margin: 0;
	}
	.function-list {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
	}

	@media (max-width: 780px) {
		.explorer-layout {
			flex-direction: column;
		}
		.functions-panel {
			max-width: none;
			width: 100%;
		}
	}
</style>
