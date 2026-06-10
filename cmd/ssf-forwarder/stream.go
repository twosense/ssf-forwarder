package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	seceventparser "github.com/sgnl-ai/caep.dev/secevent/pkg/parser"
	"github.com/sgnl-ai/caep.dev/ssfreceiver/auth"
	"github.com/twosense/ssf-forwarder/internal/config"
	"golang.org/x/oauth2/clientcredentials"
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

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
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
