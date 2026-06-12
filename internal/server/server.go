// Package server is the HTTP layer: a chi router exposing the REST API and the
// SSE event stream under /api, plus the embedded Svelte SPA served with an HTML5
// (client-side routing) fallback. All endpoints return the uniform
// Response[T]{Data,Error} envelope. The Handler holds the app's shared
// dependencies (store, events hub, logger); routes are methods on it.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/animedb"
	"github.com/modbender/ssanime-gui/internal/discovery"
	"github.com/modbender/ssanime-gui/internal/events"
	"github.com/modbender/ssanime-gui/internal/extension"
	"github.com/modbender/ssanime-gui/internal/source"
	"github.com/modbender/ssanime-gui/internal/store"
)

// MetadataRefresher refreshes one series' AniList metadata on demand. The
// background *metadata.Refresher satisfies it; kept narrow so the server doesn't
// import the whole package surface.
type MetadataRefresher interface {
	RefreshSeries(ctx context.Context, id int64) (store.Series, error)
}

// DiscoveryProvider serves the cached discovery feeds for the home page. The
// background *discovery.Service satisfies it; readers never trigger a live fetch.
type DiscoveryProvider interface {
	Snapshot() map[discovery.FeedKey][]anilist.Media
}

// Handler carries the shared dependencies every route needs and registers the
// route table.
type Handler struct {
	store         *store.Store
	hub           *events.Hub
	logger        *slog.Logger
	registry      *source.Registry
	anilist       *anilist.Client
	animedb       *animedb.DB
	extMgr        *extension.Manager
	refresher     MetadataRefresher
	discovery     DiscoveryProvider
	anilistDetail AnilistDetailFetcher
	anizip        AnizipFetcher
	logs          *RingBuffer

	// onShutdownRequest is fired (once, asynchronously) when a newer build POSTs
	// /api/shutdown to take over the port. nil in tests / when unset.
	onShutdownRequest func()
	shutdownOnce      sync.Once
}

// Config holds optional dependencies for server.New.
type Config struct {
	Registry      *source.Registry
	Anilist       *anilist.Client
	AnimeDB       *animedb.DB
	ExtMgr        *extension.Manager
	Refresher     MetadataRefresher
	Discovery     DiscoveryProvider
	AnilistDetail AnilistDetailFetcher
	Anizip        AnizipFetcher

	// OnShutdownRequest is invoked once when a newer build POSTs /api/shutdown to
	// take over the port. The daemon wires its graceful-shutdown trigger here; the
	// handler responds 204 first, then calls this in a goroutine.
	OnShutdownRequest func()
}

// New builds the Handler and returns the fully wired http.Handler: REST + SSE
// under /api and the embedded SPA (HTML5 fallback) for everything else.
func New(st *store.Store, hub *events.Hub, logger *slog.Logger, cfg Config) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	ring := NewRingBuffer(500)
	h := &Handler{
		store:         st,
		hub:           hub,
		logger:        logger,
		registry:      cfg.Registry,
		anilist:       cfg.Anilist,
		animedb:       cfg.AnimeDB,
		extMgr:        cfg.ExtMgr,
		refresher:     cfg.Refresher,
		discovery:     cfg.Discovery,
		anilistDetail: cfg.AnilistDetail,
		anizip:        cfg.Anizip,
		logs:          ring,

		onShutdownRequest: cfg.OnShutdownRequest,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	// Security response headers (CSP, nosniff, frame-deny) on every response,
	// API and embedded SPA alike.
	r.Use(secureHeaders)

	r.Route("/api", func(api chi.Router) {
		// Loopback-only host check, CSRF Origin check, and body cap on the API.
		api.Use(localGuard)

		api.Get("/healthz", h.handleHealthz)
		api.Get("/ping", h.handlePing)
		api.Get("/version", h.handleVersion)
		api.Post("/shutdown", h.handleShutdown)
		api.Get("/events", h.handleEvents)
		api.Get("/settings", h.handleGetSettings)
		api.Put("/settings", h.handlePutSettings)
		api.Get("/stats", h.handleGetStats)
		api.Get("/queue", h.handleGetQueue)
		api.Get("/logs", h.handleGetLogs)

		// Discovery home + tracking
		api.Get("/discovery", h.handleDiscovery)
		api.Get("/tracked", h.handleGetTracked)
		api.Post("/track", h.handleTrackSeries)

		// AniList detail (series page; tracked + untracked alike)
		api.Get("/anilist/{id}/detail", h.handleAnilistDetail)

		// Series
		api.Route("/series", func(r chi.Router) {
			r.Get("/", h.handleListSeries)
			r.Post("/", h.handleCreateSeries)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.handleGetSeries)
				r.Patch("/", h.handlePatchSeries)
				r.Delete("/", h.handleDeleteSeries)
				r.Get("/episodes", h.handleListEpisodes)
				r.Post("/scan", h.handleScanEpisodes)
				r.Post("/refresh", h.handleRefreshSeries)
				r.Get("/available", h.handleAvailableEpisodes)
				r.Post("/available/download", h.handleDownloadAvailable)
				r.Post("/pause", h.handlePauseSeries)
				r.Post("/drop", h.handleDropSeries)
				r.Post("/resume", h.handleResumeSeries)
			})
		})

		// Episodes
		api.Post("/encode", h.handleBulkEncode)
		api.Route("/episodes/{id}", func(r chi.Router) {
			r.Post("/encode", h.handleEncodeEpisode)
			r.Post("/retry", h.handleRetryEpisode)
			r.Delete("/", h.handleDeleteEpisode)
		})

		// Search
		api.Get("/search/anilist", h.handleSearchAnilist)
		api.Get("/search/torrents", h.handleSearchTorrents)

		// Feeds
		api.Route("/feeds", func(r chi.Router) {
			r.Get("/", h.handleListFeeds)
			r.Post("/", h.handleCreateFeed)
			r.Route("/{id}", func(r chi.Router) {
				r.Patch("/", h.handlePatchFeed)
				r.Delete("/", h.handleDeleteFeed)
			})
		})

		// Profiles
		api.Route("/profiles", func(r chi.Router) {
			r.Get("/", h.handleListProfiles)
			r.Post("/", h.handleCreateProfile)
			r.Route("/{id}", func(r chi.Router) {
				r.Patch("/", h.handlePatchProfile)
				r.Delete("/", h.handleDeleteProfile)
				r.Get("/resolved", h.handleGetResolvedProfile)
			})
		})

		// Extensions
		api.Route("/extension-repos", func(r chi.Router) {
			r.Get("/", h.handleListExtensionRepos)
			r.Post("/", h.handleCreateExtensionRepo)
			r.Route("/{id}", func(r chi.Router) {
				r.Post("/install", h.handleInstallFromRepo)
				r.Delete("/", h.handleDeleteExtensionRepo)
			})
		})
		api.Get("/extensions", h.handleListExtensions)
		api.Post("/extensions/{id}/enable", h.handleEnableExtension)
		api.Post("/extensions/{id}/disable", h.handleDisableExtension)
		api.Delete("/extensions/{id}", h.handleUninstallExtension)
	})

	// Everything not under /api falls through to the embedded SPA.
	r.NotFound(spaHandler())

	return r
}
