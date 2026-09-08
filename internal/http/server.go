// Package gateway provides the HTTP server for the Nexus AI Gateway.
package gateway

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/config"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
)

// Server is the HTTP gateway server.
type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
}

// Dependencies holds all resolved dependencies needed to build the server.
type Dependencies struct {
	Config  *config.Config
	Logger  *zap.Logger
	Metrics *observability.Metrics
	// Handlers injected by main.go after wiring
	ChatHandler          http.HandlerFunc
	ModelsHandler        http.HandlerFunc
	OrganizationsHandler http.Handler
	ProjectsHandler      http.Handler
	APIKeysHandler       http.Handler
	ProvidersHandler     http.Handler
	UsageHandler         http.HandlerFunc
	RequestsHandler      http.HandlerFunc
}

// New creates a configured HTTP server with all middleware and routes registered.
func New(deps Dependencies) *Server {
	r := chi.NewRouter()

	// --- Core middleware stack ---
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Recoverer)
	r.Use(observability.RequestIDMiddleware)
	r.Use(observability.LoggingMiddleware(deps.Logger))

	// --- Health & readiness (no auth) ---
	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler)

	// --- Prometheus metrics endpoint (internal, no auth) ---
	r.Handle("/metrics", observability.PrometheusHandler())

	// --- v1 API ---
	r.Route("/v1", func(r chi.Router) {
		// OpenAI-compatible inference endpoint
		r.Post("/chat/completions", deps.ChatHandler)

		// Model listing
		r.Get("/models", deps.ModelsHandler)

		// Admin endpoints
		r.Mount("/organizations", deps.OrganizationsHandler)
		r.Mount("/projects", deps.ProjectsHandler)
		r.Mount("/api-keys", deps.APIKeysHandler)
		r.Mount("/providers", deps.ProvidersHandler)

		// Analytics
		r.Get("/usage", deps.UsageHandler)
		r.Get("/requests/{requestID}", deps.RequestsHandler)
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", deps.Config.Server.Port),
		Handler:      r,
		ReadTimeout:  time.Duration(deps.Config.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(deps.Config.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:  time.Duration(deps.Config.Server.IdleTimeoutSec) * time.Second,
	}

	return &Server{httpServer: srv, logger: deps.Logger}
}

// Start begins listening. It blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("gateway server starting", zap.String("addr", s.httpServer.Addr))

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		s.logger.Info("gateway server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}

// healthHandler returns 200 OK for liveness probes.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// readyHandler returns 200 OK when the service is ready to serve traffic.
func readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}
