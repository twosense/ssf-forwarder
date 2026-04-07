package sink

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
)

// LogSink logs information about each received SET to stdout using structured logging.
type LogSink struct {
	logger *slog.Logger
}

func NewLogSink(logger *slog.Logger) *LogSink {
	return &LogSink{logger: logger}
}

func (ls *LogSink) Send(_ context.Context, rawToken []byte, _ http.Header) error {
	claims := extractClaims(string(rawToken))

	args := []any{
		"issuer", stringClaim(claims, "iss"),
		"jti", stringClaim(claims, "jti"),
		"iat", claims["iat"],
	}

	if txn := stringClaim(claims, "txn"); txn != "" {
		args = append(args, "txn", txn)
	}

	if types := eventTypes(claims); len(types) > 0 {
		args = append(args, "event_types", types)
	}

	ls.logger.Info("received SET", args...)

	return nil
}

func stringClaim(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	v, _ := claims[key].(string)
	return v
}

func eventTypes(claims map[string]any) []string {
	if claims == nil {
		return nil
	}
	events, ok := claims["events"].(map[string]any)
	if !ok {
		return nil
	}
	types := make([]string, 0, len(events))
	for et := range events {
		types = append(types, et)
	}
	sort.Strings(types)
	return types
}
