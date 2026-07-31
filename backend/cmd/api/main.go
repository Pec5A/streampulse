// Package main is the entry point of the StreamPulse API.
// This is intentionally a minimal bootstrap: a health endpoint only.
// Each feature (auth, streaming, playlists, admin, chat, observability...)
// is wired in here by the ticket that builds it — see docs/team/plan.md.
package main

import (
	"log/slog"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	slog.Info("starting streampulse api", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
