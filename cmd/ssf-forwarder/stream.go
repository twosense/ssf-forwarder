package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
	seceventparser "github.com/sgnl-ai/caep.dev/secevent/pkg/parser"
	"github.com/sgnl-ai/caep.dev/ssfreceiver/auth"
	"github.com/sgnl-ai/caep.dev/ssfreceiver/builder"
	"github.com/sgnl-ai/caep.dev/ssfreceiver/stream"
	"golang.org/x/oauth2/clientcredentials"
	"github.com/twosense/ssf-forwarder/internal/config"
)

// transmitterMetadata holds the fields needed from the SSF transmitter metadata endpoint.
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

func buildParser(meta *transmitterMetadata) *seceventparser.Parser {
	opts := []seceventparser.Option{
		seceventparser.WithExpectedIssuer(meta.Issuer),
	}
	if meta.JWKSUri != "" {
		opts = append(opts, seceventparser.WithJWKSURL(meta.JWKSUri))
	}

	return seceventparser.NewParser(opts...)
}

func setupStream(ctx context.Context, cfg config.TransmitterConfig, pushURL string) (stream.Stream, error) {
	authorizer, err := buildAuthorizer(cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("building authorizer: %w", err)
	}

	eventTypes := make([]event.EventType, len(cfg.EventsRequested))
	for i, et := range cfg.EventsRequested {
		eventTypes[i] = event.EventType(et)
	}

	opts := []builder.Option{
		builder.WithPushDelivery(pushURL),
		builder.WithAuth(authorizer),
		builder.WithExistingCheck(),
	}
	if len(eventTypes) > 0 {
		opts = append(opts, builder.WithEventTypes(eventTypes))
	}

	b, err := builder.New(cfg.MetadataURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating stream builder: %w", err)
	}

	return b.Setup(ctx)
}

func buildAuthorizer(cfg config.AuthConfig) (auth.Authorizer, error) {
	switch cfg.Type {
	case "bearer":
		return auth.NewBearer(cfg.Token)
	case "oauth2":
		return auth.NewOAuth2ClientCredentials(&clientcredentials.Config{
			TokenURL:     cfg.TokenURL,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
		})
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", cfg.Type)
	}
}
