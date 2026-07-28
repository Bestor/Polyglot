import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import type Anthropic from '@anthropic-ai/sdk';
import { runTurn } from '$lib/server/agent';

interface AskBody {
	history: Anthropic.MessageParam[];
	question: string;
}

export const POST: RequestHandler = async ({ request }) => {
	const { history, question } = (await request.json()) as AskBody;
	try {
		const updated = await runTurn([...history, { role: 'user', content: question }]);
		return json({ history: updated });
	} catch (e) {
		throw error(500, e instanceof Error ? e.message : 'chat failed');
	}
};
