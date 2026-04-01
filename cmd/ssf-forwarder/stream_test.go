package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/twosense/ssf-forwarder/internal/config"
)

// minimalSSFMetadata returns a JSON-encoded SSF transmitter metadata object
// whose configuration_endpoint points back to configEndpointURL.
func minimalSSFMetadata(issuer, configEndpointURL string) []byte {
	b, _ := json.Marshal(map[string]any{
		"issuer":                       issuer,
		"jwks_uri":                     issuer + "/jwks",
		"delivery_methods_supported":   []string{"urn:ietf:rfc:8935"},
		"configuration_endpoint":       configEndpointURL,
	})
	return b
}

// --- fetchTransmitterMetadata ---

func TestFetchTransmitterMetadata(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantIssuer string
		wantJWKS   string
		wantErr    string
	}{
		{
			name: "valid response with issuer and jwks_uri",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"issuer":   "https://transmitter.example.com",
					"jwks_uri": "https://transmitter.example.com/jwks",
				})
			},
			wantIssuer: "https://transmitter.example.com",
			wantJWKS:   "https://transmitter.example.com/jwks",
		},
		{
			name: "valid response without jwks_uri",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"issuer": "https://transmitter.example.com",
				})
			},
			wantIssuer: "https://transmitter.example.com",
		},
		{
			name: "non-200 status returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr: "status 404",
		},
		{
			name: "invalid JSON returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("not json"))
			},
			wantErr: "decoding metadata",
		},
		{
			name: "missing issuer returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"jwks_uri": "https://transmitter.example.com/jwks",
				})
			},
			wantErr: "missing required field: issuer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			// Act
			meta, err := fetchTransmitterMetadata(context.Background(), server.URL)

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
			if meta.Issuer != tc.wantIssuer {
				t.Errorf("Issuer = %q, want %q", meta.Issuer, tc.wantIssuer)
			}
			if meta.JWKSUri != tc.wantJWKS {
				t.Errorf("JWKSUri = %q, want %q", meta.JWKSUri, tc.wantJWKS)
			}
		})
	}
}

func TestFetchTransmitterMetadata_UnreachableServer(t *testing.T) {
	// Arrange
	// Act
	_, err := fetchTransmitterMetadata(context.Background(), "http://127.0.0.1:1")

	// Assert
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}

// --- buildAuthorizer ---

func TestBuildAuthorizer(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.AuthConfig
		wantErr string
	}{
		{
			name: "bearer auth",
			cfg:  config.AuthConfig{Type: "bearer", Token: "my-token"},
		},
		{
			name: "oauth2 auth",
			cfg: config.AuthConfig{
				Type:         "oauth2",
				TokenURL:     "https://auth.example.com/token",
				ClientID:     "client",
				ClientSecret: "secret",
			},
		},
		{
			name:    "unsupported type",
			cfg:     config.AuthConfig{Type: "apikey"},
			wantErr: "unsupported auth type",
		},
		{
			name:    "bearer with empty token",
			cfg:     config.AuthConfig{Type: "bearer", Token: ""},
			wantErr: "token cannot be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange — config set above

			// Act
			a, err := buildAuthorizer(tc.cfg)

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
			if a == nil {
				t.Error("expected non-nil authorizer")
			}
		})
	}
}

// --- setupStream ---

func TestSetupStream_TransmitterError(t *testing.T) {
	tests := []struct {
		name           string
		metadataStatus int
		configStatus   int
		wantErr        string
	}{
		{
			// 401 is not in the library's retryable status set, so this fails fast.
			name:           "metadata endpoint returns error",
			metadataStatus: http.StatusUnauthorized,
			wantErr:        "failed to fetch transmitter metadata",
		},
		{
			name:           "configuration endpoint rejects stream creation",
			metadataStatus: http.StatusOK,
			configStatus:   http.StatusForbidden,
			wantErr:        "unexpected status code",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var serverURL string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/ssf-configuration":
					if tc.metadataStatus != http.StatusOK {
						w.WriteHeader(tc.metadataStatus)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.Write(minimalSSFMetadata(
						"https://transmitter.example.com",
						serverURL+"/config",
					))
				case "/config":
					w.WriteHeader(tc.configStatus)
				}
			}))
			defer server.Close()
			serverURL = server.URL

			cfg := config.TransmitterConfig{
				MetadataURL:     server.URL + "/ssf-configuration",
				Auth:            config.AuthConfig{Type: "bearer", Token: "test-token"},
				EventsRequested: []string{"https://schemas.openid.net/secevent/caep/event-type/session-revoked"},
			}

			// Act
			_, err := setupStream(context.Background(), cfg, "https://receiver.example.com/events")

			// Assert
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestSetupStream_InvalidPushURL(t *testing.T) {
	// Arrange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	cfg := config.TransmitterConfig{
		MetadataURL: server.URL,
		Auth:        config.AuthConfig{Type: "bearer", Token: "test-token"},
	}

	// Act: push URL is not a valid absolute URL
	_, err := setupStream(context.Background(), cfg, "://bad-url")

	// Assert
	if err == nil {
		t.Fatal("expected error for invalid push URL, got nil")
	}
}
