package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Drop the timestamp so output is deterministic.
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}

func TestLogSink_Send(t *testing.T) {
	tests := []struct {
		name      string
		rawToken  string
		wantAttrs map[string]string // key → substring of expected value
	}{
		{
			name: "logs issuer and jti from valid JWT claims",
			rawToken: fakeJWT(t, map[string]any{
				"iss": "https://transmitter.example.com",
				"jti": "abc-123",
				"iat": 1700000000,
			}),
			wantAttrs: map[string]string{
				"issuer": "https://transmitter.example.com",
				"jti":    "abc-123",
			},
		},
		{
			name: "logs event types when present",
			rawToken: fakeJWT(t, map[string]any{
				"iss": "https://transmitter.example.com",
				"jti": "xyz-456",
				"iat": 1700000000,
				"events": map[string]any{
					"https://schemas.openid.net/secevent/caep/event-type/session-revoked": map[string]any{},
				},
			}),
			wantAttrs: map[string]string{
				"event_types": "session-revoked",
			},
		},
		{
			name: "logs txn when present",
			rawToken: fakeJWT(t, map[string]any{
				"iss": "https://transmitter.example.com",
				"jti": "def-789",
				"iat": 1700000000,
				"txn": "txn-id-42",
			}),
			wantAttrs: map[string]string{
				"txn": "txn-id-42",
			},
		},
		{
			name:     "does not error on malformed token",
			rawToken: "not-a-jwt",
		},
		{
			name:     "does not error on empty token",
			rawToken: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer
			ls := NewLogSink(newTestLogger(&buf))

			// Act
			err := ls.Send(context.Background(), []byte(tc.rawToken), http.Header{})

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			output := buf.String()
			for key, wantSubstr := range tc.wantAttrs {
				if !strings.Contains(output, key) {
					t.Errorf("output missing key %q: %s", key, output)
				}
				if !strings.Contains(output, wantSubstr) {
					t.Errorf("output missing value %q for key %q: %s", wantSubstr, key, output)
				}
			}
		})
	}
}

func TestLogSink_Send_AlwaysReturnsNil(t *testing.T) {
	// Arrange: send a garbage token — the sink should never return an error
	var buf bytes.Buffer
	ls := NewLogSink(newTestLogger(&buf))

	tokens := [][]byte{
		[]byte(""),
		[]byte("not.a.jwt"),
		[]byte("a.b"), // only two parts
		func() []byte {
			// valid JWT shape but payload is not valid base64
			return []byte("header.!!!.sig")
		}(),
	}

	for _, tok := range tokens {
		// Act
		err := ls.Send(context.Background(), tok, http.Header{})

		// Assert
		if err != nil {
			t.Errorf("Send(%q) returned error: %v", tok, err)
		}
	}
}

// TestLogSink_EventTypes_Sorted verifies that multiple event types are logged in
// a stable order so output is predictable.
func TestLogSink_EventTypes_Sorted(t *testing.T) {
	// Arrange
	claims := map[string]any{
		"iss": "https://transmitter.example.com",
		"jti": "sort-test",
		"iat": 1700000000,
		"events": map[string]any{
			"urn:z": map[string]any{},
			"urn:a": map[string]any{},
			"urn:m": map[string]any{},
		},
	}
	payloadJSON, _ := json.Marshal(claims)
	_ = payloadJSON // used via fakeJWT helper

	var buf bytes.Buffer
	ls := NewLogSink(newTestLogger(&buf))

	// Act
	err := ls.Send(context.Background(), []byte(fakeJWT(t, claims)), http.Header{})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	posA := strings.Index(output, "urn:a")
	posM := strings.Index(output, "urn:m")
	posZ := strings.Index(output, "urn:z")
	if posA < 0 || posM < 0 || posZ < 0 {
		t.Fatalf("not all event types found in output: %s", output)
	}
	if !(posA < posM && posM < posZ) {
		t.Errorf("event types not sorted in output: %s", output)
	}
}
