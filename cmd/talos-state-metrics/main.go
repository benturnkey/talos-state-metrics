package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/benturnkey/talos-state-metrics/internal/config"
	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
	taloseventsource "github.com/benturnkey/talos-state-metrics/internal/eventsource/talos"
	"github.com/benturnkey/talos-state-metrics/internal/health"
	"github.com/benturnkey/talos-state-metrics/internal/metrics"
	"github.com/benturnkey/talos-state-metrics/internal/state"
	"github.com/benturnkey/talos-state-metrics/internal/watch"
)

func main() {
	cfg := config.FromEnv()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	snapshot := state.NewSnapshot()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := &watch.Manager{
		Snapshot: snapshot,
		Factory: func() eventsource.Source {
			return taloseventsource.New(cfg.TalosEndpoint, cfg.TalosConfigPath, cfg.NodeName)
		},
		MinBackoff: cfg.MinBackoff,
		MaxBackoff: cfg.MaxBackoff,
		Logger:     logger,
	}
	go manager.Run(ctx)

	mux := http.NewServeMux()
	mux.Handle("/healthz", health.Healthz())
	mux.Handle("/readyz", health.Readyz(snapshot))
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(metrics.Render(snapshot.Copy())))
	})

	server := &http.Server{Addr: cfg.ListenAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.MaxBackoff)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("starting talos-state-metrics", "addr", cfg.ListenAddr, "talos_endpoint", cfg.TalosEndpoint)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("http server failed", "err", err)
		os.Exit(1)
	}
}
