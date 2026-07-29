<script lang="ts">
	import { page } from '$app/state';
	import { chat } from '$lib/stores/chat.svelte';

	// Highlights the Explorer link for both "/" and any drill-down route
	// under it (/[datasource], /[datasource]/[table]) - only "/queries"
	// itself should highlight the Recent Queries link.
	const onExplorer = $derived(page.url.pathname === '/' || !page.url.pathname.startsWith('/queries'));
</script>

<nav class="sidebar">
	<div class="sidebar-title">Polyglot</div>

	<a class="nav-link" href="/" class:active={onExplorer}>Explorer</a>

	<button class="nav-link nav-button" class:active={chat.open} onclick={() => (chat.open = !chat.open)}>
		Ask
	</button>

	<!-- Deliberately the only link to /queries anywhere in the app - Recent
	Queries is a secondary page, sidebar-only, per the explicit ask. -->
	<a class="nav-link" href="/queries" class:active={page.url.pathname === '/queries'}>
		Recent Queries
	</a>
</nav>

<style>
	.sidebar {
		position: fixed;
		top: 0;
		left: 0;
		bottom: 0;
		width: var(--sidebar-width, 220px);
		background: var(--panel-bg, #fffdf8);
		border-right: 1px solid var(--border, #d8c8a0);
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 1rem 0.75rem;
		z-index: 50;
	}
	.sidebar-title {
		font-weight: 700;
		color: var(--accent, #5f9a6f);
		padding: 0 0.5rem 1rem;
		letter-spacing: -0.01em;
	}
	.nav-link {
		display: block;
		width: 100%;
		text-align: left;
		padding: 0.5rem 0.75rem;
		border-radius: 0.5rem;
		border: none;
		background: none;
		color: #3f3826;
		text-decoration: none;
		font: inherit;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.15s ease;
	}
	.nav-link:hover {
		background: var(--muted-bg, #f1e6cd);
	}
	.nav-link.active {
		background: var(--accent-bg, #dcefdd);
		color: var(--accent-hover, #4f8a5f);
		font-weight: 700;
	}
</style>
