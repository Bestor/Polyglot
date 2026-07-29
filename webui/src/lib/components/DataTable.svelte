<script lang="ts">
	import type { ColumnDescription, TableRows } from '$lib/types';

	const PAGE_SIZE = 50;

	let {
		datasource,
		table,
		columns,
		initial
	}: {
		datasource: string;
		table: string;
		columns: ColumnDescription[];
		initial: TableRows;
	} = $props();

	// initial only seeds state at mount, deliberately (an uncontrolled-
	// component pattern - the parent remounts this whole component via
	// {#key} on table change, rather than this reacting to prop updates
	// after mount). svelte-check's state_referenced_locally warning below
	// is a known false positive for exactly this pattern.
	let rows = $state(initial.rows);
	let truncated = $state(initial.truncated);
	let offset = $state(0);
	let sort = $state<string | null>(null);
	let dir = $state<'asc' | 'desc'>('asc');
	let loading = $state(false);
	let error = $state<string | null>(null);

	async function fetchPage() {
		loading = true;
		error = null;
		try {
			const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) });
			if (sort) {
				params.set('sort', sort);
				params.set('dir', dir);
			}
			const res = await fetch(
				`/api/explorer/${encodeURIComponent(datasource)}/${encodeURIComponent(table)}?${params}`
			);
			if (!res.ok) throw new Error(await res.text());
			const data: TableRows = await res.json();
			rows = data.rows;
			truncated = data.truncated;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			loading = false;
		}
	}

	function toggleSort(col: string) {
		if (sort === col) {
			dir = dir === 'asc' ? 'desc' : 'asc';
		} else {
			sort = col;
			dir = 'asc';
		}
		offset = 0;
		fetchPage();
	}

	function next() {
		offset += PAGE_SIZE;
		fetchPage();
	}

	function prev() {
		offset = Math.max(0, offset - PAGE_SIZE);
		fetchPage();
	}
</script>

<div class="data-table-wrap">
	{#if error}
		<p class="error">{error}</p>
	{/if}
	<div class="scroll">
		<table class:loading>
			<thead>
				<tr>
					{#each columns as col (col.id)}
						<th>
							<button class="sort-button" onclick={() => toggleSort(col.name)}>
								{col.name}
								{#if sort === col.name}
									<span class="sort-arrow">{dir === 'asc' ? '▲' : '▼'}</span>
								{/if}
							</button>
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each rows as row, i (i)}
					<tr>
						{#each columns as col (col.id)}
							<td>{row[col.name] ?? ''}</td>
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
	<div class="pager">
		<button onclick={prev} disabled={offset === 0 || loading}>&larr; Previous</button>
		<span class="pager-status">
			{#if loading}
				Loading…
			{:else}
				Rows {offset + 1}&ndash;{offset + rows.length}
			{/if}
		</span>
		<button onclick={next} disabled={!truncated || loading}>Next &rarr;</button>
	</div>
</div>

<style>
	.data-table-wrap {
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.75rem;
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(95, 154, 111, 0.22));
		overflow: hidden;
	}
	.error {
		color: var(--error-fg, #a1502b);
		background: var(--error-bg, #f7e3d6);
		padding: 0.5rem 0.75rem;
		margin: 0;
	}
	.scroll {
		overflow-x: auto;
		max-height: 500px;
		overflow-y: auto;
	}
	table {
		border-collapse: collapse;
		width: 100%;
		font-size: 0.85rem;
		transition: opacity 0.15s ease;
	}
	table.loading {
		opacity: 0.5;
	}
	th {
		position: sticky;
		top: 0;
		background: var(--muted-bg, #f1e6cd);
		text-align: left;
		border-bottom: 1px solid var(--border, #d8c8a0);
	}
	.sort-button {
		width: 100%;
		text-align: left;
		background: none;
		border: none;
		font: inherit;
		font-weight: 600;
		cursor: pointer;
		padding: 0.5rem 0.75rem;
		white-space: nowrap;
	}
	.sort-arrow {
		color: var(--accent, #5f9a6f);
		font-size: 0.7rem;
	}
	td {
		padding: 0.4rem 0.75rem;
		border-bottom: 1px solid var(--border, #d8c8a0);
		white-space: nowrap;
		max-width: 300px;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	tbody tr:hover {
		background: color-mix(in srgb, var(--accent-bg, #dcefdd) 40%, transparent);
	}
	.pager {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		padding: 0.6rem;
		border-top: 1px solid var(--border, #d8c8a0);
	}
	.pager button {
		padding: 0.3rem 0.75rem;
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.5rem;
		background: var(--panel-bg, #fffdf8);
		cursor: pointer;
		transition: background 0.15s ease;
	}
	.pager button:hover:not(:disabled) {
		background: var(--accent-bg, #dcefdd);
	}
	.pager button:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.pager-status {
		font-size: 0.85rem;
		color: var(--muted-fg, #8a7a54);
	}
</style>
