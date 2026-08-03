<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>Polyglot</title>
</svelte:head>

<main>
	<h1>Data Explorer</h1>
	{#if data.datasources.length === 0}
		<p class="empty">
			No datasources onboarded yet. Onboard one via <code>POST /datasources</code> on polyglot.
		</p>
	{:else}
		<div class="datasource-grid">
			{#each data.datasources as ds (ds.name)}
				<a class="datasource-card" href="/{encodeURIComponent(ds.name)}">
					<h2>{ds.name}</h2>
					{#if ds.description}
						<p class="description">{ds.description}</p>
					{/if}
					<div class="badges">
						<span class="table-count">{ds.tableCount} table{ds.tableCount === 1 ? '' : 's'}</span>
						{#if ds.glossary.length > 0}
							<span class="glossary-count"
								>{ds.glossary.length} glossary term{ds.glossary.length === 1 ? '' : 's'}</span
							>
						{/if}
					</div>
				</a>
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
	.datasource-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 1rem;
	}
	.datasource-card {
		display: block;
		background: var(--panel-bg, #fffdf8);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.75rem;
		box-shadow: var(--shadow, 0 2px 10px rgba(95, 154, 111, 0.18));
		padding: 1.25rem;
		text-decoration: none;
		color: inherit;
		transition:
			transform 0.15s ease,
			box-shadow 0.15s ease,
			border-color 0.15s ease;
	}
	.datasource-card:hover {
		transform: translateY(-2px);
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(95, 154, 111, 0.22));
		border-color: var(--accent, #5f9a6f);
	}
	.datasource-card h2 {
		margin: 0 0 0.4rem;
		color: var(--accent, #5f9a6f);
	}
	.description {
		font-size: 0.9rem;
		color: var(--muted-fg, #8a7a54);
		margin: 0 0 0.75rem;
	}
	.badges {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.table-count,
	.glossary-count {
		font-size: 0.8rem;
		font-weight: 600;
		background: var(--accent-bg, #dcefdd);
		color: var(--accent-hover, #4f8a5f);
		padding: 0.15rem 0.5rem;
		border-radius: 999px;
	}
	.glossary-count {
		background: var(--muted-bg, #f1e6cd);
		color: var(--muted-fg, #8a7a54);
	}
</style>
