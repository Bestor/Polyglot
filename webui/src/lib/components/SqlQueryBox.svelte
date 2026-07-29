<script lang="ts">
	import type { TableRows } from '$lib/types';

	let { datasource }: { datasource: string } = $props();

	let sql = $state('');
	let loading = $state(false);
	let error = $state<string | null>(null);
	let result = $state<TableRows | null>(null);

	const columns = $derived(result && result.rows.length > 0 ? Object.keys(result.rows[0]) : []);

	function formatCell(v: unknown): string {
		if (v === null || v === undefined) return '—';
		if (typeof v === 'object') return JSON.stringify(v);
		return String(v);
	}

	async function run() {
		const q = sql.trim();
		if (!q || loading) return;
		loading = true;
		error = null;
		try {
			const res = await fetch(`/api/query/${encodeURIComponent(datasource)}`, {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ sql: q })
			});
			if (!res.ok) throw new Error(await res.text());
			result = await res.json();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			result = null;
		} finally {
			loading = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			run();
		}
	}
</script>

<section class="sql-box">
	<h2>SQL query</h2>
	<textarea
		bind:value={sql}
		onkeydown={onKeydown}
		placeholder={`SELECT * FROM "table_name" LIMIT 50`}
		rows="4"
		disabled={loading}
	></textarea>
	<div class="sql-actions">
		<button type="button" onclick={run} disabled={loading || !sql.trim()}>
			{loading ? 'Running…' : 'Run query'}
		</button>
		<span class="hint">⌘/Ctrl + Enter to run</span>
	</div>

	{#if error}
		<div class="sql-error">{error}</div>
	{/if}

	{#if result}
		<div class="sql-meta">
			{result.row_count} row{result.row_count === 1 ? '' : 's'}{result.truncated
				? ' (truncated)'
				: ''}
		</div>
		{#if result.rows.length > 0}
			<div class="sql-results">
				<table>
					<thead>
						<tr>
							{#each columns as col (col)}
								<th>{col}</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each result.rows as row, i (i)}
							<tr>
								{#each columns as col (col)}
									<td>{formatCell(row[col])}</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</section>

<style>
	.sql-box {
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.75rem;
		box-shadow: var(--shadow, 0 2px 10px rgba(95, 154, 111, 0.18));
		padding: 1rem 1.25rem;
		margin-top: 1.5rem;
	}
	.sql-box h2 {
		margin: 0 0 0.6rem;
		font-size: 1rem;
		color: var(--accent, #5f9a6f);
	}
	textarea {
		width: 100%;
		box-sizing: border-box;
		font-family: monospace;
		font-size: 0.85rem;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.5rem;
		background: var(--bg, #faf5e9);
		resize: vertical;
		transition:
			border-color 0.15s ease,
			box-shadow 0.15s ease;
	}
	textarea:focus {
		outline: none;
		border-color: var(--accent, #5f9a6f);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent, #5f9a6f) 18%, transparent);
	}
	.sql-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 0.6rem;
	}
	button {
		padding: 0.45rem 1rem;
		border: none;
		border-radius: 0.5rem;
		background: var(--accent, #5f9a6f);
		color: #fff;
		font-weight: 600;
		cursor: pointer;
		transition:
			background 0.15s ease,
			transform 0.15s ease;
	}
	button:hover:not(:disabled) {
		background: var(--accent-hover, #4f8a5f);
		transform: translateY(-1px);
	}
	button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.hint {
		font-size: 0.75rem;
		color: var(--muted-fg, #8a7a54);
	}
	.sql-error {
		margin-top: 0.75rem;
		color: var(--error-fg, #a1502b);
		background: var(--error-bg, #f7e3d6);
		border-radius: 0.5rem;
		padding: 0.5rem 0.75rem;
		font-size: 0.85rem;
		font-family: monospace;
		white-space: pre-wrap;
	}
	.sql-meta {
		margin-top: 0.75rem;
		font-size: 0.8rem;
		color: var(--muted-fg, #8a7a54);
	}
	.sql-results {
		margin-top: 0.5rem;
		max-height: 420px;
		overflow: auto;
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.5rem;
	}
	table {
		border-collapse: collapse;
		width: 100%;
		font-size: 0.8rem;
	}
	th {
		position: sticky;
		top: 0;
		text-align: left;
		background: var(--accent-bg, #dcefdd);
		padding: 0.4rem 0.6rem;
		white-space: nowrap;
	}
	td {
		padding: 0.35rem 0.6rem;
		border-top: 1px solid var(--border, #d8c8a0);
		white-space: nowrap;
	}
	tr:hover td {
		background: color-mix(in srgb, var(--accent-bg, #dcefdd) 35%, transparent);
	}
</style>
