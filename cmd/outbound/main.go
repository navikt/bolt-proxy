package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/navikt/bolt-proxy/internal/auth"
	"github.com/navikt/bolt-proxy/internal/bridge"
	"github.com/navikt/bolt-proxy/internal/config"

	"github.com/coder/websocket"
)

const (
	wsDialTimeout   = 5 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadOutbound()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	// Health + metrics HTTP server (Nais port).
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/isalive", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	healthMux.HandleFunc("/isready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// /metrics is served by the default mux which includes the prometheus handler
	// injected by the Nais platform.
	healthMux.Handle("/metrics", http.DefaultServeMux)
	healthSrv := &http.Server{
		Addr:              cfg.HealthAddr,
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("health server starting", "addr", cfg.HealthAddr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health server error", "error", err)
			os.Exit(1)
		}
	}()

	// Bolt TCP listener.
	ln, err := net.Listen("tcp", cfg.BoltListenAddr)
	if err != nil {
		logger.Error("failed to listen", "addr", cfg.BoltListenAddr, "error", err)
		os.Exit(1)
	}
	logger.Info("bolt listener starting", "addr", cfg.BoltListenAddr)

	// Graceful shutdown on SIGTERM/SIGINT.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		ln.Close()
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		healthSrv.Shutdown(shutCtx)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				logger.Error("accept error", "error", err)
				continue
			}
		}
		go handleConn(ctx, conn, cfg, logger)
	}
}

func handleConn(ctx context.Context, boltConn net.Conn, cfg config.Outbound, logger *slog.Logger) {
	remote := boltConn.RemoteAddr().String()
	logger.Info("new bolt connection", "remote", remote)
	defer boltConn.Close()

	wsConn, err := dialWS(ctx, cfg, false, logger)
	if err != nil {
		logger.Error("ws dial failed", "remote", remote, "error", err)
		return
	}

	// On 401, retry once with a fresh token (skip_cache=true).
	if wsConn == nil {
		logger.Info("ws upgrade rejected (401), retrying with fresh token", "remote", remote)
		wsConn, err = dialWS(ctx, cfg, true, logger)
		if err != nil {
			logger.Error("ws dial retry failed", "remote", remote, "error", err)
			return
		}
		if wsConn == nil {
			logger.Error("ws upgrade rejected after token refresh", "remote", remote)
			return
		}
	}

	// Wrap the WebSocket as a net.Conn (binary message stream) for the bridge.
	wsNetConn := websocket.NetConn(ctx, wsConn, websocket.MessageBinary)

	start := time.Now()
	result := bridge.Bridge(ctx, boltConn, wsNetConn)
	logger.Info("connection closed",
		"remote", remote,
		"duration_ms", time.Since(start).Milliseconds(),
		"bytes_sent", result.BytesAtoB,
		"bytes_recv", result.BytesBtoA,
	)
}

// dialWS fetches a token and opens a WebSocket connection to the inbound proxy.
// Returns (nil, nil) when the server responds with 401 (token rejected),
// signalling the caller to retry with skipCache=true.
func dialWS(ctx context.Context, cfg config.Outbound, skipCache bool, logger *slog.Logger) (*websocket.Conn, error) {
	token, err := auth.FetchToken(ctx, cfg.TokenEndpoint, cfg.TokenTarget, skipCache)
	if err != nil {
		return nil, fmt.Errorf("fetch token: %w", err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, wsDialTimeout)
	defer cancel()

	wsConn, resp, err := websocket.Dial(dialCtx, cfg.InboundWSURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			// Signal to caller: got 401, should retry with fresh token.
			return nil, nil
		}
		return nil, fmt.Errorf("websocket dial: %w", err)
	}

	return wsConn, nil
}
