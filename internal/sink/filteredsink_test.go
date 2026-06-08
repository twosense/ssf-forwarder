package sink

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"testing"
)

// recordingHandler is a slog.Handler that captures emitted records.
type recordingHandler struct {
	level   slog.Level
	records []slog.Record
}

func newRecordingLogger(level slog.Level) (*slog.Logger, *recordingHandler) {
	h := &recordingHandler{level: level}
	return slog.New(h), h
}

func (h *recordingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}

func (h *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Minimal: return self (attrs not needed for these tests).
	return h
}

func (h *recordingHandler) WithGroup(name string) slog.Handler {
	return h
}

// attrValue returns the value of the named attribute in a record, or the zero Value.
func attrValue(r slog.Record, key string) slog.Value {
	var found slog.Value
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value
			return false
		}
		return true
	})
	return found
}

// fakeInnerSink records whether Send was called and what token it received.
type fakeInnerSink struct {
	called   bool
	received []byte
}

func (f *fakeInnerSink) Send(_ context.Context, rawToken []byte, _ http.Header) error {
	f.called = true
	f.received = rawToken
	return nil
}

// riskLevelChangeEventClaims builds a minimal SET claims map with the given event URI.
func singleEventClaims(eventTypeURI string) map[string]any {
	return map[string]any{
		"jti": "test-jti-001",
		"iss": "https://idp.example.com/",
		"events": map[string]any{
			eventTypeURI: map[string]any{"current_level": "HIGH"},
		},
	}
}

func multiEventClaims() map[string]any {
	return map[string]any{
		"jti": "test-jti-multi",
		"iss": "https://idp.example.com/",
		"events": map[string]any{
			riskLevelChangeURI: map[string]any{"current_level": "HIGH"},
			sessionRevokedURI:  map[string]any{},
		},
	}
}

// stringTypedClaims returns claims where "n" is a string, causing a runtime error
// when evaluated against a numeric comparison filter.
func stringTypedClaims() map[string]any {
	return map[string]any{
		"jti": "test-jti-err",
		"n":   "notanumber",
		"events": map[string]any{
			riskLevelChangeURI: map[string]any{},
		},
	}
}

func TestFilteredSink_FiltersPass_InnerCalled(t *testing.T) {
	inner := &fakeInnerSink{}
	filters, err := CompileFilters([]string{`event_type == "` + riskLevelChangeURI + `"`})
	if err != nil {
		t.Fatalf("CompileFilters: %v", err)
	}
	logger, rec := newRecordingLogger(slog.LevelDebug)
	fs := NewFilteredSink(inner, filters, "sinks[0] (webhook https://example.com)", logger)

	token := fakeJWT(t, singleEventClaims(riskLevelChangeURI))
	err = fs.Send(context.Background(), []byte(token), http.Header{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called {
		t.Error("expected inner.Send to be called when filter passes")
	}
	if string(inner.received) != token {
		t.Errorf("inner received wrong token")
	}
	// No drop log should be emitted.
	for _, r := range rec.records {
		if r.Message == "dropped SET: filter did not match" {
			t.Error("unexpected drop log when filter passes")
		}
	}
}

func TestFilteredSink_FiltersFail_InnerNotCalled(t *testing.T) {
	inner := &fakeInnerSink{}
	filterExpr := `event_type == "` + sessionRevokedURI + `"`
	filters, err := CompileFilters([]string{filterExpr})
	if err != nil {
		t.Fatalf("CompileFilters: %v", err)
	}
	logger, rec := newRecordingLogger(slog.LevelDebug)
	fs := NewFilteredSink(inner, filters, "sinks[0] (webhook https://example.com)", logger)

	token := fakeJWT(t, singleEventClaims(riskLevelChangeURI)) // event_type won't match
	err = fs.Send(context.Background(), []byte(token), http.Header{})

	if err != nil {
		t.Fatalf("Send returned unexpected error: %v", err)
	}
	if inner.called {
		t.Error("expected inner.Send NOT to be called when filter does not match")
	}
	// A debug drop record should be emitted with the failed expression.
	var dropRecord *slog.Record
	for i := range rec.records {
		if rec.records[i].Message == "dropped SET: filter did not match" {
			dropRecord = &rec.records[i]
			break
		}
	}
	if dropRecord == nil {
		t.Fatal("expected a debug drop log record")
	}
	if dropRecord.Level != slog.LevelDebug {
		t.Errorf("expected debug level, got %v", dropRecord.Level)
	}
	filterAttr := attrValue(*dropRecord, "filter")
	if filterAttr.String() != filterExpr {
		t.Errorf("expected filter attr %q, got %q", filterExpr, filterAttr.String())
	}
}

func TestFilteredSink_MultiEvent_WarnLogged(t *testing.T) {
	inner := &fakeInnerSink{}
	// A filter that cannot resolve event_type (no single event) — it will fail.
	filters, err := CompileFilters([]string{`event_type == "` + riskLevelChangeURI + `"`})
	if err != nil {
		t.Fatalf("CompileFilters: %v", err)
	}
	logger, rec := newRecordingLogger(slog.LevelDebug)
	fs := NewFilteredSink(inner, filters, "sinks[0] (log)", logger)

	token := fakeJWT(t, multiEventClaims())
	_ = fs.Send(context.Background(), []byte(token), http.Header{})

	var warnRecord *slog.Record
	for i := range rec.records {
		if rec.records[i].Level == slog.LevelWarn {
			warnRecord = &rec.records[i]
			break
		}
	}
	if warnRecord == nil {
		t.Fatal("expected a warn log record for multi-event SET")
	}
	if warnRecord.Message != "filtered sink received SET with multiple events; event/event_type unavailable" {
		t.Errorf("unexpected warn message: %q", warnRecord.Message)
	}
}

func TestFilteredSink_RuntimeError_InnerNotCalled(t *testing.T) {
	inner := &fakeInnerSink{}
	// "claims.n > 0" compiles fine but errors at runtime when n is a string.
	filterExpr := `claims.n > 0`
	filters, err := CompileFilters([]string{filterExpr})
	if err != nil {
		t.Fatalf("CompileFilters: %v", err)
	}
	logger, rec := newRecordingLogger(slog.LevelDebug)
	fs := NewFilteredSink(inner, filters, "sinks[0] (webhook https://example.com)", logger)

	token := fakeJWT(t, stringTypedClaims())
	err = fs.Send(context.Background(), []byte(token), http.Header{})

	if err != nil {
		t.Fatalf("Send returned unexpected error (should return nil on eval error): %v", err)
	}
	if inner.called {
		t.Error("expected inner.Send NOT to be called on runtime eval error")
	}
	var errRecord *slog.Record
	for i := range rec.records {
		if rec.records[i].Level == slog.LevelError {
			errRecord = &rec.records[i]
			break
		}
	}
	if errRecord == nil {
		t.Fatal("expected an error log record for runtime eval error")
	}
	if errRecord.Message != "filter evaluation error" {
		t.Errorf("unexpected error message: %q", errRecord.Message)
	}
	filterAttr := attrValue(*errRecord, "filter")
	if filterAttr.String() != filterExpr {
		t.Errorf("expected filter attr %q, got %q", filterExpr, filterAttr.String())
	}
	errAttr := attrValue(*errRecord, "err")
	if errAttr.String() == "" {
		t.Error("expected err attr to be set in error record")
	}
}

func TestFilteredSink_MissingField_WarnLogged(t *testing.T) {
	inner := &fakeInnerSink{}
	// Filter with a typo — "current_levl" instead of "current_level".
	filterExpr := `event.current_levl == "HIGH"`
	filters, err := CompileFilters([]string{filterExpr})
	if err != nil {
		t.Fatalf("CompileFilters: %v", err)
	}
	logger, rec := newRecordingLogger(slog.LevelDebug)
	fs := NewFilteredSink(inner, filters, "sinks[0] (webhook https://example.com)", logger)

	token := fakeJWT(t, singleEventClaims(riskLevelChangeURI))
	_ = fs.Send(context.Background(), []byte(token), http.Header{})

	var warnRecord *slog.Record
	for i := range rec.records {
		if rec.records[i].Level == slog.LevelWarn &&
			rec.records[i].Message == "filter referenced field(s) not present in SET" {
			warnRecord = &rec.records[i]
			break
		}
	}
	if warnRecord == nil {
		t.Fatal("expected a warn log record for missing field reference")
	}

	sinkAttr := attrValue(*warnRecord, "sink")
	if sinkAttr.String() != "sinks[0] (webhook https://example.com)" {
		t.Errorf("unexpected sink attr: %q", sinkAttr.String())
	}

	jtiAttr := attrValue(*warnRecord, "jti")
	if jtiAttr.String() == "" {
		t.Error("expected jti attr to be set in warn record")
	}

	// "fields" attr should contain the missing path.
	fieldsAttr := attrValue(*warnRecord, "fields")
	fields, ok := fieldsAttr.Any().([]string)
	if !ok {
		t.Fatalf("expected fields attr to be []string, got %T (%v)", fieldsAttr.Any(), fieldsAttr)
	}
	if !slices.Contains(fields, "event.current_levl") {
		t.Errorf("expected fields to contain %q, got %v", "event.current_levl", fields)
	}
}
