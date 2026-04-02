package sink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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
			ws, err := NewWebhookSink("https://example.com", nil, tc.bodyTemplate, discardLogger())

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
		noRetry      bool
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
			noRetry:      true,
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

			ws, err := NewWebhookSink(server.URL, tc.sinkHeaders, tc.bodyTemplate, discardLogger())
			if err != nil {
				t.Fatalf("NewWebhookSink: %v", err)
			}
			if tc.noRetry {
				ws.retry.MaxRetries = 0
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

func noopSleep(_ context.Context, _ time.Duration) error { return nil }

func TestWebhookSink_Retry_CoreBehavior(t *testing.T) {
	tests := []struct {
		name       string
		responses  []int // sequence of HTTP status codes; -1 means network error
		maxRetries int
		wantErr    bool
		wantReqs   int // expected total attempts made
	}{
		{
			name:       "retryable status eventually succeeds",
			responses:  []int{503, 503, 200},
			maxRetries: 3,
			wantReqs:   3,
		},
		{
			name:       "retryable status exhausts all retries",
			responses:  []int{503, 503, 503, 503},
			maxRetries: 3,
			wantErr:    true,
			wantReqs:   4,
		},
		{
			name:       "network error eventually succeeds",
			responses:  []int{-1, -1, 200},
			maxRetries: 3,
			wantReqs:   3,
		},
		{
			name:       "network error exhausts all retries",
			responses:  []int{-1, -1, -1, -1},
			maxRetries: 3,
			wantErr:    true,
			wantReqs:   4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			transport := &sequencedTransport{responses: tc.responses}

			ws, err := NewWebhookSink("http://example.com", nil, "", discardLogger())
			if err != nil {
				t.Fatalf("NewWebhookSink: %v", err)
			}
			ws.retry.MaxRetries = tc.maxRetries
			ws.sleep = noopSleep
			ws.client = &http.Client{Transport: transport}

			// Act
			err = ws.Send(context.Background(), []byte("token"), http.Header{})

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if transport.attempts != tc.wantReqs {
				t.Errorf("made %d attempts, want %d", transport.attempts, tc.wantReqs)
			}
		})
	}
}

// sequencedTransport returns responses in order without a real server.
// A -1 entry causes a network error; any other value is returned as an HTTP status code.
type sequencedTransport struct {
	responses []int
	attempts  int
}

func (t *sequencedTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	if t.attempts >= len(t.responses) {
		return nil, fmt.Errorf("sequencedTransport: no response configured for attempt %d", t.attempts)
	}
	status := t.responses[t.attempts]
	t.attempts++
	if status == -1 {
		return nil, fmt.Errorf("simulated network error")
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func TestWebhookSink_Retry_Boundaries(t *testing.T) {
	tests := []struct {
		name        string
		responses   []int
		maxRetries  int
		wantErr     bool
		wantReqs    int
		wantSleeps  int
	}{
		{
			name:       "succeeds on last allowed attempt",
			responses:  []int{503, 503, 503, 200},
			maxRetries: 3,
			wantReqs:   4,
			wantSleeps: 3,
		},
		{
			name:       "zero max retries makes exactly one attempt",
			responses:  []int{200},
			maxRetries: 0,
			wantReqs:   1,
			wantSleeps: 0,
		},
		{
			name:       "zero max retries fails without retrying",
			responses:  []int{503},
			maxRetries: 0,
			wantErr:    true,
			wantReqs:   1,
			wantSleeps: 0,
		},
		{
			name:       "non-retryable 4xx fails immediately",
			responses:  []int{400},
			maxRetries: 3,
			wantErr:    true,
			wantReqs:   1,
			wantSleeps: 0,
		},
		{
			name:       "non-retryable 401 fails immediately",
			responses:  []int{401},
			maxRetries: 3,
			wantErr:    true,
			wantReqs:   1,
			wantSleeps: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			transport := &sequencedTransport{responses: tc.responses}
			sleepCount := 0

			ws, err := NewWebhookSink("http://example.com", nil, "", discardLogger())
			if err != nil {
				t.Fatalf("NewWebhookSink: %v", err)
			}
			ws.retry.MaxRetries = tc.maxRetries
			ws.sleep = func(_ context.Context, _ time.Duration) error {
				sleepCount++
				return nil
			}
			ws.client = &http.Client{Transport: transport}

			// Act
			err = ws.Send(context.Background(), []byte("token"), http.Header{})

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if transport.attempts != tc.wantReqs {
				t.Errorf("made %d attempts, want %d", transport.attempts, tc.wantReqs)
			}
			if sleepCount != tc.wantSleeps {
				t.Errorf("slept %d times, want %d", sleepCount, tc.wantSleeps)
			}
		})
	}
}

func TestWebhookSink_Retry_Backoff(t *testing.T) {
	t.Run("sleep called exactly MaxRetries times on exhaustion", func(t *testing.T) {
		// Arrange
		const maxRetries = 3
		transport := &sequencedTransport{responses: []int{503, 503, 503, 503}}
		sleepCount := 0

		ws, err := NewWebhookSink("http://example.com", nil, "", discardLogger())
		if err != nil {
			t.Fatalf("NewWebhookSink: %v", err)
		}
		ws.retry.MaxRetries = maxRetries
		ws.sleep = func(_ context.Context, _ time.Duration) error {
			sleepCount++
			return nil
		}
		ws.client = &http.Client{Transport: transport}

		// Act
		ws.Send(context.Background(), []byte("token"), http.Header{})

		// Assert
		if sleepCount != maxRetries {
			t.Errorf("sleep called %d times, want %d", sleepCount, maxRetries)
		}
	})

	t.Run("sleep durations are strictly increasing", func(t *testing.T) {
		// Arrange: all retryable so we collect all sleep durations.
		const maxRetries = 3
		transport := &sequencedTransport{responses: []int{503, 503, 503, 503}}
		var durations []time.Duration

		ws, err := NewWebhookSink("http://example.com", nil, "", discardLogger())
		if err != nil {
			t.Fatalf("NewWebhookSink: %v", err)
		}
		ws.retry.MaxRetries = maxRetries
		ws.sleep = func(_ context.Context, d time.Duration) error {
			durations = append(durations, d)
			return nil
		}
		ws.client = &http.Client{Transport: transport}

		// Act
		ws.Send(context.Background(), []byte("token"), http.Header{})

		// Assert
		if len(durations) != maxRetries {
			t.Fatalf("got %d sleep durations, want %d", len(durations), maxRetries)
		}
		for i := 1; i < len(durations); i++ {
			if durations[i] <= durations[i-1] {
				t.Errorf("duration[%d]=%v <= duration[%d]=%v, want strictly increasing", i, durations[i], i-1, durations[i-1])
			}
		}
	})
}

func TestWebhookSink_Retry_ContextCancellation(t *testing.T) {
	t.Run("cancelled before first attempt", func(t *testing.T) {
		// Arrange
		transport := &sequencedTransport{responses: []int{200}}

		ws, err := NewWebhookSink("http://example.com", nil, "", discardLogger())
		if err != nil {
			t.Fatalf("NewWebhookSink: %v", err)
		}
		ws.sleep = noopSleep
		ws.client = &http.Client{Transport: transport}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Act
		err = ws.Send(ctx, []byte("token"), http.Header{})

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if transport.attempts != 0 {
			t.Errorf("made %d attempts, want 0", transport.attempts)
		}
	})

	t.Run("cancelled during sleep returns error", func(t *testing.T) {
		// Arrange: first attempt fails with a retryable status, then the context
		// is cancelled during the sleep before the second attempt.
		transport := &sequencedTransport{responses: []int{503, 200}}

		ws, err := NewWebhookSink("http://example.com", nil, "", discardLogger())
		if err != nil {
			t.Fatalf("NewWebhookSink: %v", err)
		}
		ws.retry.MaxRetries = 3

		ctx, cancel := context.WithCancel(context.Background())
		ws.sleep = func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		}
		ws.client = &http.Client{Transport: transport}

		// Act
		err = ws.Send(ctx, []byte("token"), http.Header{})

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		// Only the first attempt should have been made before sleep cancelled it.
		if transport.attempts != 1 {
			t.Errorf("made %d attempts, want 1", transport.attempts)
		}
	})
}

func TestWebhookSink_Retry_Mixed(t *testing.T) {
	tests := []struct {
		name      string
		responses []int
		wantErr   bool
		wantReqs  int
	}{
		{
			name:      "alternating network errors and retryable statuses then success",
			responses: []int{-1, 503, -1, 200},
			wantReqs:  4,
		},
		{
			name:      "alternating network errors and retryable statuses exhausts retries",
			responses: []int{-1, 503, -1, 503},
			wantErr:   true,
			wantReqs:  4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			transport := &sequencedTransport{responses: tc.responses}

			ws, err := NewWebhookSink("http://example.com", nil, "", discardLogger())
			if err != nil {
				t.Fatalf("NewWebhookSink: %v", err)
			}
			ws.retry.MaxRetries = len(tc.responses) - 1
			ws.sleep = noopSleep
			ws.client = &http.Client{Transport: transport}

			// Act
			err = ws.Send(context.Background(), []byte("token"), http.Header{})

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if transport.attempts != tc.wantReqs {
				t.Errorf("made %d attempts, want %d", transport.attempts, tc.wantReqs)
			}
		})
	}
}

func TestWebhookSink_Send_UnreachableServer(t *testing.T) {
	// Arrange
	ws, err := NewWebhookSink("http://127.0.0.1:1", nil, "", discardLogger())
	if err != nil {
		t.Fatalf("NewWebhookSink: %v", err)
	}
	ws.retry.MaxRetries = 0

	// Act
	err = ws.Send(context.Background(), []byte("token"), http.Header{})

	// Assert
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}
