package sink

import (
	"slices"
	"strings"
	"testing"
)

// riskLevelChangeClaims builds a decoded SET claims map for a risk-level-change event.
func riskLevelChangeClaims(currentLevel string) map[string]any {
	return map[string]any{
		"iss": "https://idp.example.com/3456789/",
		"jti": "07efd930f0977e4fcc1149a733ce7f78",
		"iat": float64(1615305159),
		"aud": "https://sp.example2.net/caep",
		"sub_id": map[string]any{
			"format": "iss_sub",
			"iss":    "https://idp.example.com/3456789/",
			"sub":    "jane@example.com",
		},
		"events": map[string]any{
			"https://schemas.openid.net/secevent/caep/event-type/risk-level-change": map[string]any{
				"current_level":  currentLevel,
				"previous_level": "LOW",
				"principal":      "USER",
			},
		},
	}
}

const riskLevelChangeURI = "https://schemas.openid.net/secevent/caep/event-type/risk-level-change"
const sessionRevokedURI = "https://schemas.openid.net/secevent/caep/event-type/session-revoked"

func TestCompileFilters_MalformedExpression(t *testing.T) {
	_, err := CompileFilters([]string{"event_type =="})
	if err == nil {
		t.Fatal("expected error for malformed expression, got nil")
	}
	if !strings.Contains(err.Error(), "filters[0]") {
		t.Errorf("expected error to mention filters[0], got: %v", err)
	}
}

// event_type is typed as string in the compile-time env, so using it alone fails AsBool.
func TestCompileFilters_NonBooleanExpression(t *testing.T) {
	_, err := CompileFilters([]string{"event_type"})
	if err == nil {
		t.Fatal("expected error for non-boolean expression, got nil")
	}
	if !strings.Contains(err.Error(), "filters[0]") {
		t.Errorf("expected error to mention filters[0], got: %v", err)
	}
}

func TestCompileFilters_NilAndEmpty(t *testing.T) {
	for _, exprs := range [][]string{nil, {}} {
		label := "nil"
		if exprs != nil {
			label = "empty"
		}
		t.Run(label, func(t *testing.T) {
			f, err := CompileFilters(exprs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			result := f.Evaluate(riskLevelChangeClaims("HIGH"))
			if !result.Forward {
				t.Error("expected Forward=true for empty filters")
			}
			if result.Err != nil {
				t.Errorf("expected no error, got: %v", result.Err)
			}
		})
	}
}

func TestEvaluate_EventTypeFilter(t *testing.T) {
	f, err := CompileFilters([]string{`event_type == "` + riskLevelChangeURI + `"`})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	t.Run("matching event type", func(t *testing.T) {
		result := f.Evaluate(riskLevelChangeClaims("HIGH"))
		if !result.Forward {
			t.Error("expected Forward=true for matching event type")
		}
	})

	t.Run("non-matching event type", func(t *testing.T) {
		claims := map[string]any{
			"events": map[string]any{
				sessionRevokedURI: map[string]any{},
			},
		}
		result := f.Evaluate(claims)
		if result.Forward {
			t.Error("expected Forward=false for non-matching event type")
		}
	})
}

func TestEvaluate_EventFieldFilter(t *testing.T) {
	f, err := CompileFilters([]string{`event.current_level == "HIGH"`})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	t.Run("HIGH forwards", func(t *testing.T) {
		result := f.Evaluate(riskLevelChangeClaims("HIGH"))
		if !result.Forward {
			t.Error("expected Forward=true for HIGH level")
		}
	})

	t.Run("LOW drops", func(t *testing.T) {
		result := f.Evaluate(riskLevelChangeClaims("LOW"))
		if result.Forward {
			t.Error("expected Forward=false for LOW level")
		}
		if result.FailedExpression == "" {
			t.Error("expected FailedExpression to be set")
		}
	})
}

func TestEvaluate_ANDShortCircuit(t *testing.T) {
	// Both filters fail for the given SET. The first failing filter should be reported.
	firstFailing := `event_type == "` + sessionRevokedURI + `"` // will fail (it's a risk-level-change)
	secondFailing := `event.current_level == "LOW"`             // will also fail (it's HIGH)

	f, err := CompileFilters([]string{firstFailing, secondFailing})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	result := f.Evaluate(riskLevelChangeClaims("HIGH"))
	if result.Forward {
		t.Error("expected Forward=false when filters don't match")
	}
	if result.FailedExpression != firstFailing {
		t.Errorf("expected FailedExpression=%q, got %q", firstFailing, result.FailedExpression)
	}
}

func TestEvaluate_ClaimsFilter(t *testing.T) {
	f, err := CompileFilters([]string{`claims.iss == "https://idp.example.com/3456789/"`})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	t.Run("matching iss", func(t *testing.T) {
		result := f.Evaluate(riskLevelChangeClaims("HIGH"))
		if !result.Forward {
			t.Error("expected Forward=true for matching iss")
		}
	})

	t.Run("non-matching iss", func(t *testing.T) {
		claims := riskLevelChangeClaims("HIGH")
		claims["iss"] = "https://other.example.com/"
		result := f.Evaluate(claims)
		if result.Forward {
			t.Error("expected Forward=false for non-matching iss")
		}
	})
}

func TestEvaluate_EmptyFilters_AlwaysForward(t *testing.T) {
	f, err := CompileFilters([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := f.Evaluate(riskLevelChangeClaims("HIGH"))
	if !result.Forward {
		t.Error("expected Forward=true for empty filters")
	}
}

func TestEvaluate_MultiEventSET(t *testing.T) {
	f, err := CompileFilters([]string{`event.current_level == "HIGH"`})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	claims := map[string]any{
		"events": map[string]any{
			riskLevelChangeURI: map[string]any{"current_level": "HIGH"},
			sessionRevokedURI:  map[string]any{},
		},
	}

	result := f.Evaluate(claims)
	if result.Forward {
		t.Error("expected Forward=false for multi-event SET with unavailable event field")
	}
	if !result.MultiEvent {
		t.Error("expected MultiEvent=true for SET with multiple events")
	}
}

// Uses a filter that compares a string-typed field with > against a number, which
// compiles fine (map environment types are any) but errors at runtime.
func TestEvaluate_FailClosed_RuntimeError(t *testing.T) {
	// "n" is a string in claims, comparing it with > 0 (int) causes a runtime type error.
	f, err := CompileFilters([]string{`claims.n > 0`})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	claims := map[string]any{
		"n": "notanumber",
		"events": map[string]any{
			riskLevelChangeURI: map[string]any{},
		},
	}

	result := f.Evaluate(claims)
	if result.Forward {
		t.Error("expected Forward=false on runtime error (fail-closed)")
	}
	if result.Err == nil {
		t.Error("expected Err to be set on runtime error")
	}
	if result.FailedExpression == "" {
		t.Error("expected FailedExpression to be set on runtime error")
	}
}

// TestExtractFieldPaths verifies compile-time path extraction: only maximal
// event/claims-rooted member chains are kept, bare identifiers are ignored.
func TestExtractFieldPaths(t *testing.T) {
	paths := extractFieldPaths(`event.current_level && claims.sub_id.sub && event_type == "x"`)
	wantPaths := []string{"claims.sub_id.sub", "event.current_level"}
	// Sort both for stable comparison.
	slices.Sort(paths)
	slices.Sort(wantPaths)
	if !slices.Equal(paths, wantPaths) {
		t.Errorf("extractFieldPaths: got %v, want %v", paths, wantPaths)
	}
}

func TestMissingPaths_TypoInEventField(t *testing.T) {
	f, err := CompileFilters([]string{`event.current_levl == "HIGH"`}) // typo
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	result := f.Evaluate(riskLevelChangeClaims("HIGH"))
	if !slices.Contains(result.MissingPaths, "event.current_levl") {
		t.Errorf("expected MissingPaths to contain %q, got %v", "event.current_levl", result.MissingPaths)
	}
}

func TestMissingPaths_CorrectEventField_NotReported(t *testing.T) {
	f, err := CompileFilters([]string{`event.current_level == "HIGH"`})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	result := f.Evaluate(riskLevelChangeClaims("HIGH"))
	if len(result.MissingPaths) != 0 {
		t.Errorf("expected empty MissingPaths for correct field, got %v", result.MissingPaths)
	}
}

func TestMissingPaths_MissingClaimsField(t *testing.T) {
	tests := []struct {
		name        string
		filter      string
		wantMissing string
		wantPresent string
	}{
		{
			name:        "missing top-level claim",
			filter:      `claims.nonexistent == "x"`,
			wantMissing: "claims.nonexistent",
		},
		{
			name:        "present nested path not reported",
			filter:      `claims.sub_id.sub == "jane@example.com"`,
			wantPresent: "claims.sub_id.sub",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := CompileFilters([]string{tc.filter})
			if err != nil {
				t.Fatalf("unexpected compile error: %v", err)
			}
			result := f.Evaluate(riskLevelChangeClaims("HIGH"))
			if tc.wantMissing != "" && !slices.Contains(result.MissingPaths, tc.wantMissing) {
				t.Errorf("expected MissingPaths to contain %q, got %v", tc.wantMissing, result.MissingPaths)
			}
			if tc.wantPresent != "" && slices.Contains(result.MissingPaths, tc.wantPresent) {
				t.Errorf("expected MissingPaths NOT to contain %q (field is present), got %v", tc.wantPresent, result.MissingPaths)
			}
		})
	}
}

func TestMissingPaths_ShortCircuit_SkipsUnevaluatedFilter(t *testing.T) {
	// First filter fails (wrong event_type), second filter has typo but never runs.
	firstFailing := `event_type == "` + sessionRevokedURI + `"` // will fail (it's a risk-level-change)
	secondWithTypo := `event.typo == "x"`

	f, err := CompileFilters([]string{firstFailing, secondWithTypo})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	result := f.Evaluate(riskLevelChangeClaims("HIGH"))
	if result.Forward {
		t.Error("expected Forward=false")
	}
	if slices.Contains(result.MissingPaths, "event.typo") {
		t.Errorf("MissingPaths should not contain %q from unevaluated filter, got %v", "event.typo", result.MissingPaths)
	}
}

func TestMissingPaths_MultiEvent_SuppressesEventPaths(t *testing.T) {
	f, err := CompileFilters([]string{`event.current_level == "HIGH"`})
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}

	claims := map[string]any{
		"events": map[string]any{
			riskLevelChangeURI: map[string]any{"current_level": "HIGH"},
			sessionRevokedURI:  map[string]any{},
		},
	}

	result := f.Evaluate(claims)
	if !result.MultiEvent {
		t.Error("expected MultiEvent=true")
	}
	for _, p := range result.MissingPaths {
		if strings.HasPrefix(p, "event.") || strings.HasPrefix(p, "event_type") {
			t.Errorf("MissingPaths should not contain event-rooted path %q in multi-event SET, got %v", p, result.MissingPaths)
		}
	}
}
