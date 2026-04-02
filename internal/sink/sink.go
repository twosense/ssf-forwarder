package sink

import (
	"context"
	"net/http"
)

// Sink receives a validated SET and forwards it somewhere.
type Sink interface {
	Send(ctx context.Context, rawToken []byte, headers http.Header) error
}
