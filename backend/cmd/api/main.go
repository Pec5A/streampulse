// Package main is the entry point of the StreamPulse API.
// It wires configuration, persistence and HTTP handlers, then runs the
// server with graceful shutdown.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/infrastructure/config"
	"github.com/streampulse/backend/internal/infrastructure/observability"
	"github.com/streampulse/backend/internal/infrastructure/persistence"
	"github.com/streampulse/backend/internal/infrastructure/storage"
	"github.com/streampulse/backend/internal/infrastructure/streaming"
	"github.com/streampulse/backend/internal/transport/http/handler"
	"github.com/streampulse/backend/internal/transport/http/router"
)

// Overwritten at build time by the Dockerfile's -ldflags -X. Declared here
// because -X on a symbol that does not exist is silently ignored: the build
// looked stamped while every image reported nothing.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Installed as the default before anything else logs, so no line escapes
	// in the stdlib's text format.
	slog.SetDefault(observability.NewLogger(cfg.Environment))
	slog.Info("config loaded", "env", cfg.Environment, "port", cfg.Port)

	shutdownTracing, err := observability.InitTracing(context.Background(), observability.TracingConfig{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Environment:    cfg.Environment,
		Endpoint:       cfg.OTLPEndpoint,
		SampleRatio:    cfg.TraceSampleRatio,
	})
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		// Its own timeout, and deliberately not the request context: this runs
		// after shutdown has already been signalled, so a context derived from
		// it would be cancelled and the final batch of spans dropped.
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(flushCtx); err != nil {
			slog.Error("flush traces", "err", err)
		}
	}()
	slog.Info("tracing initialised", "otlp_endpoint", cfg.OTLPEndpoint, "sample_ratio", cfg.TraceSampleRatio)

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	db, err := persistence.Open(dbCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	slog.Info("database connected")

	migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer migrateCancel()
	if err := persistence.Migrate(migrateCtx, db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("migrations applied")

	userRepo := persistence.NewUserRepository(db)
	streamRepo := persistence.NewStreamRepository(db)
	trackRepo := persistence.NewTrackRepository(db)
	playlistRepo := persistence.NewPlaylistRepository(db)
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTExpiration)
	hasher := auth.NewBcryptHasher()

	// The streaming registry holds every live broadcast in memory for this
	// process, so it is created once here and shared by all requests. The
	// chat registry follows the exact same lifecycle, one room per live
	// stream, opened and closed by StreamUseCase in lockstep with the audio
	// hub — see StreamUseCase.StartLive/StopLive.
	registry := streaming.NewRegistry()
	defer registry.CloseAll()
	chatRegistry := streaming.NewChatRegistry()
	defer chatRegistry.CloseAll()

	// The streaming metrics are read from the registry at scrape time. This
	// adapter is the only place the two packages meet: observability declares
	// the shape it needs, streaming owns the numbers, and neither imports the
	// other. Registered on the default registerer, which is what promhttp
	// serves on /metrics.
	prometheus.MustRegister(observability.NewStreamingCollector(func() observability.StreamingTotals {
		t := registry.Totals()
		return observability.StreamingTotals{
			ActiveStreams:   t.ActiveStreams,
			ActiveListeners: t.ActiveListeners,
			SessionsStarted: t.SessionsStarted,
			BytesPublished:  t.BytesPublished,
			ChunksDropped:   t.ChunksDropped,
			Evictions:       t.Evictions,
		}
	}))

	fileStore, err := storage.NewLocal(cfg.StoragePath)
	if err != nil {
		return err
	}
	slog.Info("file storage ready", "path", cfg.StoragePath)

	authUC := usecase.NewAuthUseCase(userRepo, jwtManager, hasher)
	userUC := usecase.NewUserUseCase(userRepo)
	streamUC := usecase.NewStreamUseCase(streamRepo, userRepo, registry, chatRegistry)
	trackUC := usecase.NewTrackUseCase(trackRepo, fileStore)
	adminUC := usecase.NewAdminUseCase(userRepo)
	playlistUC := usecase.NewPlaylistUseCase(playlistRepo)

	handlers := router.Handlers{
		Build:    router.BuildInfo{Version: version, Commit: commit},
		Auth:     handler.NewAuthHandler(authUC),
		User:     handler.NewUserHandler(userUC),
		Stream:   handler.NewStreamHandler(streamUC),
		Track:    handler.NewTrackHandler(trackUC),
		Admin:    handler.NewAdminHandler(adminUC),
		Playlist: handler.NewPlaylistHandler(playlistUC),
	}
	mux := router.New(handlers, jwtManager, router.Options{
		Environment:      cfg.Environment,
		AllowedOrigins:   cfg.AllowedOrigins,
		MetricsToken:     cfg.MetricsToken,
		AuthRateLimit:    cfg.AuthRateLimit,
		TrustedProxyHops: cfg.TrustedProxyHops,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("starting streampulse api", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
