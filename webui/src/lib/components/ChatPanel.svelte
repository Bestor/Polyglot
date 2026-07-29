<script lang="ts">
	import { browser } from '$app/environment';
	import { marked } from 'marked';
	import DOMPurify from 'dompurify';
	import { chat, ask } from '$lib/stores/chat.svelte';
	import type Anthropic from '@anthropic-ai/sdk';

	let question = $state('');

	// Native CSS `resize: horizontal` is unreliable on `position: fixed`
	// elements across browsers (the drag interaction doesn't consistently
	// commit a new width) - a manual pointer-driven handle instead, which
	// behaves consistently everywhere.
	const MIN_WIDTH = 280;
	const DEFAULT_WIDTH = 420;
	let panelWidth = $state(DEFAULT_WIDTH);
	let resizing = $state(false);

	function maxWidth(): number {
		const sidebarWidth = browser
			? parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--sidebar-width')) ||
				220
			: 220;
		return window.innerWidth * 0.9 - sidebarWidth;
	}

	function startResize(e: PointerEvent) {
		e.preventDefault();
		resizing = true;
		const startX = e.clientX;
		const startWidth = panelWidth;
		const limit = maxWidth();
		const prevUserSelect = document.body.style.userSelect;
		document.body.style.userSelect = 'none';

		function onMove(ev: PointerEvent) {
			const next = startWidth + (ev.clientX - startX);
			panelWidth = Math.min(limit, Math.max(MIN_WIDTH, next));
		}
		function onUp() {
			resizing = false;
			document.body.style.userSelect = prevUserSelect;
			window.removeEventListener('pointermove', onMove);
			window.removeEventListener('pointerup', onUp);
		}
		window.addEventListener('pointermove', onMove);
		window.addEventListener('pointerup', onUp);
	}

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
	<div class="chat-panel" class:resizing style="width: {panelWidth}px">
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
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="resize-handle"
			role="separator"
			aria-orientation="vertical"
			aria-label="Resize chat panel"
			onpointerdown={startResize}
		></div>
	</div>
{/if}

<style>
	.chat-panel {
		position: fixed;
		top: 0;
		/* Docks immediately right of the persistent sidebar, not over it -
		Sidebar.svelte occupies left:0 to var(--sidebar-width). */
		left: var(--sidebar-width, 220px);
		bottom: 0;
		min-width: 280px;
		background: var(--panel-bg, #fffdf8);
		border-right: 1px solid var(--border, #d8c8a0);
		display: flex;
		flex-direction: column;
		z-index: 100;
		box-shadow: var(--shadow-lg, 0 8px 24px rgba(95, 154, 111, 0.22));
		overflow: hidden;
	}
	.resize-handle {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
		width: 6px;
		cursor: ew-resize;
		touch-action: none;
		z-index: 101;
	}
	.resize-handle:hover,
	.chat-panel.resizing .resize-handle {
		background: color-mix(in srgb, var(--accent, #5f9a6f) 45%, transparent);
	}
	.chat-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1rem;
		background: linear-gradient(135deg, var(--accent-bg, #dcefdd), var(--panel-bg, #fffdf8));
		border-bottom: 1px solid var(--border, #d8c8a0);
	}
	.chat-header strong {
		color: var(--accent, #5f9a6f);
		letter-spacing: 0.02em;
	}
	.chat-close {
		background: none;
		border: none;
		font-size: 1.25rem;
		cursor: pointer;
		line-height: 1;
		color: var(--muted-fg, #8a7a54);
		border-radius: 999px;
		width: 1.75rem;
		height: 1.75rem;
		transition:
			background 0.15s ease,
			color 0.15s ease;
	}
	.chat-close:hover {
		background: var(--muted-bg, #f1e6cd);
		color: #3f3826;
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
		border-radius: 0.75rem;
		max-width: 90%;
		box-shadow: var(--shadow, 0 2px 10px rgba(95, 154, 111, 0.18));
	}
	.chat-turn-user {
		align-self: flex-end;
		background: var(--accent-bg, #dcefdd);
		border: 1px solid color-mix(in srgb, var(--accent, #5f9a6f) 25%, transparent);
	}
	.chat-turn-assistant {
		align-self: flex-start;
		background: var(--muted-bg, #f1e6cd);
		border: 1px solid var(--border, #d8c8a0);
	}
	.chat-turn-assistant :global(table) {
		border-collapse: collapse;
		width: 100%;
	}
	.chat-turn-assistant :global(th),
	.chat-turn-assistant :global(td) {
		border: 1px solid var(--border, #d8c8a0);
		padding: 0.25rem 0.5rem;
		text-align: left;
	}
	.chat-turn-assistant :global(th) {
		background: var(--accent-bg, #dcefdd);
	}
	.chat-pending {
		opacity: 0.7;
		font-style: italic;
	}
	.chat-error {
		color: var(--error-fg, #a1502b);
		background: var(--error-bg, #f7e3d6);
		border-radius: 0.5rem;
		padding: 0.5rem 0.75rem;
		font-size: 0.9rem;
	}
	form {
		display: flex;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		background: var(--panel-bg, #fffdf8);
		border-top: 1px solid var(--border, #d8c8a0);
	}
	input {
		flex: 1;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.5rem;
		background: var(--bg, #faf5e9);
		transition:
			border-color 0.15s ease,
			box-shadow 0.15s ease;
	}
	input:focus {
		outline: none;
		border-color: var(--accent, #5f9a6f);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent, #5f9a6f) 18%, transparent);
	}
	button[type='submit'] {
		padding: 0.5rem 1rem;
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
	button[type='submit']:hover:not(:disabled) {
		background: var(--accent-hover, #4f8a5f);
		transform: translateY(-1px);
	}
	button[type='submit']:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
