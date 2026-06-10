//go:build e2e

package e2e

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// storedStream is a push stream the fake transmitter is holding, keyed by its
// description marker so the forwarder's reconcile logic can find its own stream.
type storedStream struct {
	StreamID    string
	Description string
	PushURL     string
	Method      string
	Events      []string
}

// fakeTransmitter is an HTTP server that behaves like an SSF transmitter.
// It maintains stream state (list/create/update/delete) so the forwarder's
// stream lifecycle can be exercised end-to-end, and produces valid signed SETs.
type fakeTransmitter struct {
	server     *httptest.Server
	privateKey *rsa.PrivateKey
	kid        string

	registerOnce sync.Once
	registered   chan struct{}

	mu        sync.Mutex
	streams   map[string]storedStream
	streamSeq int
	pushURL   string // endpoint_url of the most recently created/updated stream
}

func newFakeTransmitter(t *testing.T) *fakeTransmitter {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	ft := &fakeTransmitter{
		privateKey: key,
		kid:        "e2e-test-key",
		registered: make(chan struct{}),
		streams:    make(map[string]storedStream),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metadata", ft.serveMetadata)
	mux.HandleFunc("/jwks", ft.serveJWKS)
	mux.HandleFunc("/streams", ft.serveStreams)

	ft.server = httptest.NewServer(mux)
	t.Cleanup(ft.server.Close)

	return ft
}

func (ft *fakeTransmitter) issuer() string {
	return ft.server.URL + "/"
}

func (ft *fakeTransmitter) serveMetadata(w http.ResponseWriter, r *http.Request) {
	meta := map[string]interface{}{
		"issuer":                     ft.issuer(),
		"jwks_uri":                   ft.server.URL + "/jwks",
		"configuration_endpoint":     ft.server.URL + "/streams",
		"delivery_methods_supported": []string{"urn:ietf:rfc:8935"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)
}

func (ft *fakeTransmitter) serveJWKS(w http.ResponseWriter, r *http.Request) {
	pub := &ft.privateKey.PublicKey
	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{{
			"kty": "RSA",
			"kid": ft.kid,
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jwks)
}

// streamRequest is the subset of the stream config the forwarder sends when
// creating or updating a stream.
type streamRequest struct {
	StreamID    string `json:"stream_id"`
	Description string `json:"description"`
	Delivery    struct {
		Method      string `json:"method"`
		EndpointURL string `json:"endpoint_url"`
	} `json:"delivery"`
	EventsRequested []string `json:"events_requested"`
}

func (ft *fakeTransmitter) serveStreams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ft.mu.Lock()
		list := make([]map[string]interface{}, 0, len(ft.streams))
		for _, s := range ft.streams {
			list = append(list, ft.streamJSON(s))
		}
		ft.mu.Unlock()
		ft.writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		req, ok := ft.decodeStreamRequest(w, r)
		if !ok {
			return
		}

		ft.mu.Lock()
		ft.streamSeq++
		stream := storedStream{
			StreamID:    fmt.Sprintf("e2e-stream-%03d", ft.streamSeq),
			Description: req.Description,
			PushURL:     req.Delivery.EndpointURL,
			Method:      req.Delivery.Method,
			Events:      req.EventsRequested,
		}
		ft.streams[stream.StreamID] = stream
		ft.pushURL = stream.PushURL
		ft.mu.Unlock()

		ft.writeJSON(w, http.StatusCreated, ft.streamJSON(stream))
		ft.registerOnce.Do(func() { close(ft.registered) })

	case http.MethodPut:
		req, ok := ft.decodeStreamRequest(w, r)
		if !ok {
			return
		}

		ft.mu.Lock()
		stream, found := ft.streams[req.StreamID]
		if found {
			stream.Description = req.Description
			stream.PushURL = req.Delivery.EndpointURL
			stream.Method = req.Delivery.Method
			stream.Events = req.EventsRequested
			ft.streams[stream.StreamID] = stream
			ft.pushURL = stream.PushURL
		}
		ft.mu.Unlock()

		if !found {
			http.NotFound(w, r)
			return
		}
		ft.writeJSON(w, http.StatusOK, ft.streamJSON(stream))

	case http.MethodDelete:
		streamID := r.URL.Query().Get("stream_id")
		ft.mu.Lock()
		_, found := ft.streams[streamID]
		delete(ft.streams, streamID)
		ft.mu.Unlock()

		if !found {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (ft *fakeTransmitter) decodeStreamRequest(w http.ResponseWriter, r *http.Request) (streamRequest, bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusInternalServerError)
		return streamRequest{}, false
	}
	var req streamRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse body", http.StatusBadRequest)
		return streamRequest{}, false
	}
	return req, true
}

// streamJSON renders a stored stream as the transmitter's wire representation.
// Callers must hold ft.mu (it reads issuer, which is immutable, but the stream
// fields it formats come from locked state).
func (ft *fakeTransmitter) streamJSON(s storedStream) map[string]interface{} {
	return map[string]interface{}{
		"stream_id":   s.StreamID,
		"description": s.Description,
		"iss":         ft.issuer(),
		"aud":         s.PushURL,
		"delivery": map[string]string{
			"method":       s.Method,
			"endpoint_url": s.PushURL,
		},
		"events_requested": s.Events,
		"events_delivered": s.Events,
	}
}

func (ft *fakeTransmitter) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// streamCount returns how many streams the transmitter currently holds.
func (ft *fakeTransmitter) streamCount() int {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return len(ft.streams)
}

func (ft *fakeTransmitter) waitForRegistration(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-ft.registered:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for stream registration")
	}
}

func (ft *fakeTransmitter) getPushURL() string {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.pushURL
}

// signJWT signs a JWT with the given payload and returns the token string.
func (ft *fakeTransmitter) signJWT(t *testing.T, payload map[string]interface{}) string {
	t.Helper()

	headerJSON, _ := json.Marshal(map[string]interface{}{
		"alg": "RS256",
		"kid": ft.kid,
		"typ": "JWT",
	})
	payloadJSON, _ := json.Marshal(payload)

	h := base64.RawURLEncoding.EncodeToString(headerJSON)
	p := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := h + "." + p

	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, ft.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("signing JWT: %v", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// signSET returns a signed SSF verification SET as a JWT string.
// The SET is valid for the forwarder to parse: correct issuer, a registered
// event type, and a subject.
func (ft *fakeTransmitter) signSET(t *testing.T) string {
	t.Helper()
	return ft.signJWT(t, map[string]interface{}{
		"iss": ft.issuer(),
		"jti": fmt.Sprintf("e2e-%d", time.Now().UnixNano()),
		"iat": time.Now().Unix(),
		"events": map[string]interface{}{
			"https://schemas.openid.net/secevent/ssf/event-type/verification": map[string]interface{}{},
		},
		"sub_id": map[string]interface{}{
			"format": "email",
			"email":  "test@example.com",
		},
	})
}

// signRiskLevelChangeSET returns a signed CAEP risk-level-change SET as a JWT string.
// currentLevel is used as the current_level claim (e.g. "HIGH", "MEDIUM", "LOW").
func (ft *fakeTransmitter) signRiskLevelChangeSET(t *testing.T, currentLevel string) string {
	t.Helper()
	return ft.signJWT(t, map[string]interface{}{
		"iss": ft.issuer(),
		"jti": fmt.Sprintf("e2e-%d", time.Now().UnixNano()),
		"iat": time.Now().Unix(),
		"events": map[string]interface{}{
			"https://schemas.openid.net/secevent/caep/event-type/risk-level-change": map[string]interface{}{
				"current_level":   currentLevel,
				"previous_level":  "LOW",
				"principal":       "USER",
				"event_timestamp": time.Now().UnixMilli(),
			},
		},
		"sub_id": map[string]interface{}{
			"format": "email",
			"email":  "test@example.com",
		},
	})
}
