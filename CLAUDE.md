# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

val-analyzer is a Go backend that answers statistical questions about Valorant players (e.g.
headshot % comparisons across a season/act), with the AI reasoning happening in an external MCP
client rather than in-process. Riot/HenrikDev API calls are severely rate-limited, so aggressive
local caching (in an embedded PocketBase) is the core design constraint driving most of the
architecture below.

The stack is two binaries: **polyglot**, a standalone Data API (`GET /query`, `POST /warm`,
`GET /metadata`) backed by PocketBase, and **mcpserver**, an MCP server generated from polyglot's
OpenAPI spec that proxies each tool call to a running polyglot instance over HTTP. An MCP client
(e.g. a future Discord bot) drives the actual question-answering by calling mcpserver's tools.

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
./go-docker.sh go test ./internal/polyglot/...                       # single package
./go-docker.sh go test ./internal/ai/ -run TestBuildSchema           # single test
```

Some tests (e.g. `internal/ai/schema_test.go`) spin up a real PocketBase test app via
`github.com/pocketbase/pocketbase/tests` and run the app migrations — no extra setup needed, but
they exercise the actual `internal/migrations` package.

Run the stack in Docker:

```sh
./run.sh --build   # or -b: docker build then run
./run.sh            # run the existing local image without rebuilding
docker logs -f val-analyzer-polyglot
docker logs -f val-analyzer-mcpserver
```

`run.sh` builds one image (containing both the `polyglot` and `mcpserver` binaries) and runs each
as its own container on a shared Docker network, with its own PocketBase data volume — PocketBase
isn't designed for two app instances to write to the same SQLite data dir concurrently. Requires a
populated `.env` (see `.env.example`) — `HENRIK_API_KEY` and `API_AUTH_TOKEN` are required.
`SUPERUSER_EMAIL`/`SUPERUSER_PASSWORD` (set together or not at all) auto-provision the PocketBase
admin UI superuser on boot.

Manual smoke test against a running container:

```sh
./warm.sh   # POST /warm (sync_matches) to pre-load a player's match history
```

Migrations live in `internal/migrations` as hand-authored Go files (`175000000N_name.go`,
registered via `m.Register` in `init()`). They auto-apply on every boot via
`app.OnBootstrap()` + `RunAppMigrations()` in `cmd/polyglot/main.go` — `migratecmd`'s own `migrate`
subcommand does *not* run automatically on `serve`, so that boot hook is the actual mechanism
keeping a fresh container's schema up to date, not just an ops convenience.

## Architecture

**`cmd/polyglot`** (`internal/polyglot`) is the Data API, per `openapi/polyglot.yaml`: every route
first passes a static bearer-token auth middleware (`internal/polyglot/auth.go`), then one of —
- `GET /query` (`query.go`) — runs one read-only SQL statement via the shared `ai.QueryFunc`.
- `POST /warm` (`warm.go`, `functions.go`) — invokes a named `Function` (`resolve_player`,
  `sync_matches`) by name/args. This is the only way match data gets synced from upstream; there is
  no AI-driven inline sync anymore. `coverageSufficient` skips the upstream call entirely when the
  cache already covers what's being asked, so repeat `sync_matches` calls don't blow the rate
  limit.
- `GET /metadata` (`metadata.go`) — describes the live schema (via `ai.BuildSchema`) plus the
  available functions, built once at route-registration time (post-migration).

**`cmd/mcpserver`** (`internal/mcpserver`) exposes polyglot as MCP tools: `spec.go` parses
`openapi/polyglot.yaml` at load time into one `Operation` per spec operation (so tool schemas can
never drift from polyglot's actual REST contract), `server.go` registers one MCP tool per
`Operation` and proxies each call to a running polyglot instance over HTTP via `client.go`. It has
no data logic of its own — an MCP client (e.g. a future Discord bot) is what actually reasons about
a question, deciding which tools to call and how to interpret the results.

**Data layer, bottom to top:**
- `internal/data_sources` defines a provider-agnostic `Source` interface plus shared DTOs;
  `internal/data_sources/henrik` is the only implementation today, against the unofficial HenrikDev
  API. Swapping to the official Riot API later means adding a subpackage, not reworking callers.
- `internal/ratelimit` is a single process-wide token bucket shared by the henrik client, since the
  upstream API caps requests per minute regardless of which internal caller triggered them. The
  henrik client separately retries individual 429s in place (honoring `Retry-After`), rather than
  failing a whole sync on one rate-limit hit.
- `internal/ingest` (`Service`) resolves Riot IDs into cached players and syncs match history into
  the store. `SyncPlayerMatches` always pages *backward* through history (the upstream API only
  exposes "most recent N + offset", no real date filter) until `MaxMatches` is hit, a `Since` bound
  is satisfied, or history is exhausted.
- `internal/store` is a typed wrapper layer over `core.Record` for each PocketBase collection
  (players, matches, seasons, ...). `store.Player.HistoryExhausted` is a one-way flag: once a
  backward sync has walked all the way to a player's true first match (an empty upstream page), any
  future request for arbitrarily old data is trivially satisfied without re-hitting upstream.
- PocketBase collections are defined by the migrations in `internal/migrations` — 15 tables
  covering players/seasons/reference data (maps, agents, weapons, tiers) down through
  matches → match_teams/match_players → rounds → round_player_stats/damage_events/kills →
  kill_assists/event_player_locations. PocketBase is embedded as a Go library, not run as a
  separate server.

**Schema/query abstraction** (`internal/ai`): `ai.BuildSchema` introspects the *live* PocketBase
collections (so structure/types can never drift from migrations) and merges in hand-authored
`tableNotes`/`columnNotes` maps in `schema.go` for semantic context an AI client needs — these are
annotations only, not the source of truth; a new column needs a migration, and ideally a note, but
works without one. `ai.NewReadOnlyExecutor` is the actual security boundary behind `GET /query`: it
opens a *second* SQLite connection to PocketBase's own `data.db` with `?mode=ro`, so it's
physically incapable of writing regardless of query text; the `SELECT`/`WITH`-prefix check on top
of that is defense in depth, not the primary guarantee. The package has no Valorant knowledge of
its own — it just supplies the shared `QueryFunc`/`QueryResult`/`UpdateArg` types and schema
introspection that `internal/polyglot` builds on.
