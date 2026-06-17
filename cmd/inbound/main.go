package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof on http.DefaultServeMux
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/navikt/bolt-proxy/internal/auth"
	"github.com/navikt/bolt-proxy/internal/bridge"
	"github.com/navikt/bolt-proxy/internal/config"

	"github.com/coder/websocket"
)

const (
	neo4jDialTimeout  = 2 * time.Second
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadInbound()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/isalive", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/isready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// /metrics and /debug/pprof/ are registered on http.DefaultServeMux by
	// the Nais platform and net/http/pprof respectively; proxy them here.
	mux.Handle("/metrics", http.DefaultServeMux)
	mux.Handle("/debug/", http.DefaultServeMux)
	mux.HandleFunc("/ws", makeWSHandler(cfg, logger))

	srv := &http.Server{
		Addr:              cfg.HealthAddr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	logger.Info("inbound proxy starting", "addr", cfg.HealthAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func makeWSHandler(cfg config.Inbound, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remoteAddr := r.RemoteAddr

		// Extract bearer token before upgrading — reject unauthorized requests
		// at the HTTP layer so they never become WebSocket connections.
		token := extractBearer(r)
		if token == "" {
			logger.Info("rejected: missing authorization", "remote", remoteAddr)
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}

		result, err := auth.Introspect(r.Context(), cfg.IntrospectionEndpoint, token)
		if err != nil {
			logger.Error("token introspection failed", "remote", remoteAddr, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !result.Active {
			// Log azp if present, so we can see who is trying even when rejected.
			logger.Info("rejected: invalid token", "remote", remoteAddr, "azp", result.AZP)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Token is valid — upgrade to WebSocket.
		wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			CompressionMode: websocket.CompressionDisabled,
		})
		if err != nil {
			// Accept already wrote the HTTP error response.
			logger.Error("websocket accept failed", "remote", remoteAddr, "azp", result.AZP, "error", err)
			return
		}

		// Dial Neo4j ClusterIP.
		dialCtx, cancel := context.WithTimeout(r.Context(), neo4jDialTimeout)
		defer cancel()

		neo4jConn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Neo4jBoltAddr)
		if err != nil {
			logger.Error("neo4j dial failed", "remote", remoteAddr, "azp", result.AZP, "error", err)
			wsConn.Close(websocket.StatusInternalError, "upstream unavailable")
			return
		}

		logger.Info("connection accepted",
			"remote", remoteAddr,
			"azp", result.AZP,
			"neo4j_addr", cfg.Neo4jBoltAddr,
		)

		// Bridge: no timeout — Bolt sessions can be long-running (full sync).
		wsNetConn := websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary)
		start := time.Now()
		res := bridge.Bridge(context.Background(), neo4jConn, wsNetConn)

		logger.Info("connection closed",
			"remote", remoteAddr,
			"azp", result.AZP,
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes_to_neo4j", res.BytesAtoB,
			"bytes_from_neo4j", res.BytesBtoA,
		)
	}
}

func extractBearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(v, "Bearer "); ok {
		return after
	}
	return ""
}
