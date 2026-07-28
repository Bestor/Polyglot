<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import ChatPanel from '$lib/components/ChatPanel.svelte';
	import { chat } from '$lib/stores/chat.svelte';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<button class="ask-button" onclick={() => (chat.open = !chat.open)}>Ask</button>
<ChatPanel />

{@render children()}

<style>
	.ask-button {
		position: fixed;
		top: 1rem;
		left: 1rem;
		z-index: 101;
		padding: 0.5rem 1rem;
		border: none;
		border-radius: 0.375rem;
		background: #2563eb;
		color: white;
		font-weight: 600;
		cursor: pointer;
	}
</style>
