package config

import (
	"os"
	"strings"
	"testing"
)

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	f.Close()

	return f.Name()
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
		check   func(*testing.T, *Config)
	}{
		{
			name: "valid bearer config",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			check: func(t *testing.T, c *Config) {
				if c.Receiver.PublicURL != "https://receiver.example.com" {
					t.Errorf("PublicURL = %q, want %q", c.Receiver.PublicURL, "https://receiver.example.com")
				}
				if c.Transmitter.Auth.Type != "bearer" {
					t.Errorf("Auth.Type = %q, want bearer", c.Transmitter.Auth.Type)
				}
				if c.Transmitter.Auth.Token != "secret" {
					t.Errorf("Auth.Token = %q, want secret", c.Transmitter.Auth.Token)
				}
				if len(c.Sinks) != 1 {
					t.Fatalf("len(Sinks) = %d, want 1", len(c.Sinks))
				}
				if c.Sinks[0].URL != "https://webhook.example.com/events" {
					t.Errorf("Sinks[0].URL = %q", c.Sinks[0].URL)
				}
			},
		},
		{
			name: "valid oauth2 config",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: oauth2
    token_url: "https://auth.example.com/token"
    client_id: "client"
    client_secret: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			check: func(t *testing.T, c *Config) {
				if c.Transmitter.Auth.Type != "oauth2" {
					t.Errorf("Auth.Type = %q, want oauth2", c.Transmitter.Auth.Type)
				}
				if c.Transmitter.Auth.ClientID != "client" {
					t.Errorf("Auth.ClientID = %q, want client", c.Transmitter.Auth.ClientID)
				}
			},
		},
		{
			name: "defaults applied when omitted",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			check: func(t *testing.T, c *Config) {
				if c.Receiver.ListenAddr != ":8080" {
					t.Errorf("ListenAddr = %q, want :8080", c.Receiver.ListenAddr)
				}
				if c.Receiver.Endpoint != "/events" {
					t.Errorf("Endpoint = %q, want /events", c.Receiver.Endpoint)
				}
			},
		},
		{
			name: "explicit listen_addr and endpoint not overridden",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
  listen_addr: ":9090"
  endpoint: "/push"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			check: func(t *testing.T, c *Config) {
				if c.Receiver.ListenAddr != ":9090" {
					t.Errorf("ListenAddr = %q, want :9090", c.Receiver.ListenAddr)
				}
				if c.Receiver.Endpoint != "/push" {
					t.Errorf("Endpoint = %q, want /push", c.Receiver.Endpoint)
				}
			},
		},
		{
			name: "events_requested populated",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
  events_requested:
    - "https://schemas.openid.net/secevent/caep/event-type/session-revoked"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			check: func(t *testing.T, c *Config) {
				if len(c.Transmitter.EventsRequested) != 1 {
					t.Fatalf("len(EventsRequested) = %d, want 1", len(c.Transmitter.EventsRequested))
				}
				want := "https://schemas.openid.net/secevent/caep/event-type/session-revoked"
				if c.Transmitter.EventsRequested[0] != want {
					t.Errorf("EventsRequested[0] = %q, want %q", c.Transmitter.EventsRequested[0], want)
				}
			},
		},
		{
			name: "sink with optional headers and body_template",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
    headers:
      Authorization: "Bearer abc"
    body_template: '{"token":"{{.RawToken}}"}'
`,
			check: func(t *testing.T, c *Config) {
				s := c.Sinks[0]
				if s.Headers["Authorization"] != "Bearer abc" {
					t.Errorf("Headers[Authorization] = %q", s.Headers["Authorization"])
				}
				if s.BodyTemplate == "" {
					t.Error("BodyTemplate should not be empty")
				}
			},
		},
		{
			name: "public_url without scheme rejected",
			yaml: `
receiver:
  public_url: "receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "receiver.public_url must be an absolute HTTP or HTTPS URL",
		},
		{
			name: "public_url with non-http scheme rejected",
			yaml: `
receiver:
  public_url: "ftp://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "receiver.public_url must be an absolute HTTP or HTTPS URL",
		},
		{
			name: "endpoint without leading slash rejected",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
  endpoint: "events"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "receiver.endpoint must start with /",
		},
		{
			name: "missing receiver public_url",
			yaml: `
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "receiver.public_url is required",
		},
		{
			name: "missing transmitter metadata_url",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "transmitter.metadata_url is required",
		},
		{
			name: "invalid auth type",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: apikey
    token: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "transmitter.auth.type must be 'bearer' or 'oauth2'",
		},
		{
			name: "bearer auth missing token",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "transmitter.auth.token is required for bearer auth",
		},
		{
			name: "oauth2 missing token_url",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: oauth2
    client_id: "client"
    client_secret: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "transmitter.auth.token_url is required for oauth2 auth",
		},
		{
			name: "oauth2 missing client_id",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: oauth2
    token_url: "https://auth.example.com/token"
    client_secret: "secret"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "transmitter.auth.client_id is required for oauth2 auth",
		},
		{
			name: "oauth2 missing client_secret",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: oauth2
    token_url: "https://auth.example.com/token"
    client_id: "client"
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			wantErr: "transmitter.auth.client_secret is required for oauth2 auth",
		},
		{
			name: "no sinks",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
`,
			wantErr: "at least one sink is required",
		},
		{
			name: "log sink requires no url",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: log
`,
			check: func(t *testing.T, c *Config) {
				if len(c.Sinks) != 1 {
					t.Fatalf("len(Sinks) = %d, want 1", len(c.Sinks))
				}
				if c.Sinks[0].Type != "log" {
					t.Errorf("Sinks[0].Type = %q, want log", c.Sinks[0].Type)
				}
			},
		},
		{
			name: "log and webhook sinks together",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: log
  - type: webhook
    url: "https://webhook.example.com/events"
`,
			check: func(t *testing.T, c *Config) {
				if len(c.Sinks) != 2 {
					t.Fatalf("len(Sinks) = %d, want 2", len(c.Sinks))
				}
			},
		},
		{
			name: "unsupported sink type",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: kafka
    url: "https://webhook.example.com/events"
`,
			wantErr: `unsupported sink type "kafka"`,
		},
		{
			name: "sink missing url",
			yaml: `
receiver:
  public_url: "https://receiver.example.com"
transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "secret"
sinks:
  - type: webhook
`,
			wantErr: "sinks[0].url is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			path := writeTempYAML(t, tc.yaml)

			// Act
			cfg, err := Load(path)

			// Assert
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	// Arrange
	path := "/nonexistent/path/config.yaml"

	// Act
	_, err := Load(path)

	// Assert
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "reading config file") {
		t.Errorf("error %q should mention reading config file", err.Error())
	}
}
