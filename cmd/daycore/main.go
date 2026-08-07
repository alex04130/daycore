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
	"daycore/internal/blob"
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

	// Register blob drivers (self-register via init()).
	_ "daycore/internal/blob/localfs"

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
	if n, err := store.Chats().FailPendingMessages(context.Background()); err != nil {
		// Was swallowed by `err == nil && n > 0`: a database that pings but whose
		// tables are broken failed here with no trace at all. Not fatal — the
		// sweep is a convenience, and clients time out on their own — but it must
		// be visible, because it is the first write of the process and therefore
		// the earliest evidence that the store is not actually usable.
		logger.Warn("could not sweep stale pending chat messages", "err", err)
	} else if n > 0 {
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
	// PROMPTS_DIR overlays edited templates onto the embedded defaults. Failing
	// here is right: the operator put a file there on purpose, and silently
	// running the embedded prompt instead would be a very quiet way to ignore it.
	if n, err := prompts.LoadDiskDefaults(cfg.PromptsDir); err != nil {
		return fmt.Errorf("load prompts from %s: %w", cfg.PromptsDir, err)
	} else if n > 0 {
		logger.Info("prompt templates overlaid from disk", "dir", cfg.PromptsDir, "files", n)
	}

	// L1 hard boundaries. Resolved here rather than lazily at the first chat so
	// that a broken boundaries file stops the process instead of degrading one
	// request, and so HardBoundaryReminder's panic path is unreachable once we
	// are serving.
	boundaries, err := ai.StdBoundaries()
	if err != nil {
		return fmt.Errorf("load hard boundaries: %w", err)
	}
	bload, err := boundaries.LoadDir(cfg.PromptsDir)
	if err != nil {
		return fmt.Errorf("load hard boundaries from %s: %w", cfg.PromptsDir, err)
	}
	if len(bload.Locales) > 0 {
		logger.Info("hard boundaries overlaid from disk", "path", bload.Path, "locales", bload.Locales)
	}
	if bload.Stale() {
		// Their copy wins — that is the point of the file. But a copy taken
		// before a release that added a rule silently withholds that rule, and
		// this line is the only place anyone would find out.
		logger.Warn("hard boundaries file predates this build; diff it against the shipped defaults",
			"path", bload.Path, "file_version", bload.Version, "shipped_version", bload.Embedded)
	}

	// File bus. Optional: a deployment without one simply has no feature that
	// needs bytes, and refusing to boot over that would be worse than saying so.
	blobStore, err := blob.Open(cfg.BlobStore, cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open blob store (%s): %w", cfg.BlobStore, err)
	}
	if blobStore != nil {
		logger.Info("file bus enabled", "store", blobStore.Name(), "dir", cfg.DataDir)
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
		Blobs:    blobStore,
		Logger:   logger,
	})

	// Pull the database layer of the message catalog. Best-effort: a translation
	// override that cannot be read is a degraded language, not a reason to refuse
	// to boot — the file and embedded layers still render every page.
	if err := srv.ReloadLocaleOverrides(context.Background()); err != nil {
		logger.Warn("could not load locale overrides; falling back to files and embedded", "err", err)
	}

	// Background cleanup for stale temp-context entries + expired binding tokens.
	srv.StartTempContextCleanup(0)
	srv.StartChannelBindingCleanup(0)
	// Uploads nobody sent. Without this every abandoned upload is permanent:
	// its row keeps the blob referenced, so no other sweep can reclaim it.
	srv.StartAttachmentCleanup(0)

	// Leader election. Starts before the Worker so that the first cron firing
	// already has an answer to "do I lead" — LeadsWorker is false until the
	// first renewal lands, and starting the other way round would let the first
	// minute run unguarded.
	//
	// ⚠️ This is what makes more than one instance safe, together with the
	// per-occurrence job rows. Read docs/ARCHITECTURE.md before changing the
	// order of any of this.
	srv.StartWorkerLease()
	srv.StartJobRunPrune()

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()

	// Channels are optional. A channel registry is only built when one is
	// configured; the Worker gets nil and sendToChannels then logs instead of
	// sending (that branch already existed).
	var registry *channels.Registry
	if cfg.OneBotWSURL != "" {
		registry = channels.NewRegistry(logger)
		registry.Register(onebot.New(onebot.Config{
			WSURL: cfg.OneBotWSURL,
			Token: cfg.OneBotToken,
			Log:   logger,
			ValidateBinding: func(ctx context.Context, externalID string) bool {
				b, err := store.ChannelBindings().GetByChannelAndExternal(ctx, "onebot", externalID)
				return err == nil && b != nil
			},
		}))
	}

	// The proactive Worker starts unconditionally.
	//
	// It used to start only when ONEBOT_WS_URL was set, which meant a user who
	// had not bound a QQ account got no morning brief, no evening review, no
	// scheduled auto-plan and no deadline sweep — the whole proactive half of the
	// product, switched off by an unrelated setting. Nothing the Worker does
	// requires a channel: everything it produces lands in the database and is
	// read by the app, and pushing to a channel is the optional last step.
	worker := server.NewWorker(srv, registry)
	srv.SetWorker(worker)
	worker.Start()
	defer worker.Stop()

	// Sessions are scheduled on their first request after boot rather than
	// enumerated here.
	//
	// The seed used to be "every verified channel binding", which is the same
	// coupling in a second place. Enumerating sessions instead would need a new
	// SessionRepository method across four backends, and it would schedule cron
	// entries for everyone who ever hit the API once. Lazy scheduling costs one
	// map lookup per request (ScheduleUser returns immediately when already
	// scheduled) and self-limits to people actually using the thing.
	//
	// ⚠️ The cost is a gap on the morning of a restart: someone who has not made
	// a request yet that day has no cron entry, so a 07:30 brief after an 07:00
	// restart does not fire for them. Closing that needs the session enumeration
	// this is avoiding, and it belongs with Lease-based election (batch ζ) —
	// otherwise every instance would schedule every user.
	srv.SetScheduleOnUse(func(sid string) {
		// The session's own zone, not the deployment default — see
		// internal/server/session_timezone.go.
		worker.ScheduleUser(sid, srv.SessionTimezone(context.Background(), sid))
	})

	if registry != nil {
		registry.StartAll(rootCtx)

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
	} else {
		logger.Info("no channel configured — proactive jobs still run, results stay in the app")
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
		// Stop the tick loops before anything else waits: they are the only
		// background work that re-enters the store on a schedule, and a tick that
		// fires after the store closes logs an error nobody can act on. It also
		// has to happen before any loop that OWNS something (a lease) exists —
		// that loop would otherwise reclaim what the process is giving up.
		srv.StopTicks()
		// Only now: the renewal loop is stopped, so nothing can take the lease
		// back after this. Releasing before StopTicks would let the next tick
		// re-acquire what this process is giving up, and the next instance would
		// wait a whole TTL for a leader that has already exited.
		srv.ReleaseWorkerLease()
		// Wait for detached background work (async turns, channel replies) so
		// in-flight results still get persisted; stale pending placeholders
		// from a hard deadline are swept to "error" on the next boot.
		if werr := srv.WaitBackground(shutdownCtx); werr != nil {
			logger.Warn("background agents did not finish before shutdown deadline", "err", werr)
		}
		return err
	}
}
