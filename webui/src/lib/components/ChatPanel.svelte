<script lang="ts">
	import { browser } from '$app/environment';
	import { marked } from 'marked';
	import DOMPurify from 'dompurify';
	import { chat, ask } from '$lib/stores/chat.svelte';
	import type Anthropic from '@anthropic-ai/sdk';

	let question = $state('');

	// Assistant/user turns can carry a plain string (a user's typed
	// question) or an array of content blocks (an assistant's text +
	// tool_use blocks, or a user-role tool_result turn fed back after a
	// tool call) - only the text portion is ever shown as a bubble;
	// tool_use/tool_result blocks are plumbing for the API, not
	// conversation content.
	function textOf(content: Anthropic.MessageParam['content']): string {
		if (typeof content === 'string') return content;
		return content
			.filter((b): b is Anthropic.TextBlockParam => b.type === 'text')
			.map((b) => b.text)
			.join('');
	}

	function renderMarkdown(text: string): string {
		const html = marked.parse(text, { async: false });
		// DOMPurify needs a real DOM (window/document), unavailable during
		// SSR - safe to skip server-side since the panel starts closed
		// (chat.open defaults to false), so this never needs to render
		// anything meaningful before hydration.
		return browser ? DOMPurify.sanitize(html) : '';
	}

	async function submit(e: Event) {
		e.preventDefault();
		const q = question.trim();
		if (!q || chat.pending) return;
		question = '';
		await ask(q);
	}

	const visibleTurns = $derived(
		chat.history
			.map((m, i) => ({ role: m.role, text: textOf(m.content), key: i }))
			.filter((t) => t.text.trim() !== '')
	);
</script>

{#if chat.open}
	<div class="chat-panel">
		<div class="chat-header">
			<strong>Ask</strong>
			<button class="chat-close" onclick={() => (chat.open = false)} aria-label="Close">×</button>
		</div>
		<div class="chat-history">
			{#each visibleTurns as turn (turn.key)}
				<div class="chat-turn chat-turn-{turn.role}">
					{#if turn.role === 'user'}
						<p>{turn.text}</p>
					{:else}
						{@html renderMarkdown(turn.text)}
					{/if}
				</div>
			{/each}
			{#if chat.pending}
				<div class="chat-turn chat-turn-assistant chat-pending">Thinking…</div>
			{/if}
			{#if chat.error}
				<div class="chat-error">{chat.error}</div>
			{/if}
		</div>
		<form onsubmit={submit}>
			<input
				type="text"
				placeholder="Ask a stats question..."
				bind:value={question}
				disabled={chat.pending}
			/>
			<button type="submit" disabled={chat.pending || !question.trim()}>Send</button>
		</form>
	</div>
{/if}

<style>
	.chat-panel {
		position: fixed;
		top: 0;
		left: 0;
		bottom: 0;
		width: min(420px, 100vw);
		background: var(--bg, #fff);
		border-right: 1px solid var(--border, #ddd);
		display: flex;
		flex-direction: column;
		z-index: 100;
		box-shadow: 2px 0 8px rgba(0, 0, 0, 0.1);
	}
	.chat-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--border, #ddd);
	}
	.chat-close {
		background: none;
		border: none;
		font-size: 1.25rem;
		cursor: pointer;
		line-height: 1;
	}
	.chat-history {
		flex: 1;
		overflow-y: auto;
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.chat-turn {
		padding: 0.5rem 0.75rem;
		border-radius: 0.5rem;
		max-width: 90%;
	}
	.chat-turn-user {
		align-self: flex-end;
		background: var(--accent-bg, #dbeafe);
	}
	.chat-turn-assistant {
		align-self: flex-start;
		background: var(--muted-bg, #f3f4f6);
	}
	.chat-turn-assistant :global(table) {
		border-collapse: collapse;
		width: 100%;
	}
	.chat-turn-assistant :global(th),
	.chat-turn-assistant :global(td) {
		border: 1px solid var(--border, #ddd);
		padding: 0.25rem 0.5rem;
		text-align: left;
	}
	.chat-pending {
		opacity: 0.6;
		font-style: italic;
	}
	.chat-error {
		color: #b91c1c;
		font-size: 0.9rem;
	}
	form {
		display: flex;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border-top: 1px solid var(--border, #ddd);
	}
	input {
		flex: 1;
		padding: 0.5rem;
	}
</style>
