// Command daycore is the Daycore v2 API server. It registers the AI wire-format
// and database drivers (blank imports), loads config, opens the selected store,
// migrates, builds the services, and serves the HTTP API with graceful shutdown.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"daycore/internal/ai"
	"daycore/internal/auth"
	"daycore/internal/channels"
	"daycore/internal/channels/onebot"
	"daycore/internal/config"
	"daycore/internal/search"
	"daycore/internal/server"
	"daycore/internal/storage"
	"daycore/internal/version"
	"daycore/internal/weather"

	// Register AI wire formats (self-register via init()).
	_ "daycore/internal/ai/formats/anthropic"
	_ "daycore/internal/ai/formats/ollama"
	_ "daycore/internal/ai/formats/openai"

	// Register weather providers (self-register via init()).
	_ "daycore/internal/weather/openmeteo"
	_ "daycore/internal/weather/openweathermap"
	_ "daycore/internal/weather/qweather"
	_ "daycore/internal/weather/wttrin"

	// Register database drivers (self-register via init()).
	_ "daycore/internal/storage/mongostore"
	_ "daycore/internal/storage/sqlstore"

	// Embed the IANA timezone database so ICS TZID parsing works even in the
	// static CGO-free binary running in a scratch container.
	_ "time/tzdata"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	// "install" subcommand: extract defaults, generate config, print admin token.
	if len(os.Args) > 1 && os.Args[1] == "install" {
		fs := flag.NewFlagSet("install", flag.ExitOnError)
		dir := fs.String("dir", "./data", "target directory for extracted files")
		force := fs.Bool("force", false, "overwrite existing files")
		fs.Parse(os.Args[2:])
		return runInstall(*dir, *force)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.UsingDevSecrets {
		logger.Warn("using INSECURE development secrets — set APP_ENV=production with real JWT_SECRET/COOKIE_SECRET/ADMIN_TOKEN before exposing to any network")
	}

	store, err := storage.Open(cfg.DBType, cfg.DBDSN)
	if err != nil {
		return fmt.Errorf("open db (%s): %w", cfg.DBType, err)
	}
	defer store.Close()

	if err := store.Migrate(context.Background()); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Best-effort migrations (native FTS) log warnings instead of failing.
	if ws, ok := store.(interface{ MigrationWarnings() []string }); ok {
		for _, w := range ws.MigrationWarnings() {
			logger.Warn("migration", "warning", w)
		}
	}

	// A crash leaves async companion placeholders stuck in "pending" forever —
	// sweep them to "error" so clients stop polling.
	if n, err := store.Chats().FailPendingMessages(context.Background()); err == nil && n > 0 {
		logger.Info("marked stale pending chat messages as error", "count", n)
	}

	catalog, err := ai.LoadCatalog(cfg.ModelsConfigPath, cfg.DefaultChatModel, cfg.DefaultVisionModel, cfg.DefaultPlannerModel)
	if err != nil {
		return fmt.Errorf("load model catalog: %w", err)
	}

	oauthMgr, err := auth.LoadOAuthProviders(cfg.OAuthConfigPath, cfg.PublicBaseURL)
	if err != nil {
		return fmt.Errorf("load oauth providers: %w", err)
	}

	prompts, err := ai.NewPromptService(store.Prompts())
	if err != nil {
		return fmt.Errorf("load prompts: %w", err)
	}

	// Weather provider (adapter): configured primary + wttr.in fallback + cache.
	weatherProvider := weather.New(weather.Options{
		Provider:          cfg.WeatherProvider,
		QWeatherKey:       cfg.QWeatherKey,
		OpenWeatherMapKey: cfg.OpenWeatherMapKey,
	})

	srv := server.New(server.Deps{
		Config:   cfg,
		Store:    store,
		Catalog:  catalog,
		Vision:   ai.NewOrchestrator(catalog),
		Prompts:  prompts,
		Hasher:   auth.NewHasher(cfg.Pepper),
		Tokens:   auth.NewTokenIssuer(cfg.JWTSecret, cfg.JWTTTL),
		Cookies:  auth.NewCookieSigner(cfg.CookieSecret),
		OAuth:    oauthMgr,
		Searcher: search.NewMaterialSearcher(store),
		Weather:  weatherProvider,
		Logger:   logger,
	})

	// Background cleanup for stale temp-context entries + expired binding tokens.
	srv.StartTempContextCleanup(0)
	srv.StartChannelBindingCleanup(0)

	// Channels + proactive worker (only when a channel is configured).
	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	if cfg.OneBotWSURL != "" {
		registry := channels.NewRegistry(logger)
		registry.Register(onebot.New(onebot.Config{
			WSURL: cfg.OneBotWSURL,
			Token: cfg.OneBotToken,
			Log:   logger,
			ValidateBinding: func(ctx context.Context, externalID string) bool {
				b, err := store.ChannelBindings().GetByChannelAndExternal(ctx, "onebot", externalID)
				return err == nil && b != nil
			},
		}))
		worker := server.NewWorker(srv, registry)
		srv.SetWorker(worker)

		registry.StartAll(rootCtx)
		worker.Start()
		defer worker.Stop()

		// Re-schedule proactive jobs for already-bound sessions.
		if bindings, err := store.ChannelBindings().ListAllVerified(rootCtx); err == nil {
			seen := map[string]bool{}
			for _, b := range bindings {
				if !seen[b.SessionID] {
					seen[b.SessionID] = true
					worker.ScheduleUser(b.SessionID, cfg.WorkerDefaultTZ)
				}
			}
		}

		// Consume inbound channel messages and drive the agent per message.
		// Per-message goroutines run through GoTracked so graceful shutdown
		// waits for in-flight replies.
		go func() {
			for msg := range registry.Inbound() {
				m := msg
				srv.GoTracked(func() { srv.HandleInbound(context.Background(), registry, m) })
			}
		}()
		logger.Info("channels enabled", "onebot", cfg.OneBotWSURL)
	}

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening",
			"addr", httpSrv.Addr, "version", version.Full(), "db", cfg.DBType, "env", cfg.Env,
			"static", cfg.StaticDir, "models", len(catalog.List()), "vision", catalog.HasVision(),
			"oauth", oauthMgr.Providers())
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case <-stop:
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := httpSrv.Shutdown(shutdownCtx)
		// Wait for detached background work (async turns, channel replies) so
		// in-flight results still get persisted; stale pending placeholders
		// from a hard deadline are swept to "error" on the next boot.
		if werr := srv.WaitBackground(shutdownCtx); werr != nil {
			logger.Warn("background agents did not finish before shutdown deadline", "err", werr)
		}
		return err
	}
}
