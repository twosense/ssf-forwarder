package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twosense/ssf-forwarder/internal/config"
	"github.com/twosense/ssf-forwarder/internal/handler"
	"github.com/twosense/ssf-forwarder/internal/ssfadmin"
)

func runServe(cfg *config.Config) {
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

	pushURL, err := url.JoinPath(cfg.Receiver.PublicURL, cfg.Receiver.Endpoint)
	if err != nil {
		slog.Error("building push URL", "err", err)
		os.Exit(1)
	}

	client, err := buildAdminClient(ctx, cfg)
	if err != nil {
		slog.Error("creating stream admin client", "err", err)
		os.Exit(1)
	}

	autoRegister := cfg.Receiver.AutoRegister != nil && *cfg.Receiver.AutoRegister

	var registeredStreamID string
	if autoRegister {
		action, stream, err := ssfadmin.Reconcile(ctx, client, pushURL, cfg.Transmitter.EventsRequested)
		if err != nil {
			slog.Error("registering stream", "err", err)
			os.Exit(1)
		}
		registeredStreamID = stream.StreamID

		if action == ssfadmin.ActionCreated {
			slog.Info("stream registered", "stream_id", stream.StreamID, "push_url", pushURL)
		} else {
			slog.Warn("found an existing stream on boot; a previous run may have shut down uncleanly, or another instance is already registered. to run multiple instances, disable auto_register and manage the stream with the register/deregister commands.",
				"action", action, "stream_id", stream.StreamID, "push_url", pushURL)
		}
	} else {
		runBootSafeguard(ctx, client, pushURL)
	}

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

	// Only the auto-register path owns the stream, so only it deletes on shutdown.
	if autoRegister && registeredStreamID != "" {
		if err := client.Delete(shutdownCtx, registeredStreamID); err != nil {
			slog.Warn("deleting stream", "err", err)
		}
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Warn("server shutdown", "err", err)
	}
}

func runBootSafeguard(ctx context.Context, client *ssfadmin.Client, pushURL string) {
	result, err := ssfadmin.Check(ctx, client, pushURL)
	if err != nil {
		slog.Warn("could not verify stream registration; continuing", "err", err)
		return
	}

	switch result.Status {
	case ssfadmin.SafeguardOK:
		slog.Info("stream registered", "push_url", pushURL)
	case ssfadmin.SafeguardURLMismatch:
		slog.Warn("a stream is registered for a different URL; run `ssf-forwarder register` to update it",
			"registered_url", result.RegisteredURL, "expected_url", pushURL)
	case ssfadmin.SafeguardMissing:
		slog.Warn("no stream registered for this forwarder; events will not be delivered until you run `ssf-forwarder register`",
			"push_url", pushURL)
	}
}
