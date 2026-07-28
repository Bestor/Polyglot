import { env } from '$env/dynamic/private';
import { building } from '$app/environment';

// $env/dynamic/private, never $env/static/private: the latter inlines
// values at `vite build` time, which is wrong here - this repo's own
// CI/CD builds the image once with no secrets present, then configures it
// entirely via env vars injected at container start (see .github/workflows/
// deploy.yml + docker-compose.yml's env_file: .env convention). Only
// $env/dynamic/private reads process.env at request time, which is the
// only option compatible with that build/deploy split.
//
// The `building` guard matters for a different reason: SvelteKit's own
// `vite build` runs a postbuild "analyse" step that imports the whole
// server module graph (including this file, via hooks.server.ts's
// side-effect import) with no env vars present at all - without this
// guard, required() throwing eagerly at module-evaluation time breaks the
// build itself, not just a misconfigured runtime boot.
function required(name: string): string {
	const v = env[name];
	if (!v && !building) throw new Error(`${name} is required`);
	return v ?? '';
}

// polyglotUrl/polyglotAuthToken are required at boot: the recent-queries
// page (this app's core, always-on feature) can't work without them.
// mcpUrl/anthropicApiKey are deliberately NOT required here, even though
// the Ask feature can't work without them either - unlike the
// recent-queries page, Ask is one optional feature among two, and failing
// the whole process at boot over a missing key would take the
// recent-queries page down too, breaking this repo's established "core
// functionality works with minimal config" pattern (cachewarmer ships
// safe-by-default, discordbot is profile-gated rather than crashing
// unconfigured). See lib/server/mcp.ts/anthropic.ts, which validate these
// lazily, only when the Ask feature is actually invoked.
export const config = {
	polyglotUrl: required('POLYGLOT_URL'),
	polyglotAuthToken: required('POLYGLOT_AUTH_TOKEN'),
	mcpUrl: env.MCP_URL ?? '',
	anthropicApiKey: env.ANTHROPIC_API_KEY ?? '',
	// Same default and same never-silently-downgraded stance as
	// cmd/discordbot's own ANTHROPIC_MODEL handling.
	anthropicModel: env.ANTHROPIC_MODEL || 'claude-opus-4-8'
};
