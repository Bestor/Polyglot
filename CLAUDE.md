# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

val-analyzer is a Go backend that answers statistical questions about Valorant players (e.g.
headshot % comparisons across a season/act), with the AI reasoning happening in an external MCP
client rather than in-process. Riot/HenrikDev API calls are severely rate-limited, so aggressive
local caching (in an embedded PocketBase) is the core design constraint driving most of the
architecture below.

The stack is five binaries, split across two Data APIs: **valorantapi**, a standalone service that
owns all of Valorant's actual cached data (its own embedded PocketBase, its own ingest from
HenrikDev, `GET /query`, `GET /schema`, `POST`/`GET /warm`); **polyglot**, a generic, domain-agnostic
Data API host (`GET /query`, `GET /metadata`, `GET`/`POST /datasources`, `POST /datasources/
reconcile`, the `*/annotate` curation endpoints) that knows nothing about Valorant specifically - it
reaches valorantapi's data the same way it would reach any other onboarded datasource, over the
network; **mcpserver**, an MCP server generated from polyglot's OpenAPI spec that proxies each tool
call to a running polyglot instance over HTTP; **discordbot**, an MCP client that drives the actual
question-answering - it connects to mcpserver, hands its tools to Claude, and lets Claude's tool-use
loop decide which ones to call to answer a Discord `/ask` question; and **cachewarmer**, which
proactively calls `POST /warm` on valorantapi on a cadence for a configured list of players, so
caches stay fresh without a live question needing to trigger a sync.

polyglot itself has no built-in domain knowledge and no data of its own beyond its own onboarding/
catalog bookkeeping - every table a caller can query comes from a `dataprovider.Provider` connection
(`internal/dataprovider`) onboarded at runtime via `POST /datasources`: either a local SQLite file
(`internal/providers/sqlite`), or another service speaking polyglot's own small `GET /query`+
`GET /schema` contract over the network (`internal/providers/httpsql`). valorantapi is onboarded into
polyglot as an ordinary `http_sql` datasource named `valorant` - not a special case, and the same
mechanism can host other domains (e.g. a hypothetical NFL or chess.com standalone API) with zero new
code in polyglot itself. Every onboarded datasource's secrets (e.g. an `http_sql` datasource's bearer
token) live in a self-hosted OpenBao (open-source Vault fork) instance, never as plaintext in
polyglot's own persisted config - see `internal/vault`.

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

Some tests (e.g. `internal/polyglot/registry_test.go`, `internal/polyglot/metadata_test.go`,
`internal/polyglot/catalog_test.go`) spin up a real PocketBase test app via
`github.com/pocketbase/pocketbase/tests` and run the app migrations — no extra setup needed, but
they exercise the actual `internal/migrations` package (polyglot's own onboarding/catalog schema;
`internal/valorant/migrations` is the separate set valorantapi runs).

Run the stack via Docker Compose:

```sh
./run.sh --build   # or -b: docker compose up -d --build
./run.sh            # docker compose up -d, reusing existing images
docker logs -f val-analyzer-valorantapi
docker logs -f val-analyzer-openbao
docker logs -f val-analyzer-polyglot
docker logs -f val-analyzer-mcpserver
docker logs -f val-analyzer-discordbot
docker logs -f val-analyzer-cachewarmer
```

`run.sh` is a thin wrapper around `docker compose up` (`docker-compose.yml` at the repo root) - all
app configuration (ports, PocketBase data volumes, inter-service URLs) lives there, and all secrets/
values come from `.env` (see `.env.example`). Every service (`valorantapi`, `polyglot`, `mcpserver`,
`discordbot`, `cachewarmer`) builds from the one image (the same Dockerfile, containing all five
binaries), each overriding `entrypoint` to run its own binary; `openbao` runs the upstream
`openbao/openbao` image, not this repo's own image. `API_AUTH_TOKEN`, `HENRIK_API_KEY`, and the three
`VAULT_*` vars (`VAULT_ADDR`/`VAULT_TOKEN`/`VAULT_UNSEAL_KEY`) are required in `.env` - polyglot fails
fast at boot without the vault vars, matching `API_AUTH_TOKEN`'s existing pattern (see
`internal/vault` for why an unseal key, not just an address/token, is required: OpenBao's file
backend starts sealed on every restart, and polyglot auto-unseals it on boot rather than requiring a
manual step each time). `SUPERUSER_EMAIL`/`SUPERUSER_PASSWORD` (set together or not at all)
auto-provision each PocketBase-embedded binary's own admin UI superuser on boot.
`DISCORD_BOT_TOKEN`/`ANTHROPIC_API_KEY` are optional: the `discordbot` service is gated behind
Compose's `discordbot` profile (see `docker-compose.yml`), which only activates when `.env` sets
`COMPOSE_PROFILES=discordbot` - otherwise `docker compose up` starts everything else, since
`cmd/discordbot` fails fast without those values. `cachewarmer` is not profile-gated - it ships with
an empty `cmd/cachewarmer/players.txt` (a safe no-op) and starts by default; add Riot IDs to that
file (one `name#tag` per line) to have it actually warm anyone.

Manual smoke test against a running container:

```sh
./warm.sh   # POST /warm (sync_matches) against valorantapi directly; prints a 202 + job id to poll
```

Two separate migration sets, one per PocketBase-embedded binary: `internal/migrations` (polyglot's
own `datasources`/`tables`/`columns` onboarding/catalog bookkeeping, hand-authored Go files
registered via `m.Register` in `init()`) and `internal/valorant/migrations` (valorantapi's 15
Valorant domain tables, same authoring pattern, unchanged filenames/timestamps from before the
two-binary split - see the Architecture section's cutover note for why that matters). Both
auto-apply on every boot via `app.OnBootstrap()` + `RunAppMigrations()` in each binary's own
`main.go` — `migratecmd`'s own `migrate` subcommand does *not* run automatically on `serve`, so that
boot hook is the actual mechanism keeping a fresh container's schema up to date, not just an ops
convenience.

## Deployment (CI/CD)

`.github/workflows/deploy.yml` runs on every push to `main`: **build** (test, then build+push the
image to `ghcr.io/bestor/polyglot` under both `:latest` and `:sha-<commit>` tags) ->
**provision** (`terraform apply` in `terraform/`, guaranteeing a DigitalOcean droplet/volume/firewall
exist matching code) -> **deploy** (SSH in, `git pull` + `docker compose pull` + `docker compose up
-d`). All three jobs are required, in that order (`needs:`), and a `concurrency` group serializes
overlapping runs.

**`deploy`'s SSH script runs `cloud-init status --wait` before touching anything** -
`terraform apply` only waits for DigitalOcean's API to report the droplet "active," which says
nothing about whether `cloud-init.yaml.tftpl`'s `runcmd` (volume mount, git clone, docker
pull/up) has actually finished inside the VM, and SSH itself comes up well before that finishes.
On a freshly (re)created droplet this raced in practice - `deploy` connected and found
`/opt/val-analyzer` missing seconds before cloud-init would have created it. `cloud-init status
--wait` blocks until cloud-init reaches a real terminal state (and surfaces its actual exit code -
0 success, 1 crashed, 2 recoverable errors - so a broken `runcmd` step fails the job loudly instead
of limping on); on a droplet that's been up a while it returns near-instantly since cloud-init
caches its "done" state, so this doesn't slow down normal deploys.

**`terraform/`** is deliberately idempotent, not destroy-and-recreate-on-every-push: a plain push
does a fast `terraform apply` with no `-replace`, so `deploy`'s SSH step (`git pull` + `docker
compose pull/up`) is what actually ships each commit, not a droplet rebuild. State lives in a
DigitalOcean Space (`terraform/main.tf`'s `s3` backend block, pointed at a DO Spaces endpoint)
rather than Terraform Cloud - one provider to manage - with `use_lockfile = true` for real state
locking (no DynamoDB-equivalent needed). `volume.tf`'s `digitalocean_volume` is a separate,
`prevent_destroy`-protected resource specifically so the cache - the whole point of this project's
design - survives even a real droplet recreate; it now backs three subdirectories, not one:
`pb_data` (valorantapi's Valorant cache - same name/path from before the two-binary split, so a
cutover just retargets the existing volume to a different service, zero backfill),
`polyglot_metadata` (polyglot's own, separate, always-fresh onboarding/catalog database), and
`openbao_file` (OpenBao's file storage backend). `docker-compose.yml`'s services point their
respective volumes at those subdirectories in production via `PB_DATA_HOST_PATH`/
`PB_METADATA_HOST_PATH`/`VAULT_DATA_HOST_PATH` (all unset locally, so local dev is unaffected).

**OpenBao's one-time init/policy setup is fully automated, not a by-hand step** -
`terraform/cloud-init.yaml.tftpl` brings OpenBao up alone first (before the rest of the stack), then
runs `terraform/bootstrap-vault.sh`, which does `bao operator init` (with a single key share/
threshold, deliberately, so the resulting single key can become `VAULT_UNSEAL_KEY` and power fully
automatic unsealing on every restart via `internal/vault`), enables the KV v2 engine, and creates a
narrowly-scoped policy/token for polyglot (never the root token) - all against a fresh droplet with
no manual SSH step required. The resulting `VAULT_TOKEN`/`VAULT_UNSEAL_KEY` are written to
`/mnt/val-analyzer-data/vault-init.env`, a file on the same `prevent_destroy`-protected volume
OpenBao's own data lives on, then appended to `.env` - never GitHub secrets, never Terraform state/
variables. A droplet recreate reconnects to the same already-initialized vault via that same
persisted file (`bao operator init` errors if run twice against the same vault) instead of losing
access to whatever's stored in it. See `terraform/bootstrap-vault.sh` itself (a plain shell script)
and `terraform/README.md`'s "OpenBao setup and operations" section for the manual recovery path if
the automated bootstrap ever fails (e.g. an unusually slow boot outrunning its wait loop).

**Rebuilding the droplet is a deliberate, manual action, not something a plain push triggers** -
`digitalocean_droplet.app`'s `user_data` is schema-`ForceNew` in the DigitalOcean provider, but the
provider also has a `DiffSuppressFunc` on that field that suppresses content-only diffs in
practice, so a normal `terraform apply` does *not* reliably recreate the droplet just because
`cloud-init.yaml.tftpl`'s rendered content changed (confirmed the hard way - see git history around
the initial deploy). Use the `deploy.yml` workflow's `workflow_dispatch` trigger with
`recreate_droplet: true` to force it (`terraform apply -replace="digitalocean_droplet.app"`) -
needed after editing `cloud-init.yaml.tftpl` itself, or after rotating an app secret (`HENRIK_API_KEY`,
`DISCORD_BOT_TOKEN`, `VAULT_TOKEN`, etc. - these flow GitHub Actions secret -> `TF_VAR_*` -> templated
into `cloud-init.yaml.tftpl` -> the droplet's `.env`, written once at boot, so a rotated secret's new
value never reaches an already-running droplet without an explicit recreate).

**The deployed bot runs `claude-sonnet-5`, not `cmd/discordbot`'s own `claude-opus-4-8`
default** - `terraform/variables.tf`'s `anthropic_model` variable overrides it via the same
`TF_VAR_*` -> `cloud-init.yaml.tftpl` -> `.env` path as the secrets above, a deliberate production
choice (not the binary silently downgrading itself - see the "never silently downgraded" note in
the `cmd/discordbot` paragraph below, which is still true of the code's own default). Previously
`claude-haiku-4-5`; switched after Haiku underperformed on tool-use quality in practice. Change
`anthropic_model`'s default in `terraform/variables.tf` and force a recreate (`recreate_droplet:
true`) to switch it again.

**Every service in `docker-compose.yml` declares both `build: .` and `image:
ghcr.io/bestor/polyglot:latest`** (except `openbao`, which runs the upstream image) - local dev
(`run.sh --build`) builds and tags locally under that name with zero registry interaction; the
droplet's `docker compose pull` fetches the same tag from GHCR instead of building (small droplet, no
need to compile 5 Go binaries on it). The GHCR package is public (the image only ever contains
compiled binaries/`openapi/`/the secret-free `players.txt` - never a secret), so the pull needs no
registry auth on the droplet side.

Nothing besides SSH (22) is reachable on the droplet's public IP (`terraform/droplet.tf`'s
`digitalocean_firewall`) - every internal service (`valorantapi`, `openbao`, `polyglot`, `mcpserver`)
is only ever reached over the internal Compose network by another service, and `discordbot` itself
only makes outbound connections, so none of this stack needs to be internet-facing.

## Architecture

**`internal/dataprovider`** is the generic plugin contract polyglot hosts datasources through - zero
domain knowledge of any kind, not even Valorant's. A `Provider` (`provider.go`) is a compiled-in,
self-describing connector type: `Type()` (its stable slug), `ConfigSchema()` (what onboarding config
it needs - `Secret` fields become vault path references once persisted, never the literal value -
see `internal/polyglot/secrets.go`), and `New(ctx, config)` which performs real I/O immediately
(dialing/pinging/opening) and returns a live `Instance` (`Catalog`/`Query`/`Close`). There is
deliberately no PocketBase app handle or shared-collection-creation mechanism anywhere in this
interface (no `Bind`, no `TableSpec`) - neither real implementation ever needs one, since the one
domain that used to need that pattern (Valorant) now runs as its own standalone service
(`cmd/valorantapi`) with its own PocketBase, reached like any other connection. `RowSampler` is an
optional capability a `Provider`'s `Instance` may implement for future human-curation UX.

**`internal/providers/sqlite`** connects to an existing local SQLite file, read-only
(`file:...?mode=ro`, the same physical-incapable-of-writing guarantee `internal/ai`'s own executor
relies on). `Provider.New` enforces the MUST-FIX security guard `rejectOwnDataDir`: without it,
onboarding a SQLite file that resolves inside polyglot's own data directory would read back other
datasources' vault path references through `GET /query`. `Catalog` introspects `sqlite_master`/
`PRAGMA table_info`; `Query` delegates to `ai.RunReadOnlyQuery` (see below).

**`internal/providers/httpsql`** connects to another service speaking polyglot's own small
machine-to-machine contract: `GET /query` (returning `ai.QueryResult`'s columnar JSON shape directly,
not the row-object shape `GET /query` itself returns to external callers) and `GET /schema`
(returning a `dataprovider.TableCatalog` list). `cmd/valorantapi` is the first and, for now, only
real implementation of that contract - onboarded into polyglot as `name=valorant, type=http_sql`,
not a special case. `Provider.New` does a real round trip (calls `Catalog`) so a bad `base_url`/
`auth_token` fails onboarding immediately, not the first query.

**`internal/vault`** is a thin wrapper around OpenBao's KV v2 secrets engine (mount `secret`, path
convention `datasources/<name>/<field>`, keyed by datasource *name* so two datasources sharing one
provider type never collide). `New` also auto-unseals OpenBao on every boot if `unsealKey` is
non-empty, retrying for a few seconds since a container's listener may not be up yet right after
`docker compose up` - see its own doc comment for why storing the unseal key alongside other deploy
secrets (rather than requiring a manual `bao operator unseal` after every restart) is a deliberate,
explicit trade-off.

**`internal/polyglot`** is the generic host - it never imports a specific provider package.
- `registry.go`'s `Registry` is the onboarding engine, keyed by a user-chosen **name**, not
  `Provider.Type()` (multiple datasources can share one provider type - e.g. two onboarded SQLite
  files). `Onboard` validates config, calls `provider.New` (real I/O), persists the registration via
  `secrets.go`'s `PersistConfig` (every `Secret`-flagged field written to vault and replaced with a
  reference) into the `datasources` collection, activates the instance (closing any previous one
  under the same name), and kicks off an async catalog-reconcile job. `Rehydrate` replays every
  persisted registration on boot via `secrets.go`'s `ResolveConfig` - a plain-string secret (not yet
  ref-shaped) resolves as a pass-through and gets re-persisted as a vault ref on that same call, so
  no separate migration pass was ever needed to move the original plaintext `henrik_api_key` off of
  polyglot's persisted config (moot now anyway, since that credential moved to `cmd/valorantapi`'s
  own boot-time env var entirely - see below).
- `catalog.go`'s `reconcileCatalog` calls `Instance.Catalog(ctx)` for live ground truth and
  **upserts, never overwrites** the persisted `tables`/`columns` snapshot: inserts new tables/
  columns, deletes ones no longer present (cascade), refreshes a column's `type`, but never touches
  an existing row's hand-curated `description`/`query_guidance`. Runs via `internal/jobstore`,
  triggered automatically post-onboard and re-runnable via `POST /datasources/reconcile`.
- `datasources.go` — `GET /datasources` lists provider types + active datasources (by name);
  `POST /datasources` onboards; `POST /datasources/reconcile` re-runs catalog reconciliation;
  `POST /datasources/annotate`, `POST /tables/annotate`, `POST /columns/annotate` patch curated
  description/query_guidance fields (pointer fields, so omitted vs. explicitly-cleared are
  distinguishable).
- `query.go` — `GET /query` optionally routes via `?datasource=` to that instance's own `Query`;
  omitting it queries polyglot's own bookkeeping db. `reservedTablePattern` (blocking
  `datasources`/`tables`/`columns` by name) applies **unconditionally, on every path** - not just the
  default one - since nothing here assumes any datasource's connection is physically incapable of
  reaching polyglot's own tables.
- `metadata.go` — `GET /metadata` reads the persisted `datasources`/`tables`/`columns` snapshot
  directly (fast, local `FindRecords` calls), **never** a live `Instance.Catalog()` call - keeps
  this endpoint's latency independent of any one datasource's health/speed, even a slow or
  temporarily-unreachable network one.
- `routes.go` — also exposes `GET /jobs?id=` (renamed from the old `GET /warm?id=` - "warm" no
  longer describes what polyglot's own async jobs are for, since `/warm` itself doesn't live here
  anymore).

There is no `/warm` on polyglot itself, and no `Function`/`Instance.Functions()` concept in
`internal/dataprovider` - both were built for the old ingest-style "provider declares PocketBase
collections, polyglot creates and owns them" pattern, which left with Valorant. Ingest/warm triggers
now live entirely on `cmd/valorantapi`.

**`cmd/valorantapi`** (`internal/valorant`) is the standalone Valorant Data API: its own embedded
PocketBase (own `pb_data`, own migration set at `internal/valorant/migrations` - same filenames/
timestamps as before the two-binary split, so the droplet's existing cache carries over with zero
backfill on cutover), `ingest.Service` wired directly against it (no `dataprovider.Provider`/
`Registry` layer at all - there's exactly one domain here, so that plugin abstraction is pure
overhead). Exposes `GET /query` (`ai.NewReadOnlyExecutor` against its own db, raw `ai.QueryResult`
JSON - a machine-to-machine contract for `internal/providers/httpsql`, not row-objects for
mcpserver), `GET /schema` (introspects its own live collections), and `POST`/`GET /warm` (using
`internal/jobstore`, dispatching to `functions.go`'s `resolve_player`/`sync_matches`/`sync_seasons`/
`backfill_match_seasons` - local `Function`/`FunctionArg` types now, since this binary doesn't
implement `dataprovider.Provider`). `HENRIK_API_KEY` etc. are plain boot-time env vars here, not
vault-managed - deliberately: vault protection in this design is for secrets persisted into a
queryable PocketBase collection (`datasources.config`), and this credential is held only in this
process's memory, the same threat model `discordbot`'s/`cachewarmer`'s own plain secrets already
live under. It wraps three sub-packages, unchanged in behavior from before the split, just relocated
from `internal/providers/valorant`:
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

Curated table/column descriptions that used to live as hardcoded `Description` strings in
`internal/providers/valorant/tables.go` have no equivalent home in `cmd/valorantapi` (that binary
doesn't have a `tables`/`columns` catalog - only polyglot does) - they need to be re-entered by hand
via `POST /tables/annotate`/`POST /columns/annotate` against polyglot's catalog post-onboarding, a
one-time manual curation pass, not something any migration automates.

`internal/ratelimit` (a plain, provider-agnostic token bucket) stays at the top level - imported by
`cmd/valorantapi`, since rate limiting is specific to whichever upstream API a given ingest client
talks to.

`internal/jobstore` (generic in-memory async job tracking) and `internal/httpauth` (the static
bearer-token middleware) are both shared by `cmd/polyglot` and `cmd/valorantapi` - small, deliberate
extractions so the async-job-polling and auth-gating patterns aren't duplicated across the two
PocketBase-embedded binaries.

**`cmd/mcpserver`** (`internal/mcpserver`) exposes polyglot as MCP tools: `spec.go` parses
`openapi/polyglot.yaml` at load time into one `Operation` per spec operation (so tool schemas can
never drift from polyglot's actual REST contract), `server.go` registers one MCP tool per
`Operation` and proxies each call to a running polyglot instance over HTTP via `client.go`. It has
no data logic of its own — the MCP client (`cmd/discordbot`) is what actually reasons about a
question, deciding which tools to call and how to interpret the results. It only ever points at
polyglot's own spec, never valorantapi's - an AI conversation can still query Valorant data (via
`?datasource=valorant` routing through polyglot), but has lost the ability to trigger a fresh
Valorant sync mid-conversation now that `/warm` lives only on valorantapi; cachewarmer's proactive
hourly warming is the sole warming mechanism now (this was already the AI's secondary path even
before the split - `POST /warm`'s own description always said "only call this when a human
explicitly asks for a refresh").

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

**`cmd/cachewarmer`** (`internal/cachewarmer`) is the proactive counterpart to `/warm` being async:
since an AI tool-caller can't usefully call `warm` mid-question and get data back in time, caches
need to be kept warm some other way. `players.go`'s `ReadPlayerTags` reads a newline-delimited Riot
ID list (`cmd/cachewarmer/players.txt`, blank/`#`-comment lines skipped) fresh on every pass, so
edits take effect without a restart; `client.go` is a minimal bearer-token `POST /warm` client
(against valorantapi directly now - `POLYGLOT_URL`/`POLYGLOT_AUTH_TOKEN` env var names are unchanged
from before the split, just retargeted, and the request body no longer carries a `datasource` field
since valorantapi hosts exactly one domain); `run.go`'s `RunPass` fires one `Warm` call per player
*sequentially* (no benefit to concurrency - each call returns in milliseconds since the slow work
happens server-side and async) and never waits for a job to finish. `main.go` runs one pass
immediately on startup, then on a `time.Ticker` at `WARM_INTERVAL` (default `1h`), until `SIGTERM`.
Not gated behind a Compose profile like `discordbot` - it ships with an empty `players.txt` (a safe
no-op) and starts by default.

**`internal/ai`** is scoped to exactly one thing: read-only SQL execution.
`ai.NewReadOnlyExecutor` opens a *second* SQLite connection with `?mode=ro`, so it's physically
incapable of writing regardless of query text; the `SELECT`/`WITH`-prefix check on top of that is
defense in depth, not the primary guarantee. Both binaries use it for their own database
(`cmd/polyglot` for its bookkeeping db, `cmd/valorantapi` for its Valorant cache), and
`internal/providers/sqlite` opens its own separate `mode=ro` connection to whatever file it's
onboarded against - all three share the exact same row/cell/cumulative-byte truncation safety caps
via the extracted `ai.RunReadOnlyQuery(ctx, db, sqlText)`, never reimplemented per caller. Schema
description now lives entirely in `internal/polyglot`'s persisted `tables`/`columns` catalog, not
built from any hardcoded table list.
