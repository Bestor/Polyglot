import type Anthropic from '@anthropic-ai/sdk';
import { getAnthropicClient } from './anthropic';
import { getMcpSession } from './mcp';
import { getAnthropicTools } from './tools';
import { config } from './config';

const MAX_TOOL_ITERATIONS = 8;
const ANSWER_MAX_TOKENS = 8192;

// Ported from internal/discordbot/agent.go's systemPrompt. The first two
// paragraphs are unchanged; the third (originally Discord-specific
// formatting guidance, since Discord's markdown renderer doesn't support
// pipe tables) is rewritten for a web UI - the chat panel renders full
// markdown, so tables are expected here, not avoided.
const SYSTEM_PROMPT = `You are answering Valorant statistics questions using tools backed by a local cache (query, getMetadata) and, separately, a tool that reaches out to a rate-limited upstream API (warm, polled via getJob).

Only call warm if the user has explicitly asked you to refresh, update, or sync the cache. Never call it just because a query came back empty or incomplete - warm runs in the background and will not return usable data in time to help answer the current question. If the data you need isn't cached, say so plainly instead of trying to warm it yourself.

Your answer is rendered directly in a web chat UI that supports full markdown, including pipe tables (\`| col | col |\` style). Use a markdown table whenever the answer is naturally tabular or a side-by-side comparison - it will render as a real table, not raw text.`;

// runTurn mirrors agent.go's Answer(): loop until a non-tool_use
// stop_reason or MAX_TOOL_ITERATIONS is hit. Unlike Go, `messages` here is
// the FULL caller-supplied history (already including the new user turn)
// rather than being built from a bare question string - the caller
// (routes/api/ask/+server.ts) owns appending the user's new question,
// since the browser, not this server, is the authoritative transcript
// owner (see src/lib/stores/chat.svelte.ts - "container holds no state").
export async function runTurn(
	messages: Anthropic.MessageParam[]
): Promise<Anthropic.MessageParam[]> {
	const anthropic = getAnthropicClient();
	const [mcpSession, tools] = await Promise.all([getMcpSession(), getAnthropicTools()]);

	for (let i = 0; i < MAX_TOOL_ITERATIONS; i++) {
		const resp = await anthropic.messages.create({
			model: config.anthropicModel,
			max_tokens: ANSWER_MAX_TOKENS,
			system: SYSTEM_PROMPT,
			tools,
			messages
		});

		// No .toParam() in the TS SDK (unlike the Go SDK's resp.ToParam()) -
		// response.content is already the shape MessageParam.content wants.
		messages = [...messages, { role: resp.role, content: resp.content }];

		if (resp.stop_reason !== 'tool_use') {
			return messages;
		}

		const toolResults: Anthropic.ToolResultBlockParam[] = [];
		for (const block of resp.content) {
			if (block.type !== 'tool_use') continue;
			try {
				// block.input is already a parsed object here (unlike Go's
				// json.RawMessage) - the TS SDK decodes the whole response
				// body, so there's no separate json.Unmarshal step to port.
				const result = await mcpSession.callTool({
					name: block.name,
					arguments: block.input as Record<string, unknown>
				});
				toolResults.push({
					type: 'tool_result',
					tool_use_id: block.id,
					content: mcpResultText(result.content as unknown[]),
					is_error: (result.isError as boolean | undefined) ?? false
				});
			} catch (err) {
				toolResults.push({
					type: 'tool_result',
					tool_use_id: block.id,
					content: `tool call failed: ${err instanceof Error ? err.message : String(err)}`,
					is_error: true
				});
			}
		}
		messages = [...messages, { role: 'user', content: toolResults }];
	}

	throw new Error(`gave up after ${MAX_TOOL_ITERATIONS} tool-call rounds without a final answer`);
}

function mcpResultText(content: unknown[]): string {
	return content
		.filter((c): c is { type: 'text'; text: string } => (c as { type?: string }).type === 'text')
		.map((c) => c.text)
		.join('');
}
