package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/token"
	"github.com/twosense/ssf-forwarder/internal/sink"
)

// mockParser lets tests control whether SET validation succeeds or fails.
type mockParser struct {
	err error
}

func (m *mockParser) ParseSecEvent(_ string) (*token.SecEvent, error) {
	return nil, m.err
}

// recordingSink captures tokens it receives for later inspection.
// It signals on ch after each Send so tests can wait without polling.
type recordingSink struct {
	mu     sync.Mutex
	tokens [][]byte
	ch     chan struct{}
}

func newRecordingSink() *recordingSink {
	return &recordingSink{ch: make(chan struct{}, 100)}
}

func (s *recordingSink) Send(_ context.Context, rawToken []byte, _ http.Header) error {
	s.mu.Lock()
	cp := make([]byte, len(rawToken))
	copy(cp, rawToken)
	s.tokens = append(s.tokens, cp)
	s.mu.Unlock()

	s.ch <- struct{}{}

	return nil
}

func (s *recordingSink) received() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.tokens
}

// waitFor blocks until n tokens have been received or the test times out.
func (s *recordingSink) waitFor(t *testing.T, n int) {
	t.Helper()

	for range n {
		select {
		case <-s.ch:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for sink to receive token")
		}
	}
}

// errorSink always returns an error from Send.
type errorSink struct {
	ch chan struct{}
}

func newErrorSink() *errorSink {
	return &errorSink{ch: make(chan struct{}, 1)}
}

func (e *errorSink) Send(_ context.Context, _ []byte, _ http.Header) error {
	e.ch <- struct{}{}
	return errors.New("sink unavailable")
}

func (e *errorSink) waitForCall(t *testing.T) {
	t.Helper()

	select {
	case <-e.ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error sink to be called")
	}
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		parseErr   error
		wantStatus int
		wantSent   bool
	}{
		{
			name:       "GET rejected with 405",
			method:     http.MethodGet,
			body:       "some.token.here",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "PUT rejected with 405",
			method:     http.MethodPut,
			body:       "some.token.here",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "empty body returns 400",
			method:     http.MethodPost,
			body:       "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid SET returns 400",
			method:     http.MethodPost,
			body:       "bad.token.value",
			parseErr:   errors.New("signature verification failed"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "body exceeding size limit returns 413",
			method:     http.MethodPost,
			body:       string(make([]byte, maxBodySize+1)),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "valid SET returns 202 and forwards to sink",
			method:     http.MethodPost,
			body:       "valid.token.value",
			wantStatus: http.StatusAccepted,
			wantSent:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			rs := newRecordingSink()
			h := New(&mockParser{err: tc.parseErr}, []sink.Sink{rs})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, "/events", strings.NewReader(tc.body))

			// Act
			h.ServeHTTP(w, r)

			// Assert
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}

			if tc.wantSent {
				rs.waitFor(t, 1)
				got := rs.received()
				if len(got) != 1 {
					t.Fatalf("sink received %d tokens, want 1", len(got))
				}
				if string(got[0]) != tc.body {
					t.Errorf("sink got %q, want %q", got[0], tc.body)
				}
			} else {
				got := rs.received()
				if len(got) != 0 {
					t.Errorf("sink should not have received any tokens, got %d", len(got))
				}
			}
		})
	}
}

func TestHandler_FanOut_MultipleSinks(t *testing.T) {
	// Arrange
	sinkA := newRecordingSink()
	sinkB := newRecordingSink()
	h := New(&mockParser{}, []sink.Sink{sinkA, sinkB})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader("valid.token.value"))

	// Act
	h.ServeHTTP(w, r)

	// Assert
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	sinkA.waitFor(t, 1)
	sinkB.waitFor(t, 1)
	if len(sinkA.received()) != 1 {
		t.Errorf("sinkA received %d tokens, want 1", len(sinkA.received()))
	}
	if len(sinkB.received()) != 1 {
		t.Errorf("sinkB received %d tokens, want 1", len(sinkB.received()))
	}
}

func TestHandler_SinkError_DoesNotAffectResponse(t *testing.T) {
	// Arrange: parser succeeds but sink always errors
	es := newErrorSink()
	h := New(&mockParser{}, []sink.Sink{es})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader("valid.token.value"))

	// Act
	h.ServeHTTP(w, r)

	// Assert: 202 is still returned; sink errors are logged, not surfaced to the transmitter
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
	es.waitForCall(t)
}
