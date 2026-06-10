//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

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
