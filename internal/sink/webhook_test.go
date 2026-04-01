package sink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeJWT builds a JWT-shaped string (header.payload.sig) with the given claims.
// The signature is fake; this is only for testing claim extraction.
func fakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"secevent+jwt"}`))

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshaling claims: %v", err)
	}

	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)

	return header + "." + payload + ".fakesig"
}

func TestNewWebhookSink(t *testing.T) {
	tests := []struct {
		name         string
		bodyTemplate string
		wantErr      bool
	}{
		{
			name:         "no template",
			bodyTemplate: "",
		},
		{
			name:         "valid template",
			bodyTemplate: `{"token":"{{.RawToken}}"}`,
		},
		{
			name:         "invalid template syntax",
			bodyTemplate: `{{.Unclosed`,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange (url and headers are irrelevant for construction)

			// Act
			ws, err := NewWebhookSink("https://example.com", nil, tc.bodyTemplate)

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ws == nil {
				t.Fatal("expected non-nil WebhookSink")
			}
		})
	}
}

func TestWebhookSink_Send(t *testing.T) {
	tests := []struct {
		name         string
		bodyTemplate string
		sinkHeaders  map[string]string
		serverStatus int
		rawToken     string
		incomingCT   string
		wantErr      bool
		checkReq     func(*testing.T, *http.Request, []byte)
	}{
		{
			name:         "raw token forwarded when no template",
			serverStatus: http.StatusOK,
			rawToken:     "header.payload.sig",
			checkReq: func(t *testing.T, r *http.Request, body []byte) {
				if string(body) != "header.payload.sig" {
					t.Errorf("body = %q, want %q", body, "header.payload.sig")
				}
			},
		},
		{
			name:         "incoming content-type forwarded",
			serverStatus: http.StatusOK,
			rawToken:     "header.payload.sig",
			incomingCT:   "application/secevent+jwt",
			checkReq: func(t *testing.T, r *http.Request, _ []byte) {
				if ct := r.Header.Get("Content-Type"); ct != "application/secevent+jwt" {
					t.Errorf("Content-Type = %q, want application/secevent+jwt", ct)
				}
			},
		},
		{
			name:         "static headers added to request",
			serverStatus: http.StatusOK,
			rawToken:     "header.payload.sig",
			sinkHeaders:  map[string]string{"Authorization": "Bearer token123", "X-Custom": "value"},
			checkReq: func(t *testing.T, r *http.Request, _ []byte) {
				if got := r.Header.Get("Authorization"); got != "Bearer token123" {
					t.Errorf("Authorization = %q, want Bearer token123", got)
				}
				if got := r.Header.Get("X-Custom"); got != "value" {
					t.Errorf("X-Custom = %q, want value", got)
				}
			},
		},
		{
			name:         "body template renders RawToken",
			serverStatus: http.StatusOK,
			rawToken:     "header.payload.sig",
			bodyTemplate: `{"token":"{{.RawToken}}"}`,
			checkReq: func(t *testing.T, r *http.Request, body []byte) {
				want := `{"token":"header.payload.sig"}`
				if string(body) != want {
					t.Errorf("body = %q, want %q", body, want)
				}
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
			},
		},
		{
			name:         "body template accesses parsed claims",
			serverStatus: http.StatusOK,
			bodyTemplate: `{"issuer":"{{index .Claims "iss"}}"}`,
			checkReq: func(t *testing.T, r *http.Request, body []byte) {
				if !strings.Contains(string(body), "https://transmitter.example.com") {
					t.Errorf("body %q does not contain expected issuer", body)
				}
			},
		},
		{
			name:         "server returns 2xx no error",
			serverStatus: http.StatusAccepted,
			rawToken:     "header.payload.sig",
		},
		{
			name:         "server returns 4xx returns error",
			serverStatus: http.StatusUnauthorized,
			rawToken:     "header.payload.sig",
			wantErr:      true,
		},
		{
			name:         "server returns 5xx returns error",
			serverStatus: http.StatusInternalServerError,
			rawToken:     "header.payload.sig",
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var capturedReq *http.Request
			var capturedBody []byte

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedReq = r
				capturedBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(tc.serverStatus)
			}))
			defer server.Close()

			rawToken := tc.rawToken
			if rawToken == "" {
				rawToken = fakeJWT(t, map[string]any{
					"iss": "https://transmitter.example.com",
					"jti": "test-id",
				})
			}

			ws, err := NewWebhookSink(server.URL, tc.sinkHeaders, tc.bodyTemplate)
			if err != nil {
				t.Fatalf("NewWebhookSink: %v", err)
			}

			incomingHeaders := http.Header{}
			if tc.incomingCT != "" {
				incomingHeaders.Set("Content-Type", tc.incomingCT)
			}

			// Act
			err = ws.Send(context.Background(), []byte(rawToken), incomingHeaders)

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.checkReq != nil {
				tc.checkReq(t, capturedReq, capturedBody)
			}
		})
	}
}

func TestWebhookSink_Send_UnreachableServer(t *testing.T) {
	// Arrange
	ws, err := NewWebhookSink("http://127.0.0.1:1", nil, "")
	if err != nil {
		t.Fatalf("NewWebhookSink: %v", err)
	}

	// Act
	err = ws.Send(context.Background(), []byte("token"), http.Header{})

	// Assert
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}
