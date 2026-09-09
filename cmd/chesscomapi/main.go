// Command chesscomapi is the standalone Data API for chess.com player/
// game data: it embeds its own PocketBase (own pb_data, own migration set
// - see internal/chesscom/migrations), ingests from chess.com's own
// Published Data API via internal/chesscom/ingest, and exposes the same
// small HTTP surface every standalone Data API does - GET /query,
// GET /schema, GET /functions, POST/GET /warm, all served by the shared
// internal/dataapi handlers - consumed by core polyglot (cmd/polyglot) as
// one onboarded "http_sql" datasource, not by end users directly. Mirrors
// cmd/valorantapi's own structure exactly; there is exactly one domain
// here too, so ingest.Service is wired directly against this process's
// own PocketBase app, no dataprovider.Provider/Registry plugin layer.
package main

import (
	"context"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"val-analyzer/internal/ai"
	"val-analyzer/internal/chesscom"
	chesscomsource "val-analyzer/internal/chesscom/data_sources/chesscom"
	"val-analyzer/internal/chesscom/ingest"
	_ "val-analyzer/internal/chesscom/migrations"
	"val-analyzer/internal/chesscom/store"
	"val-analyzer/internal/dataapi"
	"val-analyzer/internal/httpauth"
	"val-analyzer/internal/jobstore"
	"val-analyzer/internal/logging"
	"val-analyzer/internal/ratelimit"
	"val-analyzer/internal/tracing"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	logging.Init(cfg.Debug)

	// Must run before anything below constructs an otelhttp-wrapped
	// http.Client - see internal/tracing's doc comment on why that
	// ordering is load-bearing, not stylistic.
	shutdownTracing, err := tracing.Init(context.Background(), "chesscomapi", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		log.Fatalf("tracing: %v", err)
	}

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: cfg.PBDataDir,
	})

	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		shutdownTracing(context.Background())
		return e.Next()
	})

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Dir:         "internal/chesscom/migrations",
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
		limiter := ratelimit.NewLimiter(cfg.ChesscomRatePerM, cfg.ChesscomRatePerM)
		source := chesscomsource.NewClient(cfg.ChesscomBaseURL, cfg.ChesscomUserAgent, limiter)
		ing := ingest.NewService(source,
			store.NewPlayerStore(se.App), store.NewRatingStore(se.App), store.NewMonthStore(se.App), store.NewGameStore(se.App))
		functions := chesscom.Functions(ing)

		query, err := ai.NewReadOnlyExecutor(cfg.PBDataDir)
		if err != nil {
			return err
		}

		jobs := jobstore.New()

		group := se.Router.Group("")
		group.BindFunc(httpauth.RequireToken(cfg.APIAuthToken))
		group.BindFunc(tracing.Middleware("chesscomapi"))
		group.GET("/query", dataapi.HandleQuery(query))
		group.GET("/schema", dataapi.HandleSchema(se.App))
		group.GET("/functions", dataapi.HandleFunctions(functions))
		group.POST("/warm", dataapi.HandleWarm("chesscom", functions, jobs))
		group.GET("/warm", dataapi.HandleWarmStatus(jobs))

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
