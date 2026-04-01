package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
	seceventparser "github.com/sgnl-ai/caep.dev/secevent/pkg/parser"
	"github.com/sgnl-ai/caep.dev/ssfreceiver/auth"
	"github.com/sgnl-ai/caep.dev/ssfreceiver/builder"
	"golang.org/x/oauth2/clientcredentials"
	"github.com/twosense/ssf-forwarder/internal/config"
	"github.com/twosense/ssf-forwarder/internal/handler"
	"github.com/twosense/ssf-forwarder/internal/sink"
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

	transmitterMeta, err := fetchTransmitterMetadata(ctx, cfg.Transmitter.MetadataURL)
	if err != nil {
		slog.Error("fetching transmitter metadata", "err", err)
		os.Exit(1)
	}

	sinks, err := buildSinks(cfg.Sinks)
	if err != nil {
		slog.Error("building sinks", "err", err)
		os.Exit(1)
	}

	parserOpts := []seceventparser.Option{
		seceventparser.WithExpectedIssuer(transmitterMeta.Issuer),
	}
	if transmitterMeta.JWKSUri != "" {
		parserOpts = append(parserOpts, seceventparser.WithJWKSURL(transmitterMeta.JWKSUri))
	}

	p := seceventparser.NewParser(parserOpts...)
	h := handler.New(p, sinks)

	authorizer, err := buildAuthorizer(cfg.Transmitter.Auth)
	if err != nil {
		slog.Error("building authorizer", "err", err)
		os.Exit(1)
	}

	pushURL := cfg.Receiver.PublicURL + cfg.Receiver.Endpoint

	eventTypes := make([]event.EventType, len(cfg.Transmitter.EventsRequested))
	for i, et := range cfg.Transmitter.EventsRequested {
		eventTypes[i] = event.EventType(et)
	}

	builderOpts := []builder.Option{
		builder.WithPushDelivery(pushURL),
		builder.WithAuth(authorizer),
		builder.WithExistingCheck(),
	}
	if len(eventTypes) > 0 {
		builderOpts = append(builderOpts, builder.WithEventTypes(eventTypes))
	}

	b, err := builder.New(cfg.Transmitter.MetadataURL, builderOpts...)
	if err != nil {
		slog.Error("creating stream builder", "err", err)
		os.Exit(1)
	}

	stream, err := b.Setup(ctx)
	if err != nil {
		slog.Error("setting up stream", "err", err)
		os.Exit(1)
	}

	slog.Info("stream registered", "push_url", pushURL)

	mux := http.NewServeMux()
	mux.Handle(cfg.Receiver.Endpoint, h)

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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*1e9)
	defer shutdownCancel()

	if err := stream.Delete(shutdownCtx); err != nil {
		slog.Warn("deleting stream", "err", err)
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Warn("server shutdown", "err", err)
	}
}

func buildAuthorizer(authCfg config.AuthConfig) (auth.Authorizer, error) {
	switch authCfg.Type {
	case "bearer":
		return auth.NewBearer(authCfg.Token)
	case "oauth2":
		return auth.NewOAuth2ClientCredentials(&clientcredentials.Config{
			TokenURL:     authCfg.TokenURL,
			ClientID:     authCfg.ClientID,
			ClientSecret: authCfg.ClientSecret,
		})
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", authCfg.Type)
	}
}

func buildSinks(sinkCfgs []config.SinkConfig) ([]sink.Sink, error) {
	sinks := make([]sink.Sink, 0, len(sinkCfgs))

	for i, sc := range sinkCfgs {
		switch sc.Type {
		case "webhook":
			ws, err := sink.NewWebhookSink(sc.URL, sc.Headers, sc.BodyTemplate)
			if err != nil {
				return nil, fmt.Errorf("sinks[%d]: %w", i, err)
			}

			sinks = append(sinks, ws)
		case "log":
			sinks = append(sinks, sink.NewLogSink(slog.Default()))
		default:
			return nil, fmt.Errorf("sinks[%d]: unsupported type %q", i, sc.Type)
		}
	}

	return sinks, nil
}

// transmitterMetadata holds the fields we need from the SSF transmitter metadata endpoint.
type transmitterMetadata struct {
	Issuer  string `json:"issuer"`
	JWKSUri string `json:"jwks_uri"`
}

func fetchTransmitterMetadata(ctx context.Context, metadataURL string) (*transmitterMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching metadata: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata endpoint returned status %d", resp.StatusCode)
	}

	var meta transmitterMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("decoding metadata: %w", err)
	}

	if meta.Issuer == "" {
		return nil, fmt.Errorf("metadata missing required field: issuer")
	}

	return &meta, nil
}
