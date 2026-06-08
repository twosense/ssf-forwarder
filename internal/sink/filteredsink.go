package sink

import (
	"context"
	"log/slog"
	"net/http"
)

// FilteredSink wraps another Sink and forwards SETs only when the compiled
// filters allow it. It logs dropped and erroneous SETs but never returns an
// error to the caller for filter-related drops (fail closed without signaling
// a delivery failure).
type FilteredSink struct {
	inner   Sink
	filters *Filters
	label   string
	logger  *slog.Logger
}

// NewFilteredSink wraps inner, forwarding a SET only if filters.Evaluate allows it.
// label is a human-readable sink identity used in logs (e.g. "sinks[0] (webhook https://...)").
func NewFilteredSink(inner Sink, filters *Filters, label string, logger *slog.Logger) *FilteredSink {
	return &FilteredSink{
		inner:   inner,
		filters: filters,
		label:   label,
		logger:  logger,
	}
}

func (fs *FilteredSink) Send(ctx context.Context, rawToken []byte, headers http.Header) error {
	claims := extractClaims(string(rawToken))
	jti := stringClaim(claims, "jti")

	result := fs.filters.Evaluate(claims)

	if result.MultiEvent {
		fs.logger.Warn(
			"filtered sink received SET with multiple events; event/event_type unavailable",
			"sink", fs.label,
			"jti", jti,
		)
	}

	if len(result.MissingPaths) > 0 {
		fs.logger.Warn(
			"filter referenced field(s) not present in SET",
			"sink", fs.label,
			"jti", jti,
			"fields", result.MissingPaths,
		)
	}

	if result.Err != nil {
		fs.logger.Error(
			"filter evaluation error",
			"sink", fs.label,
			"jti", jti,
			"filter", result.FailedExpression,
			"err", result.Err,
		)
		return nil
	}

	if !result.Forward {
		fs.logger.Debug(
			"dropped SET: filter did not match",
			"sink", fs.label,
			"jti", jti,
			"filter", result.FailedExpression,
		)
		return nil
	}

	return fs.inner.Send(ctx, rawToken, headers)
}
