// Side-effect-only import: config's top-level required() calls throw
// eagerly at module evaluation, and hooks.server.ts is guaranteed to load
// once at process startup before any request is served - the SvelteKit
// equivalent of the Go binaries' mustEnv-in-main() fail-fast pattern
// (cmd/discordbot/main.go, cmd/mcpserver/main.go), since adapter-node has
// no single "main()" of its own to hook into directly.
import '$lib/server/config';
