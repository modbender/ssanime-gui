// Command ssanime is the ssanime-gui daemon: a long-running core that serves the
// embedded Svelte SPA over localhost and runs the download→encode→archive pipeline
// in the background. The browser tab is a window into the daemon, not the app.
//
// The process lifetime is owned by the system-tray icon (fyne.io/systray). Closing
// the browser tab leaves downloads and encodes running; the tray keeps the process
// alive. Quit from the tray menu (or Ctrl-C) triggers graceful shutdown.
//
// Build flags:
//
//	Standard (with console):
//	  go build ./cmd/ssanime
//
//	Windows GUI (no console window):
//	  go build -ldflags "-H=windowsgui -s -w" ./cmd/ssanime
//	  Logs are written to {DataDir}/ssanime.log in both modes.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/cli/browser"

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/animedb"
	"github.com/modbender/ssanime-gui/internal/anizip"
	"github.com/modbender/ssanime-gui/internal/binaries"
	"github.com/modbender/ssanime-gui/internal/config"
	"github.com/modbender/ssanime-gui/internal/discovery"
	"github.com/modbender/ssanime-gui/internal/doh"
	"github.com/modbender/ssanime-gui/internal/download"
	"github.com/modbender/ssanime-gui/internal/encode"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/extension"
	"github.com/modbender/ssanime-gui/internal/logging"
	"github.com/modbender/ssanime-gui/internal/metadata"
	"github.com/modbender/ssanime-gui/internal/poller"
	"github.com/modbender/ssanime-gui/internal/server"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
	"github.com/modbender/ssanime-gui/internal/tray/icon"
)

func main() {
	// Parse flags; avoid the flag package to prevent conflicts with systray.
	// --no-open: suppress auto-opening the browser tab.
	// --headless: run server + workers without the systray (used by the Tauri desktop shell).
	noOpen := false
	headless := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--no-open", "-no-open":
			noOpen = true
		case "--headless", "-headless":
			headless = true
			noOpen = true // headless implies no-open
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssanime: load config: %v\n", err)
		os.Exit(1)
	}

	logger, logBridge, logRing, logCloser, err := buildLogger(cfg.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssanime: open log file: %v\n", err)
		os.Exit(1)
	}
	if logCloser != nil {
		defer logCloser.Close()
	}

	logger.Info("starting", "app", config.AppName, "dataDir", cfg.DataDir, "port", cfg.Port, "headless", headless)

	// Single-instance preflight: before binding the port, probe any daemon already
	// holding it. An identical build -> reopen its UI and exit; a different build
	// (new version or rebuilt dev binary) -> ask it to shut down and take over.
	// On reopen/failure this exits the process; if it returns we proceed to bind.
	if !takeoverOrReopen(cfg, logger, noOpen, headless) {
		return
	}

	if headless {
		// Headless mode: used by the Tauri desktop shell. Run the full daemon
		// (server + workers) but without the systray. Block until SIGINT or
		// parent process kills us, then execute graceful LIFO shutdown.
		runHeadless(cfg, logger, logBridge, logRing)
		return
	}

	// daemonShutdown is populated by startDaemonFull inside onReady.
	// onExit (called on the systray event-loop thread when Quit fires) calls it.
	// The SIGINT handler in onReady also calls it and then triggers systray.Quit().
	var daemonShutdown func()

	// systray.Run MUST be called from the main goroutine.
	// onReady fires in a separate goroutine (per fyne.io/systray contract).
	systray.Run(
		onReady(cfg, logger, logBridge, logRing, noOpen, &daemonShutdown),
		onExit(&daemonShutdown, logger),
	)
}

// runHeadless starts the full daemon (store, hub, queues, HTTP server) without
// a systray. It blocks until SIGINT or SIGTERM, then runs graceful LIFO shutdown.
// This is the mode used when the Tauri desktop shell owns the process lifetime.
func runHeadless(cfg *config.Config, logger *slog.Logger, logBridge *logging.HubBridge, logRing *logging.Ring) {
	shutdown, _, _, shutdownRequested := startDaemon(cfg, logger, logBridge, logRing)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SIGINT (manual run / Ctrl-C).
	go func() {
		sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		<-sigCtx.Done()
		cancel()
	}()

	// A newer build taking over the port also triggers shutdown.
	go func() {
		<-shutdownRequested
		cancel()
	}()

	// Die-with-parent: when launched by the Tauri shell, our stdin is an anonymous
	// pipe held by that parent. If the parent exits — even via crash or force-kill —
	// the OS closes the pipe and the read returns EOF, so we shut down instead of
	// orphaning. Only armed when stdin is actually a pipe, so a standalone
	// `--headless` run (console / null stdin) still relies on SIGINT.
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeNamedPipe != 0 {
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			logger.Info("headless: parent pipe closed, shutting down")
			cancel()
		}()
	}

	<-ctx.Done()
	logger.Info("headless: shutting down")
	shutdown()
	logger.Info("headless: shutdown complete")
}

// onReady returns the systray onReady callback. It runs in its own goroutine.
// It starts the HTTP server + workers, optionally opens the browser, then waits
// for SIGINT. On signal it calls shutdown() and then systray.Quit() so main returns.
func onReady(cfg *config.Config, logger *slog.Logger, logBridge *logging.HubBridge, logRing *logging.Ring, noOpen bool, daemonShutdown *func()) func() {
	return func() {
		systray.SetIcon(icon.Data)
		systray.SetTitle(config.DisplayName)
		systray.SetTooltip(config.DisplayName + " — anime download & encode manager")

		mOpen := systray.AddMenuItem("Open UI", fmt.Sprintf("Open http://127.0.0.1:%d/", cfg.Port))
		mPause := systray.AddMenuItem("Pause all", "Pause download and encode queues")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "Graceful shutdown")

		// Start the daemon. This wires the store, hub, queues, and HTTP server;
		// it returns immediately (server runs in its own goroutine).
		var dlQ *download.Queue
		var encQ *encode.Queue
		var shutdownRequested <-chan struct{}
		*daemonShutdown, dlQ, encQ, shutdownRequested = startDaemon(cfg, logger, logBridge, logRing)

		// Auto-open the UI in the default browser once the listener is ready.
		if !noOpen {
			url := fmt.Sprintf("http://127.0.0.1:%d/", cfg.Port)
			waitForListener(cfg.Port, 5*time.Second)
			if err := browser.OpenURL(url); err != nil {
				logger.Warn("auto-open browser", "url", url, "err", err)
			}
		}

		// Pause state toggle for the "Pause all" menu item.
		paused := false

		// SIGINT handler: graceful shutdown and then quit the tray.
		sigCtx, sigStop := signal.NotifyContext(context.Background(), os.Interrupt)

		// Menu event loop.
		go func() {
			defer sigStop()
			for {
				select {
				case <-sigCtx.Done():
					return

				case <-shutdownRequested:
					// A newer build POSTed /api/shutdown to take over the port.
					// Cancel sigCtx so the outer wait runs the same graceful
					// shutdown + systray.Quit() as the SIGINT/Quit paths.
					logger.Info("tray: shutdown requested by takeover")
					sigStop()
					return

				case <-mOpen.ClickedCh:
					url := fmt.Sprintf("http://127.0.0.1:%d/", cfg.Port)
					if err := browser.OpenURL(url); err != nil {
						logger.Warn("tray: open browser", "err", err)
					}

				case <-mPause.ClickedCh:
					paused = !paused
					if paused {
						if dlQ != nil {
							dlQ.Pause()
						}
						if encQ != nil {
							encQ.Pause()
						}
						mPause.SetTitle("Resume all")
						mPause.SetTooltip("Resume download and encode queues")
					} else {
						if dlQ != nil {
							dlQ.Resume()
						}
						if encQ != nil {
							encQ.Resume()
						}
						mPause.SetTitle("Pause all")
						mPause.SetTooltip("Pause download and encode queues")
					}

				case <-mQuit.ClickedCh:
					logger.Info("tray: quit requested")
					sigStop()
					systray.Quit()
					return
				}
			}
		}()

		// Block until SIGINT or the Quit menu item closes sigCtx.
		<-sigCtx.Done()
		logger.Info("shutting down")
		if *daemonShutdown != nil {
			(*daemonShutdown)()
		}
		systray.Quit()
	}
}

// onExit returns the systray onExit callback. It runs on the event-loop thread
// as the tray tears down. It ensures daemonShutdown is called even if the Quit
// path didn't reach the onReady goroutine's shutdown sequence (e.g. OS-level kill).
func onExit(daemonShutdown *func(), logger *slog.Logger) func() {
	return func() {
		if *daemonShutdown != nil {
			(*daemonShutdown)()
		}
		logger.Info("tray exited")
	}
}

// startDaemon wires the full daemon (store, hub, queues, HTTP server).
// It starts the HTTP server in a goroutine and returns a shutdown func plus
// references to the download and encode queues (so the tray can pause them).
// The shutdown func tears everything down in LIFO order.
func startDaemon(cfg *config.Config, logger *slog.Logger, logBridge *logging.HubBridge, logRing *logging.Ring) (shutdown func(), dlQueue *download.Queue, encQueue *encode.Queue, shutdownRequested <-chan struct{}) {
	var cleanups []func()
	add := func(fn func()) { cleanups = append(cleanups, fn) }

	shutdown = func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	// shutdownRequested is closed (once) when a newer build POSTs /api/shutdown to
	// take over the port. The headless and tray loops select on it to run the same
	// graceful shutdown they run on SIGINT.
	shutdownCh := make(chan struct{})
	var shutdownReqOnce sync.Once
	onShutdownRequest := func() {
		shutdownReqOnce.Do(func() {
			logger.Info("shutdown requested by a newer instance taking over the port")
			close(shutdownCh)
		})
	}

	// --- Store ---
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 30*time.Second)
	st, err := store.Open(bootCtx, cfg)
	bootCancel()
	if err != nil {
		logger.Error("open store", "err", err)
		os.Exit(1)
	}
	add(func() {
		if err := st.Close(); err != nil {
			logger.Error("close store", "err", err)
		}
	})
	if set, err := st.Read().GetSettings(context.Background()); err == nil {
		logger.Info("store ready", "db", cfg.DBPath, "downloadRoot", set.DownloadRoot, "encodedRoot", set.EncodedRoot)
	} else {
		logger.Warn("store opened but settings unreadable", "err", err)
	}

	// --- Events hub ---
	hub := events.NewHub(logger)
	hub.Start()
	add(hub.Stop)

	// Wire the slog->hub bridge now that the hub is live, so subsequent log
	// records stream to the in-app Logs view. Pre-attach records went to file only.
	if logBridge != nil {
		logBridge.Attach(hub)
	}

	// --- Source registry + DoH ---
	resolver := doh.NewResolver("")
	registry := source.NewRegistry()

	// --- AniList client ---
	anilistClient := anilist.New()

	// --- Offline anime database (add-series search) ---
	// Serves the add-series title search from a local manami-project index so
	// search costs zero AniList calls. The dataset downloads over normal DNS
	// (GitHub raw) — a plain client, NOT the DoH-guarded one. Start kicks off a
	// background load/download; animeDB.Stop cancels it on shutdown.
	animeDB := animedb.New(cfg.DataDir,
		animedb.WithHTTPClient(&http.Client{Timeout: 5 * time.Minute}),
		animedb.WithLogger(logger),
	)
	animeDB.Start(context.Background())
	add(animeDB.Stop)

	// --- Extension manager ---
	// Extensions are third-party JS pulled from user-added repos; their fetch()
	// runs through a guarded DoH client that cannot reach loopback/private hosts.
	extManager := extension.NewManager(st, registry, resolver.GuardedHTTPClient(25*time.Second), cfg.DataDir, logger)
	if err := extManager.LoadAndRegisterAll(context.Background()); err != nil {
		logger.Warn("extension: load failed (non-fatal)", "err", err)
	}

	// --- Feed poller ---
	feedPoller := poller.New(st, registry, hub, logger)
	feedPoller.Start()
	add(feedPoller.Stop)

	// --- AniList metadata refresher ---
	// Keeps subscribed, still-airing series fresh so a RELEASING show auto-flips to
	// FINISHED and the poller stops chasing a completed series. Rate-limit-tolerant
	// by construction: on 429/network error it keeps the existing DB data.
	refresher := metadata.New(st, anilistClient, hub, logger)
	refresher.Start()
	add(refresher.Stop)

	// --- Discovery cache ---
	// Hourly-refreshed AniList feeds (trending/seasonal/popular/genre) powering the
	// discovery home. Serves a cache so page-loads cost zero AniList calls; on
	// 429/network error it keeps the prior slice and retries next tick.
	discoverySvc := discovery.New(anilistClient, logger,
		discovery.WithLogoFetcher(anizip.New()))
	discoverySvc.Start()
	add(discoverySvc.Stop)

	// --- Download queue ---
	dlWorkers := 2
	if set, err := st.Read().GetSettings(context.Background()); err == nil && set.ConcurrencyDownload > 0 {
		dlWorkers = int(set.ConcurrencyDownload)
	}
	dlRegistry := download.NewRegistry()
	add(func() {
		if err := dlRegistry.Close(); err != nil {
			logger.Error("close download backends", "err", err)
		}
	})
	dlQueue = download.New(st, dlRegistry, hub, download.Options{Workers: dlWorkers, Logger: logger})
	dlQueue.Start()
	add(dlQueue.Stop)

	// --- Binary provisioning ---
	binMgr := binaries.New(st, cfg.DataDir, logger)
	provCtx, provCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	logProg := func(recv, total int64) {
		if total > 0 {
			logger.Info("binaries: progress", "recv_mb", recv>>20, "total_mb", total>>20)
		}
	}
	if _, err := binMgr.EnsureFFmpeg(provCtx, logProg); err != nil {
		logger.Warn("binaries: ffmpeg unavailable (encode stage idle)", "err", err)
	}
	if _, err := binMgr.EnsureFFprobe(provCtx, logProg); err != nil {
		logger.Warn("binaries: ffprobe unavailable", "err", err)
	}
	if _, err := binMgr.EnsureYtDlp(provCtx, logProg); err != nil {
		logger.Warn("binaries: yt-dlp unavailable", "err", err)
	}
	provCancel()

	// --- Encode queue ---
	encWorkers := 1
	if set, err := st.Read().GetSettings(context.Background()); err == nil && set.ConcurrencyEncode > 0 {
		encWorkers = int(set.ConcurrencyEncode)
	}
	if enc, err := encode.NewFFmpegEncoder(binMgr.FFmpegPath); err != nil {
		logger.Warn("encode stage disabled: ffmpeg not found", "err", err)
	} else {
		encQueue = encode.New(st, enc, hub, encode.Options{
			Workers: encWorkers, DataDir: cfg.DataDir, Logger: logger,
		})
		encQueue.Start()
		add(encQueue.Stop)
	}

	// --- HTTP server ---
	srv := &http.Server{
		Addr: fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		Handler: server.New(st, hub, logger, server.Config{
			Registry:          registry,
			Anilist:           anilistClient,
			AnimeDB:           animeDB,
			ExtMgr:            extManager,
			Refresher:         refresher,
			Discovery:         discoverySvc,
			AnilistDetail:     anilistClient,
			Anizip:            anizip.New(),
			Logs:              logRing,
			OnShutdownRequest: onShutdownRequest,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server", "err", err)
		}
	}()
	add(func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		logger.Info("shutting down HTTP server")
		if err := srv.Shutdown(shutCtx); err != nil {
			logger.Error("shutdown", "err", err)
		}
	})

	return shutdown, dlQueue, encQueue, shutdownCh
}

// waitForListener polls tcp/127.0.0.1:<port> until it accepts or the timeout
// elapses. Prevents the browser from opening before the server is ready.
func waitForListener(port int, timeout time.Duration) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// buildLogger builds the daemon slog.Logger: a rotating {dataDir}/ssanime.log
// (10 MB/file, 30 backups, 30-day retention, gzip) mirrored to stdout, an
// in-memory *logging.Ring of recent formatted lines that backs GET /api/logs, and
// a HubBridge that streams Info+ records to the events hub for the in-app Logs
// view. The bridge's hub side stays inert until startDaemon calls bridge.Attach
// after the hub starts. The returned io.Closer flushes and closes the rotating sink.
func buildLogger(dataDir string) (*slog.Logger, *logging.HubBridge, *logging.Ring, io.Closer, error) {
	logger, bridge, ring, closer := logging.Build(dataDir)
	return logger, bridge, ring, closer, nil
}
