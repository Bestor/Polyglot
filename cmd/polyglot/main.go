// Command polyglot runs the standalone Data API for val-analyzer: it
// embeds PocketBase for storage and exposes the small ANSI-SQL-oriented
// REST API described by openapi/polyglot.yaml (GET /query, POST /warm, GET
// /metadata, GET/POST /datasources) that any caller - including an MCP
// server - can use to answer statistical questions. It has no domain
// knowledge of its own: data sources are dataprovider.Provider
// implementations (see internal/providers/valorant) registered below and
// onboarded at runtime via POST /datasources.
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/config"
	"val-analyzer/internal/dataprovider"
	"val-analyzer/internal/logging"
	_ "val-analyzer/internal/migrations"
	"val-analyzer/internal/polyglot"
	"val-analyzer/internal/providers/valorant"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	logging.Init(cfg.Debug)

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

	providers := map[string]dataprovider.Provider{
		valorant.Type: valorant.Provider{},
		// register additional compiled-in provider types here.
	}
	reg := polyglot.NewRegistry(providers)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// Collections only exist once migrations have run during
		// bootstrap, which happens before this hook fires, so it's safe
		// here to: rehydrate previously-onboarded datasources,
		// auto-onboard valorant from env if configured, open the
		// read-only query connection, and register routes - in that
		// order.
		if err := reg.Rehydrate(se.App); err != nil {
			return err
		}
		if err := autoOnboardValorantFromEnv(se.App, reg); err != nil {
			return err
		}

		query, err := ai.NewReadOnlyExecutor(cfg.PBDataDir)
		if err != nil {
			return err
		}

		if err := polyglot.RegisterRoutes(se, reg, query, cfg.APIAuthToken); err != nil {
			return err
		}

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// autoOnboardValorantFromEnv preserves the "just works from a populated
// .env" experience: if HENRIK_API_KEY is set and valorant hasn't already
// been onboarded (e.g. rehydrated from a prior POST /datasources call),
// onboard it now through the exact same Registry.Onboard path POST
// /datasources uses - a convenience wrapper around the real mechanism,
// not a separate code path.
func autoOnboardValorantFromEnv(app core.App, reg *polyglot.Registry) error {
	apiKey := os.Getenv("HENRIK_API_KEY")
	if apiKey == "" {
		return nil
	}
	if _, ok := reg.Instance(valorant.Type); ok {
		return nil
	}

	cfg := map[string]any{"henrik_api_key": apiKey}
	if v := os.Getenv("HENRIK_BASE_URL"); v != "" {
		cfg["henrik_base_url"] = v
	}
	if v := os.Getenv("HENRIK_RATE_LIMIT_PER_MINUTE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid HENRIK_RATE_LIMIT_PER_MINUTE: %w", err)
		}
		cfg["rate_limit_per_minute"] = n
	}

	_, err := reg.Onboard(app, valorant.Type, cfg)
	return err
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
