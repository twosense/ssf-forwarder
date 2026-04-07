package sink

import (
	"context"
	"encoding/json"
	"fmt"
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

	ls.logger.Info("received SET",
		"issuer", stringClaim(claims, "iss"),
		"jti", stringClaim(claims, "jti"),
	)

	if events, ok := claims["events"].(map[string]any); ok {
		for et, data := range events {
			fmt.Printf("  event: %s\n  claims:\n%s\n", et, prettyJSON(data, "    "))
		}
	}

	if subID, ok := claims["sub_id"]; ok {
		fmt.Printf("  subject:\n%s\n", prettyJSON(subID, "    "))
	}

	return nil
}

func stringClaim(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	v, _ := claims[key].(string)
	return v
}

func prettyJSON(v any, prefix string) string {
	b, err := json.MarshalIndent(v, prefix, "  ")
	if err != nil {
		return fmt.Sprintf("%s%v", prefix, v)
	}
	return prefix + string(b)
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
