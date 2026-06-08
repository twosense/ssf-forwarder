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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const dockerImage = "ssf-forwarder:e2e-test"

var (
	binaryPath string
	useDocker  = os.Getenv("E2E_DOCKER") == "1"
)

func TestMain(m *testing.M) {
	var cleanup func()

	if useDocker {
		cmd := exec.Command("docker", "build", "-t", dockerImage, "../..")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "docker build failed: %v\n%s\n", err, out)
			os.Exit(1)
		}
		cleanup = func() {}
	} else {
		dir, err := os.MkdirTemp("", "ssf-forwarder-e2e-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
			os.Exit(1)
		}
		binaryPath = filepath.Join(dir, "ssf-forwarder")
		cmd := exec.Command("go", "build", "-o", binaryPath, "github.com/twosense/ssf-forwarder/cmd/ssf-forwarder")
		if out, err := cmd.CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			fmt.Fprintf(os.Stderr, "binary build failed: %v\n%s\n", err, out)
			os.Exit(1)
		}
		cleanup = func() { os.RemoveAll(dir) }
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}

// fakeTransmitter is an HTTP server that behaves like an SSF transmitter.
// It handles stream registration and can produce valid signed SETs.
type fakeTransmitter struct {
	server     *httptest.Server
	privateKey *rsa.PrivateKey
	kid        string

	registerOnce sync.Once
	registered   chan struct{}

	mu       sync.Mutex
	pushURL  string
	streamID string
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

func (ft *fakeTransmitter) serveStreams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Signal no existing stream so the forwarder always creates a new one.
		http.NotFound(w, r)

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}

		var req struct {
			Delivery struct {
				Method      string `json:"method"`
				EndpointURL string `json:"endpoint_url"`
			} `json:"delivery"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "parse body", http.StatusBadRequest)
			return
		}

		ft.mu.Lock()
		ft.pushURL = req.Delivery.EndpointURL
		ft.streamID = "e2e-stream-001"
		ft.mu.Unlock()

		resp := map[string]interface{}{
			"stream_id": "e2e-stream-001",
			"iss":       ft.issuer(),
			"aud":       req.Delivery.EndpointURL,
			"delivery": map[string]string{
				"method":       req.Delivery.Method,
				"endpoint_url": req.Delivery.EndpointURL,
			},
			"events_delivered": []string{
				"https://schemas.openid.net/secevent/ssf/event-type/verification",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)

		ft.registerOnce.Do(func() { close(ft.registered) })

	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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

// testSink is an HTTP server that records the raw bodies of all POST requests.
type testSink struct {
	server *httptest.Server
	mu     sync.Mutex
	tokens []string
	ch     chan struct{}
}

func newTestSink(t *testing.T) *testSink {
	t.Helper()

	ts := &testSink{ch: make(chan struct{}, 10)}
	ts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts.mu.Lock()
		ts.tokens = append(ts.tokens, string(body))
		ts.mu.Unlock()
		ts.ch <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.server.Close)

	return ts
}

func (ts *testSink) waitForToken(t *testing.T, timeout time.Duration) string {
	t.Helper()
	select {
	case <-ts.ch:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for token at test sink")
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.tokens[len(ts.tokens)-1]
}

func (ts *testSink) expectNoToken(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-ts.ch:
		t.Fatal("test sink received a token but expected none (SET should have been filtered out)")
	case <-time.After(timeout):
	}
}

// startForwarder launches the forwarder with the given config file and
// registers a cleanup to send SIGTERM and wait for exit.
//
// In binary mode the compiled binary is run directly. In Docker mode
// (E2E_DOCKER=1) the pre-built image is run with --network host so the
// container shares the host's network stack and can reach 127.0.0.1 services.
// Docker mode is only supported on Linux.
func startForwarder(t *testing.T, configPath string) {
	t.Helper()

	var cmd *exec.Cmd
	if useDocker {
		cmd = exec.Command("docker", "run", "--rm",
			"--network", "host",
			"-v", configPath+":/etc/ssf-forwarder/config.yaml:ro",
			dockerImage,
		)
	} else {
		cmd = exec.Command(binaryPath, "--config", configPath)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting forwarder: %v", err)
	}

	t.Cleanup(func() {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
	})
}

// waitForServer polls url until the server responds or the timeout elapses.
func waitForServer(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for server at %s", url)
}

// freePort finds and returns a free TCP port on localhost.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// writeConfig writes a forwarder config.yaml to a temp file and returns its path.
// The file is world-readable so the Docker container user can read it when mounted.
// When filters is non-empty, a filters block is appended under the webhook sink.
func writeConfig(t *testing.T, metadataURL, sinkURL, publicURL, listenAddr string, eventTypes []string, filters []string) string {
	t.Helper()

	var eventsBlock strings.Builder
	for _, et := range eventTypes {
		fmt.Fprintf(&eventsBlock, "    - %s\n", et)
	}

	var filtersBlock strings.Builder
	if len(filters) > 0 {
		filtersBlock.WriteString("    filters:\n")
		for _, f := range filters {
			fmt.Fprintf(&filtersBlock, "      - '%s'\n", f)
		}
	}

	content := fmt.Sprintf(`receiver:
  public_url: %q
  listen_addr: %q
  endpoint: /events

transmitter:
  metadata_url: %q
  auth:
    type: bearer
    token: test-token
  events_requested:
%s
sinks:
  - type: webhook
    url: %q
%s`, publicURL, listenAddr, metadataURL, eventsBlock.String(), sinkURL, filtersBlock.String())

	f, err := os.CreateTemp("", "ssf-forwarder-config-*.yaml")
	if err != nil {
		t.Fatalf("creating config temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	f.Close()

	// World-readable so the container user (uid 1000) can read the mounted file.
	if err := os.Chmod(f.Name(), 0644); err != nil {
		t.Fatalf("chmod config: %v", err)
	}

	return f.Name()
}

func TestForwardsSETToWebhookSink(t *testing.T) {
	transmitter := newFakeTransmitter(t)
	sink := newTestSink(t)

	port := freePort(t)
	listenAddr := fmt.Sprintf("127.0.0.1:%d", port)
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	cfgPath := writeConfig(t,
		transmitter.server.URL+"/metadata",
		sink.server.URL,
		publicURL,
		listenAddr,
		[]string{"https://schemas.openid.net/secevent/ssf/event-type/verification"},
		nil,
	)

	startForwarder(t, cfgPath)

	// Block until the forwarder registers its push stream with the transmitter.
	// This also confirms the forwarder started and reached the transmitter.
	transmitter.waitForRegistration(t, 15*time.Second)

	// The forwarder starts its HTTP server after stream registration,
	// so poll until it is accepting connections.
	waitForServer(t, publicURL+"/events", 5*time.Second)

	token := transmitter.signSET(t)

	resp, err := http.Post(
		transmitter.getPushURL(),
		"application/secevent+jwt",
		strings.NewReader(token),
	)
	if err != nil {
		t.Fatalf("pushing SET to forwarder: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("forwarder returned %d, want 202", resp.StatusCode)
	}

	received := sink.waitForToken(t, 5*time.Second)

	if received != token {
		t.Errorf("sink received unexpected token\ngot:  %s\nwant: %s", received, token)
	}
}

func TestForwardsRiskLevelChangeSETToWebhookSink(t *testing.T) {
	transmitter := newFakeTransmitter(t)
	sink := newTestSink(t)

	port := freePort(t)
	listenAddr := fmt.Sprintf("127.0.0.1:%d", port)
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	cfgPath := writeConfig(t,
		transmitter.server.URL+"/metadata",
		sink.server.URL,
		publicURL,
		listenAddr,
		[]string{"https://schemas.openid.net/secevent/caep/event-type/risk-level-change"},
		nil,
	)

	startForwarder(t, cfgPath)

	transmitter.waitForRegistration(t, 15*time.Second)
	waitForServer(t, publicURL+"/events", 5*time.Second)

	token := transmitter.signRiskLevelChangeSET(t, "MEDIUM")

	resp, err := http.Post(
		transmitter.getPushURL(),
		"application/secevent+jwt",
		strings.NewReader(token),
	)
	if err != nil {
		t.Fatalf("pushing SET to forwarder: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("forwarder returned %d, want 202", resp.StatusCode)
	}

	received := sink.waitForToken(t, 5*time.Second)

	if received != token {
		t.Errorf("sink received unexpected token\ngot:  %s\nwant: %s", received, token)
	}
}

func TestFilterForwardsOnlyHighRiskLevelChange(t *testing.T) {
	transmitter := newFakeTransmitter(t)
	sink := newTestSink(t)

	port := freePort(t)
	listenAddr := fmt.Sprintf("127.0.0.1:%d", port)
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	cfgPath := writeConfig(t,
		transmitter.server.URL+"/metadata",
		sink.server.URL,
		publicURL,
		listenAddr,
		[]string{"https://schemas.openid.net/secevent/caep/event-type/risk-level-change"},
		[]string{
			`event_type == "https://schemas.openid.net/secevent/caep/event-type/risk-level-change"`,
			`event.current_level == "HIGH"`,
		},
	)

	startForwarder(t, cfgPath)
	transmitter.waitForRegistration(t, 15*time.Second)
	waitForServer(t, publicURL+"/events", 5*time.Second)

	// Push a HIGH risk-level-change SET — the filter should pass and forward it.
	highToken := transmitter.signRiskLevelChangeSET(t, "HIGH")

	resp, err := http.Post(
		transmitter.getPushURL(),
		"application/secevent+jwt",
		strings.NewReader(highToken),
	)
	if err != nil {
		t.Fatalf("pushing HIGH SET to forwarder: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("forwarder returned %d, want 202 (HIGH SET)", resp.StatusCode)
	}

	received := sink.waitForToken(t, 5*time.Second)
	if received != highToken {
		t.Errorf("sink received unexpected token for HIGH SET\ngot:  %s\nwant: %s", received, highToken)
	}

	// Push a LOW risk-level-change SET — the filter should drop it (current_level != "HIGH").
	lowToken := transmitter.signRiskLevelChangeSET(t, "LOW")

	resp, err = http.Post(
		transmitter.getPushURL(),
		"application/secevent+jwt",
		strings.NewReader(lowToken),
	)
	if err != nil {
		t.Fatalf("pushing LOW SET to forwarder: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("forwarder returned %d, want 202 (LOW SET)", resp.StatusCode)
	}

	sink.expectNoToken(t, 2*time.Second)
}
