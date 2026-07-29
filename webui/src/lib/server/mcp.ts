import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import { config } from './config';

let sessionPromise: Promise<Client> | null = null;

async function connect(): Promise<Client> {
	if (!config.mcpUrl) {
		throw new Error('MCP_URL is not configured - the Ask feature is unavailable');
	}
	const client = new Client({ name: 'polyglot-webui', version: '0.1.0' });
	const transport = new StreamableHTTPClientTransport(new URL(config.mcpUrl));
	await client.connect(transport);
	return client;
}

// Lazy singleton: the first request that needs it pays the connect cost,
// every later request (and tools.ts's own singleton below) reuses the same
// session for the whole container's lifetime - mcpserver is stateless
// (StreamableHTTPOptions{Stateless: true} in cmd/mcpserver/main.go), so
// there's no per-conversation session state to worry about, only the
// underlying connection. Mirrors internal/discordbot/mcpclient.go's own
// "construct once, reuse for the process's lifetime" precedent, adapted
// from "construct once in main()" to "construct once, cached across
// per-request handler invocations."
export function getMcpSession(): Promise<Client> {
	if (!sessionPromise) sessionPromise = connect();
	return sessionPromise;
}
