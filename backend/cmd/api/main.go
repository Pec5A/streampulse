// Package main is the entry point of the StreamPulse API.
// It wires configuration, persistence and HTTP handlers, then runs the
// server with graceful shutdown.
package main

import (
	"context"
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

	userRepo := persistence.NewUserRepository(db)
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTExpiration)
	hasher := auth.NewBcryptHasher()

	authUC := usecase.NewAuthUseCase(userRepo, jwtManager, hasher)
	adminUC := usecase.NewAdminUseCase(userRepo)

	handlers := router.Handlers{
		Auth:  handler.NewAuthHandler(authUC),
		Admin: handler.NewAdminHandler(adminUC),
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
