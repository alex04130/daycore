// Command daycore is the Daycore v2 API server. It registers the AI wire-format
// and database drivers (blank imports), loads config, opens the selected store,
// migrates, builds the services, and serves the HTTP API with graceful shutdown.
package main

import (
	"context"
	"errors"
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
	"daycore/internal/domain"
	"daycore/internal/search"
	"daycore/internal/server"
	"daycore/internal/storage"
	"daycore/internal/version"

	// Register AI wire formats (self-register via init()).
	_ "daycore/internal/ai/formats/anthropic"
	_ "daycore/internal/ai/formats/ollama"
	_ "daycore/internal/ai/formats/openai"

	// Register web search engines (self-register via init()).
	_ "daycore/internal/websearch/duckduckgo"
	_ "daycore/internal/websearch/tavily"

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
	// ⚠️ CAPTURED FIRST, BEFORE ANYTHING READS .env — see startEnv.
	captureStartEnv()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	// `install` / `config` never reach config.Load: they exist to WRITE the
	// configuration, so requiring a valid one first would make the command
	// unusable in exactly the situation it is for.
	if handled, err := runSetup(os.Args[1:]); handled {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if cfg.GeneratedAdminToken {
		// Printed once, and it has to be printed: a generated credential nobody
		// is told about is the same as no credential, except harder to diagnose.
		// This replaces the old "unset ADMIN_TOKEN = open admin API outside
		// production", which was an unauthenticated configuration API on every
		// dev box and every self-hosted instance with no APP_ENV set.
		logger.Warn("no ADMIN_TOKEN configured; generated one for this process only — it changes on every restart",
			"admin_token", cfg.AdminToken)
	}
	if cfg.UsingDevSecrets {
		logger.Warn("using INSECURE development secrets — set APP_ENV=production with real JWT_SECRET/COOKIE_SECRET/ADMIN_TOKEN before exposing to any network")
	}

	// Storage. A failure here no longer takes the process with it — see
	// internal/server/degraded.go for why, and for what is served instead.
	//
	// The short version: dying means a crash loop under any supervisor, the only
	// evidence is a log line somebody has to know to look for, and the screen
	// that would let you FIX the configuration is served by the process that
	// keeps dying.
	var degradedReason string
	store, err := storage.Open(cfg.DBType, cfg.DBDSN)
	if err != nil {
		degradedReason = fmt.Sprintf("open db (%s): %v", cfg.DBType, err)
		store = nil
	}
	if store != nil {
		defer store.Close()
		if err := store.Migrate(context.Background()); err != nil {
			// Migrating is where a wrong DSN, a missing permission or an
			// incompatible schema usually surfaces — later than Open, and just
			// as fatal to every feature.
			degradedReason = fmt.Sprintf("migrate: %v", err)
		}
	}
	if degradedReason != "" {
		logger.Error("STORAGE UNAVAILABLE — starting in degraded mode: the admin console is served, everything else answers 503. Fix the configuration and restart.",
			"reason", degradedReason)
	}
	// Best-effort migrations (native FTS) log warnings instead of failing.
	if ws, ok := store.(interface{ MigrationWarnings() []string }); ok {
		for _, w := range ws.MigrationWarnings() {
			logger.Warn("migration", "warning", w)
		}
	}

	// A crash leaves async companion placeholders stuck in "pending" forever —
	// sweep them to "error" so clients stop polling.
	if store == nil {
	} else if n, err := store.Chats().FailPendingMessages(context.Background()); err != nil {
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

	// A nil repository means "embedded and disk only" — which is exactly right in
	// degraded mode: the prompts still render, they just cannot be overridden
	// from the console until storage is back.
	var promptRepo domain.PromptRepository
	if store != nil {
		promptRepo = store.Prompts()
	}
	prompts, err := ai.NewPromptService(promptRepo)
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

	// External capability sources (F2-A).
	//
	// One place assembles them because the pieces come from three layers that
	// only make sense together: config/providers.yaml declares identity and
	// wiring, the provider_overrides table carries what the console may change,
	// and the environment still supplies the credentials. Resolve merges them
	// per source; see internal/adapters and docs/ROADMAP.md for why the line
	// between file and table falls where it does.
	weatherSources, searchSources, srcWarnings := buildSources(cfg, store, logger)
	for _, w := range srcWarnings {
		// A source that will not build is a warning, not a fatal: the others
		// still work, and refusing to start over one misconfigured weather
		// adapter would take down the whole deployment for a sentence in a
		// morning brief.
		logger.Warn("provider unavailable", "err", w)
	}

	srv := server.New(server.Deps{
		Config:    cfg,
		Store:     store,
		Catalog:   catalog,
		Vision:    ai.NewOrchestrator(catalog),
		Prompts:   prompts,
		Hasher:    auth.NewHasher(cfg.Pepper),
		Tokens:    auth.NewTokenIssuer(cfg.JWTSecret, cfg.JWTTTL),
		Cookies:   auth.NewCookieSigner(cfg.CookieSecret),
		OAuth:     oauthMgr,
		Searcher:  search.NewMaterialSearcher(store),
		Weather:   weatherSources,
		WebSearch: searchSources,
		Blobs:     blobStore,
		Logger:    logger,
	})

	if degradedReason != "" {
		srv.EnterDegraded(degradedReason)
	}

	// Runtime configuration overrides (θ-F4b). Best-effort for the same reason
	// the locale overrides are: a database that cannot be read is a deployment
	// already in trouble, and refusing to start over it would take away the
	// console that could fix it. The environment seeds are still correct.
	// The override table into the live sources, same shape as ReloadSettings and
	// for the same reason: a row nothing reads is a console setting the process
	// ignores. Best-effort — the file's values are still correct.
	if err := srv.ReloadProviders(context.Background()); err != nil {
		logger.Warn("could not load provider overrides; using providers.yaml as-is", "err", err)
	}
	if err := srv.ReloadSettings(context.Background()); err != nil {
		logger.Warn("could not load runtime settings; using the environment seeds", "err", err)
	}

	// Pull the database layer of the message catalog. Best-effort: a translation
	// override that cannot be read is a degraded language, not a reason to refuse
	// to boot — the file and embedded layers still render every page.
	if store != nil {
		if err := srv.ReloadLocaleOverrides(context.Background()); err != nil {
			logger.Warn("could not load locale overrides; falling back to files and embedded", "err", err)
		}
	}

	// Everything below reads or writes rows on a timer. In degraded mode it would
	// be a loop logging the same failure every interval — noise on top of a
	// problem the operator already knows about, and every tick another chance to
	// nil-dereference. The console is what this process is for right now.
	if store == nil {
		return serveDegraded(logger, cfg, srv)
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
	srv.StartAILogRollUp()
	// Proposal lifecycle. Its first pass runs at boot, which is what settles the
	// decision cards orphaned by the process that died — they are already lapsed
	// (a decision card's TTL is the agent's own wait budget), so they need
	// expiring, not a special case. See proposal_sweep.go.
	srv.StartProposalSweep()

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

	fromPID, _ := restartRequested()
	ln, inherited, err := openListener(httpSrv.Addr, logger)
	if err != nil {
		return err
	}
	srv.SetListenerInherited(inherited)

	// The console's restart button. Installed here rather than in the server
	// package because the shutdown sequence below is main's, in main's order,
	// and a second copy of that order is a second copy that will drift.
	//
	// The function runs INSIDE the HTTP handler: it preflights and returns an
	// error the operator sees on the connection they asked from, then signals
	// this loop. Everything after that happens once the handler has returned,
	// because Shutdown waits for handlers and a handler waiting for Shutdown
	// would be waiting for itself.
	restart := make(chan struct{}, 1)
	srv.SetRestarter(func() error {
		if perr := preflightRestart(); perr != nil {
			return perr
		}
		select {
		case restart <- struct{}{}:
		default: // already restarting; a second click is not a second restart
		}
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening",
			"addr", httpSrv.Addr, "version", version.Full(), "db", cfg.DBType, "env", cfg.Env,
			"static", srv.StaticRoot(), "console", server.ConsoleBuilt(), "models", len(catalog.List()), "vision", catalog.HasVision(),
			"oauth", oauthMgr.Providers(), "restartedFrom", fromPID, "inheritedSocket", inherited)
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	// ⚠️ A restart and a signal run the SAME shutdown sequence. It is written
	// once, below the select, for exactly that reason: two copies would be two
	// orderings, and the ordering is the part with the sharp edges (see the
	// comments inside it).
	replacing := false
	var handover socketHandover
	select {
	case err := <-errCh:
		return err
	case <-restart:
		replacing = true
		logger.Warn("restarting on console request")
		// ⚠️ BEFORE the shutdown below, which closes the listener. Afterwards
		// there is nothing left to dup and the handover silently degrades to a
		// rebind. See finishRestart.
		handover = prepareHandover(ln, httpSrv.Addr, logger)
	case <-stop:
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = httpSrv.Shutdown(shutdownCtx)
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
	if replacing {
		// Only now, with the listener closed and the lease released. The store
		// is handed in rather than left to the deferred Close above, because on
		// Unix this call replaces the process image and that defer never runs.
		return finishRestart(logger, func() {
			if store != nil {
				if cerr := store.Close(); cerr != nil {
					logger.Warn("could not close the store before restarting", "err", cerr)
				}
			}
		}, handover)
	}
	return err
}

// finishRestart is the tail of both serve loops.
//
// # The socket handover is prepared by the CALLER, not here
//
// ⚠️ It has to be, and the ordering is the whole reason: preparing it means
// dup'ing the listening fd, and Shutdown closes the listener. A dup taken here
// — after the shutdown — fails with "use of closed network connection" and the
// restart silently degrades to a rebind. That is not a guess; it is what the
// first version did, and the e2e is what noticed.
//
// # It takes a release function, and that is not optional on Unix
//
// The Unix path is syscall.Exec, which replaces the process image — so NOTHING
// after it runs. Not a deferred store.Close, not a flush, nothing. A restart
// that relied on `defer store.Close()` would leak a server-side connection on
// Postgres, MySQL and Mongo every single time, and nothing would report it
// because from the database's side it looks like an ordinary client that went
// away.
//
// So the caller hands in what it owns, and it is released here, immediately
// before the point of no return.
//
// # The two platforms return differently, on purpose
//
//	Unix     replaceSelf does not return on success. The `return nil` below is
//	         unreachable there.
//	Windows  replaceSelf returns nil once the replacement is started, and this
//	         function's nil makes run() return, main exit 0, and the port free.
//
// ⚠️ A failed replacement does NOT exit. This process has already stopped
// serving so it is no use as it is — but it is still attached to whatever
// started it, and its logs are still where somebody is looking. Exiting would
// turn a visible failure into a machine that stopped answering for no stated
// reason.
func finishRestart(logger *slog.Logger, release func(), h socketHandover) error {
	// Resolved BEFORE releasing anything: if this fails there is nothing to
	// replace ourselves with, and we would rather find that out while the store
	// is still open than after.
	exe, err := os.Executable()
	if err != nil {
		logger.Error("cannot resolve this program's own path; not restarting", "err", err)
		return fmt.Errorf("%w: %v", errRestartNotPossible, err)
	}
	if release != nil {
		release()
	}
	if err := replaceSelf(logger, exe, h); err != nil {
		logger.Error("could not replace this process; it is NOT serving any more, "+
			"and is staying up only so this line is somewhere you can find it",
			"err", err, "hint", "start the binary again by hand")
		return fmt.Errorf("%w: %v", errRestartNotPossible, err)
	}
	return nil
}

// serveDegraded runs the HTTP server with no storage behind it.
//
// A separate, much shorter path rather than a set of `if store != nil` guards
// through the normal one: there is no worker, no lease, no sweep and no channel
// consumer here, and threading nil past all of them would leave five places
// where a future change reintroduces the crash. What this process does is
// answer, at the address the operator already knows, with what is wrong.
func serveDegraded(logger *slog.Logger, cfg *config.Config, srv *server.Server) error {
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	fromPID, _ := restartRequested()
	ln, inherited, err := openListener(httpSrv.Addr, logger)
	if err != nil {
		return err
	}
	srv.SetListenerInherited(inherited)

	// ⚠️ Degraded is the state where this button matters MOST: the operator has
	// just fixed DB_DSN in .env and a restart is the only way to pick it up.
	// Nothing here touches the store, which is what makes that possible.
	restart := make(chan struct{}, 1)
	srv.SetRestarter(func() error {
		if perr := preflightRestart(); perr != nil {
			return perr
		}
		select {
		case restart <- struct{}{}:
		default:
		}
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		logger.Warn("listening in DEGRADED mode — admin console only; every other route answers 503",
			"addr", httpSrv.Addr, "version", version.Full(), "db", cfg.DBType, "env", cfg.Env,
			"reason", srv.DegradedReason(), "restartedFrom", fromPID, "inheritedSocket", inherited)
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case <-restart:
		logger.Warn("restarting on console request (degraded)")
		// Before the shutdown, for the same reason as the normal path.
		handover := prepareHandover(ln, httpSrv.Addr, logger)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		// Nothing to release: a degraded process never opened a store, which is
		// the whole reason it is able to serve at all.
		return finishRestart(logger, nil, handover)
	case <-stop:
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
}
