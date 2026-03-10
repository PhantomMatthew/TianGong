package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// ServerConfig holds the configuration for the HTTP server.
type ServerConfig struct {
	Host string
	Port int
}

// Gateway is an HTTP server for the TianGong platform.
type Gateway struct {
	cfg    ServerConfig
	server *http.Server
}

// New creates a new Gateway with the given configuration.
func New(cfg ServerConfig) *Gateway {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	return &Gateway{
		cfg: cfg,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

// Start starts the HTTP server. It blocks until the server is shut down or an error occurs.
func (g *Gateway) Start(ctx context.Context) error {
	slog.Info("starting gateway", "addr", g.server.Addr)

	if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("gateway server error", "err", err)
		return fmt.Errorf("gateway server failed: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the HTTP server with the given context timeout.
func (g *Gateway) Stop(ctx context.Context) error {
	slog.Info("stopping gateway")

	// Create a shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := g.server.Shutdown(shutdownCtx); err != nil {
		slog.Error("gateway shutdown error", "err", err)
		return fmt.Errorf("gateway shutdown failed: %w", err)
	}

	slog.Info("gateway stopped")
	return nil
}

// healthHandler handles the GET /health endpoint.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]string{"status": "ok"}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode health response", "error", err)
	}
}
