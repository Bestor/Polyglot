// Command valorantapi is the standalone Data API for Valorant esports
// data: it embeds its own PocketBase (own pb_data, own migration set - see
// internal/valorant/migrations), ingests from HenrikDev via
// internal/valorant/ingest, and exposes a small HTTP surface - GET /query
// (read-only ANSI SQL), GET /schema (structure-only introspection), and
// POST/GET /warm (async sync triggers) - consumed by core polyglot
// (cmd/polyglot) as one onboarded "http_sql" datasource, not by end users
// directly. There is exactly one domain here, so unlike cmd/polyglot this
// binary has no dataprovider.Provider/Registry plugin layer - ingest.Service
// is wired directly against this process's own PocketBase app.
package main

import (
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/httpauth"
	"val-analyzer/internal/jobstore"
	"val-analyzer/internal/logging"
	"val-analyzer/internal/ratelimit"
	"val-analyzer/internal/valorant"
	"val-analyzer/internal/valorant/data_sources/henrik"
	"val-analyzer/internal/valorant/ingest"
	_ "val-analyzer/internal/valorant/migrations"
	"val-analyzer/internal/valorant/store"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	logging.Init(cfg.Debug)

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: cfg.PBDataDir,
	})

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Dir:         "internal/valorant/migrations",
		Automigrate: false,
	})

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return e.App.RunAppMigrations()
	})

	if len(os.Args) == 1 {
		os.Args = append(os.Args, "serve", "--http=0.0.0.0:"+cfg.Port)
	}

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		limiter := ratelimit.NewLimiter(cfg.HenrikRatePerM, cfg.HenrikRatePerM)
		source := henrik.NewClient(cfg.HenrikBaseURL, cfg.HenrikAPIKey, limiter)
		ing := ingest.NewService(source,
			store.NewPlayerStore(se.App), store.NewMatchStore(se.App), store.NewSeasonStore(se.App))
		functions := valorant.Functions(ing)

		query, err := ai.NewReadOnlyExecutor(cfg.PBDataDir)
		if err != nil {
			return err
		}

		jobs := jobstore.New()

		group := se.Router.Group("")
		group.BindFunc(httpauth.RequireToken(cfg.APIAuthToken))
		group.GET("/query", handleQuery(query))
		group.GET("/schema", handleSchema(se.App))
		group.POST("/warm", handleWarm(functions, jobs))
		group.GET("/warm", handleWarmStatus(jobs))

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
