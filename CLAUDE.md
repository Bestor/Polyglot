# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

val-analyzer is a Go backend that answers statistical questions about Valorant players (e.g.
headshot % comparisons across a season/act), with the AI reasoning happening in an external MCP
client rather than in-process. Riot/HenrikDev API calls are severely rate-limited, so aggressive
local caching (in an embedded PocketBase) is the core design constraint driving most of the
architecture below.

The stack is three binaries: **polyglot**, a standalone Data API (`GET /query`, `POST /warm`,
`GET /metadata`, `GET`/`POST /datasources`) backed by PocketBase; **mcpserver**, an MCP server
generated from polyglot's OpenAPI spec that proxies each tool call to a running polyglot instance
over HTTP; and **discordbot**, an MCP client that drives the actual question-answering - it
connects to mcpserver, hands its tools to Claude, and lets Claude's tool-use loop decide which
ones to call to answer a Discord `/ask` question.

polyglot itself has no built-in domain knowledge - every table and `/warm` function comes from a
`DataProvider` (`internal/dataprovider`) onboarded at runtime via `POST /datasources`. Valorant is
the one `DataProvider` that ships with this repo (`internal/providers/valorant`), but the same API
can host other domains (e.g. NFL, chess.com) by adding another compiled-in provider package.

## Go toolchain: Docker only, never local

Do not install or rely on a local Go toolchain. Every `go` command (build, test, vet, mod tidy,
gofmt, etc.) must run inside a Docker container, even if a local Go is present on the host. Use the
`go-docker.sh` wrapper at the repo root instead of inventing a new invocation:

```sh
./go-docker.sh go build ./...
./go-docker.sh go vet ./...
./go-docker.sh go mod tidy
./go-docker.sh gofmt -l .          # gofmt -w . to fix
```

It runs `golang:1.26-alpine` (match go.mod's version), bind-mounts the repo at `/app`, and uses
named Docker volumes for the module/build cache so repeated commands don't redownload deps.

## Commands

Tests (also via `go-docker.sh`):

```sh
./go-docker.sh go test ./...
./go-docker.sh go test ./internal/polyglot/...                          # single package
./go-docker.sh go test ./internal/polyglot/ -run TestRegistryOnboard    # single test
```

Some tests (e.g. `internal/polyglot/registry_test.go`, `internal/polyglot/metadata_test.go`) spin
up a real PocketBase test app via `github.com/pocketbase/pocketbase/tests` and run the app
migrations — no extra setup needed, but they exercise the actual `internal/migrations` package.

Run the stack in Docker:

```sh
./run.sh --build   # or -b: docker build then run
./run.sh            # run the existing local image without rebuilding
docker logs -f val-analyzer-polyglot
docker logs -f val-analyzer-mcpserver
docker logs -f val-analyzer-discordbot
```

`run.sh` builds one image (containing the `polyglot`, `mcpserver`, and `discordbot` binaries) and
runs each as its own container on a shared Docker network, with polyglot/mcpserver each getting
their own PocketBase data volume — PocketBase isn't designed for two app instances to write to the
same SQLite data dir concurrently. Requires a populated `.env` (see `.env.example`) — only
`API_AUTH_TOKEN` is required. `HENRIK_API_KEY` is optional: if set, polyglot auto-onboards a
`valorant` datasource on boot; if unset, polyglot still boots fine with zero datasources onboarded
(onboard any datasource, including `valorant`, later via `POST /datasources`).
`SUPERUSER_EMAIL`/`SUPERUSER_PASSWORD` (set together or not at all) auto-provision the PocketBase
admin UI superuser on boot. `DISCORD_BOT_TOKEN` is also optional: `run.sh` skips the
`val-analyzer-discordbot` container entirely if it's unset, since `cmd/discordbot` fails fast
without it.

Manual smoke test against a running container:

```sh
./warm.sh   # POST /warm (sync_matches on the valorant datasource) to pre-load a player's match history
```

Migrations live in `internal/migrations` as hand-authored Go files (`175000000N_name.go`,
registered via `m.Register` in `init()`). They auto-apply on every boot via
`app.OnBootstrap()` + `RunAppMigrations()` in `cmd/polyglot/main.go` — `migratecmd`'s own `migrate`
subcommand does *not* run automatically on `serve`, so that boot hook is the actual mechanism
keeping a fresh container's schema up to date, not just an ops convenience. The last migration
(`1750000018_datasources.go`) creates polyglot's own `datasources` bookkeeping collection (see
below); Valorant's 15 domain tables are the rest, created once and never touched by the dynamic
onboarding path described next.

## Architecture

**`internal/dataprovider`** is the generic plugin contract polyglot hosts data sources through -
zero Valorant (or any domain) knowledge. A `Provider` (`provider.go`) is a compiled-in, self-describing
type: `Type()` (its stable slug/datasource id), `ConfigSchema()` (what onboarding config it needs,
e.g. an API key - `Secret` fields are masked in API responses), `Tables()` (the PocketBase
collections it needs, as `TableSpec`/`FieldSpec` - `table.go`'s `FieldSpec.ToCoreField` converts
these into real `core.Field`s for dynamic collection creation), and `New(config)` which builds an
`Instance` bound to a live PocketBase app via `Instance.Bind`, exposing its `POST /warm` actions via
`Instance.Functions()`.

**`internal/polyglot`** is the generic host - it never imports a specific provider package.
- `registry.go`'s `Registry` is the onboarding engine: `Onboard` validates config, ensures every
  `TableSpec` exists as a collection (`ensureTable` creates any that are missing via
  `core.NewBaseCollection`/`Fields.Add`/`app.Save` - exactly the pattern PocketBase's own dashboard
  API uses internally; a table name already owned by a different active datasource is a `409`, not
  a silent collision), binds the instance, and persists the registration (type + config, including
  secrets) into the `datasources` collection so it survives a restart. `Rehydrate` replays every
  persisted registration on boot.
- `datasources.go` — `GET /datasources` lists every compiled-in provider type (with its config
  schema, for onboarding) and every active datasource (its tables/functions); `POST /datasources`
  onboards or idempotently reconfigures one.
- `query.go` — `GET /query` runs one read-only SQL statement via the shared `ai.QueryFunc`, fully
  provider-agnostic (all datasources share the one PocketBase `data.db`, so a query can join across
  datasources if their table names don't collide). It rejects any statement mentioning `datasources`
  by name (`reservedTablePattern`) so onboarded secrets can never be read back out through this
  endpoint, even though `datasources` was never advertised via `/metadata` either.
- `warm.go` — `POST /warm` is datasource-scoped: the request names both a `datasource` and a
  `function`, dispatched to that active instance's `Functions()`.
- `metadata.go` — `GET /metadata` merges every active instance's tables/functions into one response,
  each tagged with its owning `datasource`. Built fresh per request (not cached at boot), since
  `POST /datasources` can change what's active at runtime.
- `auth.go` — a static bearer-token middleware, unchanged, guarding every route above.

**`internal/providers/valorant`** is the Valorant `DataProvider`: `provider.go` implements
`Provider`/`Instance` (config: `henrik_api_key`, `henrik_base_url`, `rate_limit_per_minute`),
`functions.go` has the `resolve_player`/`sync_matches` warm actions, `tables.go` mirrors Valorant's
15 migrated tables as `TableSpec`s (used only for `/metadata` descriptions and onboarding's
collision check, since these collections already exist via migrations - onboarding `valorant` is a
schema no-op). It wraps three sub-packages, unchanged in behavior from before this was a plugin,
just relocated:
- `data_sources` (+ `data_sources/henrik`) — a provider-agnostic `Source` interface plus Valorant
  DTOs; `henrik` is the only implementation today, against the unofficial HenrikDev API.
- `ingest` (`Service`) — resolves Riot IDs into cached players and syncs match history.
  `SyncPlayerMatches` always pages *backward* through history (the upstream API only exposes "most
  recent N + offset", no real date filter) until `MaxMatches` is hit, a `Since` bound is satisfied,
  or history is exhausted.
- `store` — a typed wrapper layer over `core.Record` for each PocketBase collection (players,
  matches, seasons, ...). `store.Player.HistoryExhausted` is a one-way flag: once a backward sync
  has walked all the way to a player's true first match (an empty upstream page), any future
  request for arbitrarily old data is trivially satisfied without re-hitting upstream.

`internal/ratelimit` (a plain, provider-agnostic token bucket) stays at the top level - it's
imported by the valorant provider, not by `internal/polyglot` itself, since rate limiting is a
per-provider concern (the upstream API each provider talks to has its own limits).

**`cmd/mcpserver`** (`internal/mcpserver`) exposes polyglot as MCP tools: `spec.go` parses
`openapi/polyglot.yaml` at load time into one `Operation` per spec operation (so tool schemas can
never drift from polyglot's actual REST contract), `server.go` registers one MCP tool per
`Operation` and proxies each call to a running polyglot instance over HTTP via `client.go`. It has
no data logic of its own — the MCP client (`cmd/discordbot`) is what actually reasons about a
question, deciding which tools to call and how to interpret the results. It needed zero code
changes for the `DataProvider` rework - it's entirely spec-driven, so the two new `/datasources`
operations and the `datasource` field additions just show up as new/changed tool schemas.

**`cmd/discordbot`** (`internal/discordbot`) is the MCP client that does the actual
question-answering: `mcpclient.go` connects to a running `mcpserver` over Streamable HTTP
(`mcp.NewClient` + `mcp.StreamableClientTransport`, the go-sdk's real network client - not the
in-memory transport `internal/mcpserver/server_test.go` uses for testing), `tools.go` converts
every MCP tool it lists into an Anthropic tool definition (MCP's `InputSchema` is already JSON
Schema, so this is a field reshape, not a schema rewrite), and `agent.go` runs the manual
tool-use loop: send the question + tool defs to Claude, execute any `tool_use` blocks via
`mcpSession.CallTool`, feed `tool_result`s back, repeat (capped at `maxToolIterations`) until
Claude returns a final answer. `bot.go` wires this to a single Discord slash command, `/ask`,
deferring the interaction response since the tool-use loop routinely takes longer than Discord's
~3s initial-response window. Defaults to `claude-opus-4-8` (`ANTHROPIC_MODEL` overrides it) -
never silently downgraded to a cheaper model.

**`internal/ai`** is now scoped to exactly one thing: read-only SQL execution.
`ai.NewReadOnlyExecutor` is the actual security boundary behind `GET /query`: it opens a *second*
SQLite connection to PocketBase's own `data.db` with `?mode=ro`, so it's physically incapable of
writing regardless of query text; the `SELECT`/`WITH`-prefix check on top of that is defense in
depth, not the primary guarantee. Schema description (what used to be `ai.BuildSchema`) now lives in
`internal/polyglot/metadata.go`, built from whatever `TableSpec`s each active `DataProvider`
declares rather than a hardcoded table list.
