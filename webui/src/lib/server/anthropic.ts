import Anthropic from '@anthropic-ai/sdk';
import { config } from './config';

let client: Anthropic | null = null;

// Lazy, not a top-level const: config.anthropicApiKey is allowed to be
// empty (see config.ts's own comment on why Ask's config is optional at
// boot) - constructing eagerly would either throw a raw SDK error at
// import time (taking down routes that don't even use Anthropic) or
// silently construct a client that fails opaquely on first real call.
// This throws one clear, specific error, only when something actually
// tries to use it.
export function getAnthropicClient(): Anthropic {
	if (!config.anthropicApiKey) {
		throw new Error('ANTHROPIC_API_KEY is not configured - the Ask feature is unavailable');
	}
	if (!client) client = new Anthropic({ apiKey: config.anthropicApiKey });
	return client;
}
