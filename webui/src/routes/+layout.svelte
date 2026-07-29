<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import ChatPanel from '$lib/components/ChatPanel.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import SqlQueryBox from '$lib/components/SqlQueryBox.svelte';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<Sidebar />
<ChatPanel />
<SqlQueryBox />

<div class="content">
	{@render children()}
</div>

<style>
	.content {
		margin-left: var(--sidebar-width, 220px);
	}
</style>
