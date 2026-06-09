package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/twosense/ssf-forwarder/internal/config"
	"github.com/twosense/ssf-forwarder/internal/ssfadmin"
)

func newAdminClient(ctx context.Context, cfg *config.Config) (*ssfadmin.Client, string, error) {
	authorizer, err := buildAuthorizer(cfg.Transmitter.Auth)
	if err != nil {
		return nil, "", err
	}

	pushURL, err := url.JoinPath(cfg.Receiver.PublicURL, cfg.Receiver.Endpoint)
	if err != nil {
		return nil, "", err
	}

	client, err := ssfadmin.NewClient(ctx, cfg.Transmitter.MetadataURL, authorizer)
	if err != nil {
		return nil, "", err
	}
	return client, pushURL, nil
}

func runRegister(cfg *config.Config) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	client, pushURL, err := newAdminClient(ctx, cfg)
	if err != nil {
		slog.Error("preparing registration", "err", err)
		os.Exit(1)
	}

	action, stream, err := ssfadmin.Reconcile(ctx, client, pushURL, cfg.Transmitter.EventsRequested)
	if err != nil {
		slog.Error("registering stream", "err", err)
		os.Exit(1)
	}

	slog.Info("stream registration reconciled", "action", action, "stream_id", stream.StreamID, "push_url", pushURL)
}

func runDeregister(cfg *config.Config) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	client, _, err := newAdminClient(ctx, cfg)
	if err != nil {
		slog.Error("preparing deregistration", "err", err)
		os.Exit(1)
	}

	streams, err := client.List(ctx)
	if err != nil {
		slog.Error("listing streams", "err", err)
		os.Exit(1)
	}

	stream, err := ssfadmin.FindOurStream(streams)
	if err != nil {
		slog.Error("finding stream", "err", err)
		os.Exit(1)
	}
	if stream == nil {
		slog.Info("no stream registered; nothing to deregister")
		return
	}

	if err := client.Delete(ctx, stream.StreamID); err != nil {
		slog.Error("deleting stream", "err", err)
		os.Exit(1)
	}
	slog.Info("stream deregistered", "stream_id", stream.StreamID)
}
