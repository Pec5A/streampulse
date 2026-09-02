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

	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/infrastructure/config"
	"github.com/streampulse/backend/internal/infrastructure/persistence"
	"github.com/streampulse/backend/internal/infrastructure/storage"
	"github.com/streampulse/backend/internal/infrastructure/streaming"
	"github.com/streampulse/backend/internal/transport/http/handler"
	"github.com/streampulse/backend/internal/transport/http/router"
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
	slog.Info("config loaded", "env", cfg.Environment, "port", cfg.Port)

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
	// process, so it is created once here and shared by all requests.
	registry := streaming.NewRegistry()
	defer registry.CloseAll()

	fileStore, err := storage.NewLocal(cfg.StoragePath)
	if err != nil {
		return err
	}
	slog.Info("file storage ready", "path", cfg.StoragePath)

	authUC := usecase.NewAuthUseCase(userRepo, jwtManager, hasher)
	userUC := usecase.NewUserUseCase(userRepo)
	streamUC := usecase.NewStreamUseCase(streamRepo, registry)
	trackUC := usecase.NewTrackUseCase(trackRepo, fileStore)
	adminUC := usecase.NewAdminUseCase(userRepo)
	playlistUC := usecase.NewPlaylistUseCase(playlistRepo)

	handlers := router.Handlers{
		Auth:     handler.NewAuthHandler(authUC),
		User:     handler.NewUserHandler(userUC),
		Stream:   handler.NewStreamHandler(streamUC),
		Track:    handler.NewTrackHandler(trackUC),
		Admin:    handler.NewAdminHandler(adminUC),
		Playlist: handler.NewPlaylistHandler(playlistUC),
	}
	mux := router.New(handlers, jwtManager)

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
