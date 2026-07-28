import type Anthropic from '@anthropic-ai/sdk';

// Svelte 5 runes store: a reactive object shared by every importer,
// module-scoped for the tab's lifetime. This IS "conversation history
// lives in the browser" - no server persistence, so it's gone on refresh
// (acceptable for a personal-project chat panel; the fix, if that's ever a
// problem in practice, would be sessionStorage/localStorage sync, not a
// server-side change - the server is designed to be stateless).
export const chat = $state<{
	open: boolean;
	history: Anthropic.MessageParam[];
	pending: boolean;
	error: string | null;
}>({ open: false, history: [], pending: false, error: null });

export async function ask(question: string) {
	chat.pending = true;
	chat.error = null;
	try {
		const res = await fetch('/api/ask', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ history: chat.history, question })
		});
		if (!res.ok) throw new Error(await res.text());
		chat.history = (await res.json()).history;
	} catch (err) {
		chat.error = err instanceof Error ? err.message : String(err);
	} finally {
		chat.pending = false;
	}
}
