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

	"github.com/benturnkey/talos-state-metrics/internal/config"
	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
	taloseventsource "github.com/benturnkey/talos-state-metrics/internal/eventsource/talos"
	"github.com/benturnkey/talos-state-metrics/internal/health"
	"github.com/benturnkey/talos-state-metrics/internal/metrics"
	"github.com/benturnkey/talos-state-metrics/internal/state"
	"github.com/benturnkey/talos-state-metrics/internal/watch"
	"golang.org/x/sync/errgroup"
)

const (
	httpReadTimeout       = 10 * time.Second
	httpReadHeaderTimeout = 5 * time.Second
	httpWriteTimeout      = 30 * time.Second
	httpIdleTimeout       = 60 * time.Second
)

func main() {
	cfg := config.FromEnv()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	snapshot := state.NewSnapshot()

	if err := run(context.Background(), cfg, logger, snapshot); err != nil {
		logger.Error("talos-state-metrics failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger, snapshot *state.Snapshot) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(ctx)

	manager := &watch.Manager{
		Snapshot: snapshot,
		Factory: func() eventsource.Source {
			src := taloseventsource.New(cfg.TalosEndpoint, cfg.TalosConfigPath, cfg.FullSyncInterval)
			src.Logger = logger
			return src
		},
		MinBackoff: cfg.MinBackoff,
		MaxBackoff: cfg.MaxBackoff,
		Logger:     logger,
	}

	mux := http.NewServeMux()
	mux.Handle("/healthz", health.Healthz())
	mux.Handle("/readyz", health.Readyz(snapshot))
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if _, err := w.Write([]byte(metrics.Render(snapshot.Copy()))); err != nil {
			logger.Warn("metrics response write failed", "err", err)
		}
	})

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadTimeout:       httpReadTimeout,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
	group.Go(func() error {
		manager.Run(groupCtx)
		if groupCtx.Err() != nil {
			return nil
		}

		return fmt.Errorf("watch manager exited unexpectedly")
	})

	group.Go(func() error {
		// When any critical goroutine fails or the process receives a shutdown signal,
		// stop accepting new HTTP work and give in-flight requests a bounded drain window.
		<-groupCtx.Done()
		logger.Info("shutting down http server", "timeout", cfg.ShutdownTimeout)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Warn("http server shutdown failed", "err", err)
		}
		return nil
	})

	logger.Info("starting talos-state-metrics", "addr", cfg.ListenAddr, "talos_endpoint", cfg.TalosEndpoint)
	group.Go(func() error {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http server failed: %w", err)
		}
		if groupCtx.Err() != nil {
			return nil
		}

		return fmt.Errorf("http server exited unexpectedly")
	})

	if err := group.Wait(); err != nil {
		return err
	}

	return nil
}
