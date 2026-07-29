<script lang="ts">
	import type { PageProps } from './$types';
	import DataTable from '$lib/components/DataTable.svelte';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>{data.table.name} - {data.datasource.name} - Polyglot</title>
</svelte:head>

<main>
	<a class="back-link" href="/{encodeURIComponent(data.datasource.name)}">
		&larr; {data.datasource.name}
	</a>
	<h1>{data.table.name}</h1>
	{#if data.table.description}
		<p class="description">{data.table.description}</p>
	{/if}
	{#if data.table.query_guidance}
		<p class="guidance"><strong>Query guidance:</strong> {data.table.query_guidance}</p>
	{/if}

	<h2>Schema</h2>
	<div class="schema-table">
		<table>
			<thead>
				<tr>
					<th>Column</th>
					<th>Type</th>
					<th>Description</th>
				</tr>
			</thead>
			<tbody>
				{#each data.table.columns as col (col.id)}
					<tr>
						<td class="col-name">{col.name}</td>
						<td class="col-type">{col.type}</td>
						<td class="col-description">{col.description || '—'}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<h2>Data preview</h2>
	<!-- Keyed on table id: without this, navigating client-side between
	tables reuses the same DataTable instance, and its internal rows/
	truncated $state (only ever initialized from `initial`, per Svelte 5's
	own state_referenced_locally warning) would keep showing the PREVIOUS
	table's data until a sort/page interaction overwrote it. Keying forces
	a full remount, resetting state cleanly on every navigation. -->
	{#key data.table.id}
		<DataTable
			datasource={data.datasource.name}
			table={data.table.name}
			columns={data.table.columns}
			initial={data.initial}
		/>
	{/key}
</main>

<style>
	main {
		max-width: 1100px;
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
		font-family: monospace;
	}
	h2 {
		margin-top: 2rem;
		color: var(--accent, #5f9a6f);
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
	.schema-table {
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.75rem;
		box-shadow: var(--shadow, 0 2px 10px rgba(95, 154, 111, 0.18));
		overflow: hidden;
	}
	.schema-table table {
		border-collapse: collapse;
		width: 100%;
		font-size: 0.85rem;
	}
	.schema-table th {
		text-align: left;
		background: var(--muted-bg, #f1e6cd);
		padding: 0.5rem 0.75rem;
	}
	.schema-table td {
		padding: 0.4rem 0.75rem;
		border-top: 1px solid var(--border, #d8c8a0);
	}
	.col-name {
		font-family: monospace;
		font-weight: 600;
	}
	.col-type {
		color: var(--muted-fg, #8a7a54);
		white-space: nowrap;
	}
</style>
