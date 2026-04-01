package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/token"
	"github.com/twosense/ssf-forwarder/internal/sink"
)

// setParser validates an incoming SET token string.
type setParser interface {
	ParseSecEvent(tokenString string) (*token.SecEvent, error)
}

// Handler is an http.Handler that receives push-delivered SETs, validates them,
// and fans out to all configured sinks.
type Handler struct {
	parser setParser
	sinks  []sink.Sink
}

func New(p setParser, sinks []sink.Sink) *Handler {
	return &Handler{
		parser: p,
		sinks:  sinks,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawToken, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("reading request body", "err", err)
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	if len(rawToken) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	if _, err := h.parser.ParseSecEvent(string(rawToken)); err != nil {
		slog.Warn("SET validation failed", "err", err)
		http.Error(w, "invalid SET", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)

	h.fanOut(r.Context(), rawToken, r.Header)
}

func (h *Handler) fanOut(ctx context.Context, rawToken []byte, headers http.Header) {
	var wg sync.WaitGroup

	for _, s := range h.sinks {
		wg.Add(1)

		go func(s sink.Sink) {
			defer wg.Done()

			if err := s.Send(ctx, rawToken, headers); err != nil {
				slog.Error("sink send failed", "err", err)
			}
		}(s)
	}

	wg.Wait()
}
