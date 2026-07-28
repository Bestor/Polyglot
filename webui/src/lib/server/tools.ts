import type Anthropic from '@anthropic-ai/sdk';
import { getMcpSession } from './mcp';

let toolsPromise: Promise<Anthropic.Tool[]> | null = null;

// Ported from internal/discordbot/tools.go: MCP's Tool.inputSchema is
// already JSON Schema, so this is a field reshape, not a schema rewrite.
// Fetched once and cached, same as cmd/discordbot/main.go calling
// ListTools exactly once at startup rather than per question.
export function getAnthropicTools(): Promise<Anthropic.Tool[]> {
	if (!toolsPromise) {
		toolsPromise = getMcpSession()
			.then((client) => client.listTools())
			.then(({ tools }) =>
				tools.map((t) => ({
					name: t.name,
					description: t.description ?? '',
					input_schema: t.inputSchema as Anthropic.Tool['input_schema']
				}))
			);
	}
	return toolsPromise;
}
