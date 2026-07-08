// Command server runs the Valorant stats analyzer backend: it embeds
// PocketBase for storage/admin UI, syncs and caches match data from the
// configured Valorant data source, and exposes an internal HTTP API that a
// future Discord bot can call to answer statistical questions.
package main

import (
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/api"
	"val-analyzer/internal/config"
	"val-analyzer/internal/data_sources/henrik"
	"val-analyzer/internal/ingest"
	_ "val-analyzer/internal/migrations"
	"val-analyzer/internal/ratelimit"
	"val-analyzer/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: cfg.PBDataDir,
	})

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Dir:         "internal/migrations",
		Automigrate: false,
	})

	// migratecmd only exposes a manual `migrate` subcommand; apply any
	// pending app migrations automatically on every boot too, so a plain
	// `docker run` (i.e. `serve`) is enough to get a fresh container to a
	// working schema without a separate ops step.
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if err := e.App.RunAppMigrations(); err != nil {
			return err
		}
		return upsertSuperuserFromEnv(e.App, cfg)
	})

	// Keep the container fully env-driven: `docker run` needs no extra
	// args, but `migrate`/`superuser` subcommands still work for ops.
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "serve", "--http=0.0.0.0:"+cfg.Port)
	}

	limiter := ratelimit.NewLimiter(cfg.RateLimitPerMinute, cfg.RateLimitBurst)
	source := henrik.NewClient(cfg.HenrikBaseURL, cfg.HenrikAPIKey, limiter)

	players := store.NewPlayerStore(app)
	matches := store.NewMatchStore(app)
	seasons := store.NewSeasonStore(app)
	ing := ingest.NewService(source, players, matches, seasons)

	var provider ai.Provider = ai.MockProvider{}
	if cfg.AIProvider == "claude" {
		provider = ai.NewClaudeProvider(cfg.AnthropicAPIKey)
	}

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// Collections only exist once migrations have run during bootstrap,
		// which happens before this hook fires, so it's safe to introspect
		// the schema and open the read-only query connection here.
		schema, err := ai.BuildSchema(se.App)
		if err != nil {
			return err
		}
		query, err := ai.NewReadOnlyExecutor(cfg.PBDataDir)
		if err != nil {
			return err
		}

		api.RegisterRoutes(se, ing, schema, query, provider, cfg.APIAuthToken)

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// upsertSuperuserFromEnv creates (or updates the password of) a superuser
// account from SUPERUSER_EMAIL/SUPERUSER_PASSWORD, mirroring PocketBase's
// own `superuser upsert` CLI command - so a container can be fully
// provisioned via env vars, with no manual first-run step through the
// admin UI.
func upsertSuperuserFromEnv(app core.App, cfg config.Config) error {
	if cfg.SuperuserEmail == "" {
		return nil
	}

	superusersCol, err := app.FindCachedCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return err
	}

	superuser, err := app.FindAuthRecordByEmail(superusersCol, cfg.SuperuserEmail)
	if err != nil {
		superuser = core.NewRecord(superusersCol)
	}

	superuser.SetEmail(cfg.SuperuserEmail)
	superuser.SetPassword(cfg.SuperuserPassword)

	return app.Save(superuser)
}
