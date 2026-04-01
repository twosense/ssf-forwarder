package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twosense/ssf-forwarder/internal/config"
	"github.com/twosense/ssf-forwarder/internal/handler"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("loading config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	meta, err := fetchTransmitterMetadata(ctx, cfg.Transmitter.MetadataURL)
	if err != nil {
		slog.Error("fetching transmitter metadata", "err", err)
		os.Exit(1)
	}

	sinks, err := buildSinks(cfg.Sinks)
	if err != nil {
		slog.Error("building sinks", "err", err)
		os.Exit(1)
	}

	pushURL := cfg.Receiver.PublicURL + cfg.Receiver.Endpoint

	stream, err := setupStream(ctx, cfg.Transmitter, pushURL)
	if err != nil {
		slog.Error("setting up stream", "err", err)
		os.Exit(1)
	}

	slog.Info("stream registered", "push_url", pushURL)

	mux := http.NewServeMux()
	mux.Handle(cfg.Receiver.Endpoint, handler.New(buildParser(meta), sinks))

	server := &http.Server{
		Addr:    cfg.Receiver.ListenAddr,
		Handler: mux,
	}

	go func() {
		slog.Info("listening", "addr", cfg.Receiver.ListenAddr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := stream.Delete(shutdownCtx); err != nil {
		slog.Warn("deleting stream", "err", err)
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Warn("server shutdown", "err", err)
	}
}
