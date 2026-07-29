<script lang="ts">
	import { sqlbox, runSqlQuery } from '$lib/stores/sqlbox.svelte';

	function formatCell(v: unknown): string {
		if (v === null || v === undefined) return '—';
		if (typeof v === 'object') return JSON.stringify(v);
		return String(v);
	}

	const columns = $derived(
		sqlbox.result && sqlbox.result.rows.length > 0 ? Object.keys(sqlbox.result.rows[0]) : []
	);

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			runSqlQuery();
		}
	}
</script>

{#if sqlbox.open}
	<div class="sql-box">
		<div class="sql-header">
			<strong>SQL query{sqlbox.datasource ? ` — ${sqlbox.datasource}` : ''}</strong>
			<button class="sql-close" onclick={() => (sqlbox.open = false)} aria-label="Close">×</button>
		</div>
		<div class="sql-body">
			<textarea
				bind:value={sqlbox.sql}
				onkeydown={onKeydown}
				placeholder={`SELECT * FROM "table_name" LIMIT 50`}
				rows="4"
				disabled={sqlbox.loading}
			></textarea>
			<div class="sql-actions">
				<button
					class="run-btn"
					type="button"
					onclick={runSqlQuery}
					disabled={sqlbox.loading || !sqlbox.sql.trim()}
				>
					{sqlbox.loading ? 'Running…' : 'Run query'}
				</button>
				<span class="hint">⌘/Ctrl + Enter to run</span>
			</div>

			{#if sqlbox.error}
				<div class="sql-error">{sqlbox.error}</div>
			{/if}

			{#if sqlbox.result}
				<div class="sql-meta">
					{sqlbox.result.row_count} row{sqlbox.result.row_count === 1 ? '' : 's'}{sqlbox.result
						.truncated
						? ' (truncated)'
						: ''}
				</div>
				{#if sqlbox.result.rows.length > 0}
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
								{#each sqlbox.result.rows as row, i (i)}
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
		</div>
	</div>
{/if}

<style>
	/* Fixed + floats above whatever content is scrolled beneath it (page
	title, table list, table data preview) rather than pushing that content
	down - it's meant to stay reachable while browsing between a
	datasource's tables, not act like a page section. */
	.sql-box {
		position: fixed;
		top: 1rem;
		right: 1rem;
		width: min(560px, calc(100vw - var(--sidebar-width, 220px) - 2rem));
		max-height: calc(100vh - 2rem);
		display: flex;
		flex-direction: column;
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.75rem;
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(95, 154, 111, 0.22));
		z-index: 90;
		overflow: hidden;
	}
	.sql-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.65rem 1rem;
		background: linear-gradient(135deg, var(--accent-bg, #dcefdd), var(--panel-bg, #fffdf8));
		border-bottom: 1px solid var(--border, #d8c8a0);
	}
	.sql-header strong {
		color: var(--accent, #5f9a6f);
		letter-spacing: 0.02em;
		font-size: 0.9rem;
	}
	.sql-close {
		background: none;
		border: none;
		font-size: 1.25rem;
		cursor: pointer;
		line-height: 1;
		color: var(--muted-fg, #8a7a54);
		border-radius: 999px;
		width: 1.75rem;
		height: 1.75rem;
		flex-shrink: 0;
		transition:
			background 0.15s ease,
			color 0.15s ease;
	}
	.sql-close:hover {
		background: var(--muted-bg, #f1e6cd);
		color: #3f3826;
	}
	.sql-body {
		padding: 1rem;
		overflow-y: auto;
	}
	textarea {
		width: 100%;
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
	.run-btn {
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
	.run-btn:hover:not(:disabled) {
		background: var(--accent-hover, #4f8a5f);
		transform: translateY(-1px);
	}
	.run-btn:disabled {
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
		max-height: 320px;
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
