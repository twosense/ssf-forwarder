package caepext

import (
	"encoding/json"
	"testing"
)

func TestSessionPresentedRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{
			name:  "empty",
			input: map[string]any{},
		},
		{
			name: "all optional fields",
			input: map[string]any{
				"fp_ua":           "abb0b6e7da81a42233f8f2b1a8ddb1b9a4c81611",
				"ext_id":          "12345",
				"event_timestamp": int64(1615304991),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("marshaling input: %v", err)
			}

			var e SessionPresentedEvent
			if err := json.Unmarshal(data, &e); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}

			if e.Type() != EventTypeSessionPresented {
				t.Errorf("Type() = %q, want %q", e.Type(), EventTypeSessionPresented)
			}

			if err := e.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}

			if fp, ok := tt.input["fp_ua"].(string); ok && e.FpUA != fp {
				t.Errorf("FpUA = %q, want %q", e.FpUA, fp)
			}
			if extID, ok := tt.input["ext_id"].(string); ok && e.ExtID != extID {
				t.Errorf("ExtID = %q, want %q", e.ExtID, extID)
			}
		})
	}
}

func TestSessionPresentedInvalidMetadata(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"event_timestamp": int64(-1)})
	var e SessionPresentedEvent
	if err := json.Unmarshal(data, &e); err == nil {
		t.Error("expected error for negative event_timestamp, got nil")
	}
}

func TestSessionPresentedParser(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"fp_ua":           "abc123",
		"event_timestamp": int64(1615304991),
	})

	e, err := parseSessionPresentedEvent(data)
	if err != nil {
		t.Fatalf("parseSessionPresentedEvent: %v", err)
	}
	if e.Type() != EventTypeSessionPresented {
		t.Errorf("Type() = %q, want %q", e.Type(), EventTypeSessionPresented)
	}
}
