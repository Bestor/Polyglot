<script lang="ts">
	import '../app.css';
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
		padding: 0.5rem 1.25rem;
		border: none;
		border-radius: 999px;
		background: var(--accent, #5f9a6f);
		color: #fff;
		font-weight: 600;
		cursor: pointer;
		box-shadow: var(--shadow, 0 2px 10px rgba(95, 154, 111, 0.18));
		transition:
			transform 0.15s ease,
			box-shadow 0.15s ease,
			background 0.15s ease;
	}
	.ask-button:hover {
		background: var(--accent-hover, #4f8a5f);
		transform: translateY(-1px);
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(95, 154, 111, 0.22));
	}
	.ask-button:active {
		transform: translateY(0);
	}
</style>
