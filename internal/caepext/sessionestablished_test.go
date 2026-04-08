package caepext

import (
	"encoding/json"
	"testing"
)

func TestSessionEstablishedRoundtrip(t *testing.T) {
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
				"acr":             "AAL2",
				"amr":             []any{"otp", "pwd"},
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

			var e SessionEstablishedEvent
			if err := json.Unmarshal(data, &e); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}

			if e.Type() != EventTypeSessionEstablished {
				t.Errorf("Type() = %q, want %q", e.Type(), EventTypeSessionEstablished)
			}

			if err := e.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}

			if fp, ok := tt.input["fp_ua"].(string); ok && e.FpUA != fp {
				t.Errorf("FpUA = %q, want %q", e.FpUA, fp)
			}
			if acr, ok := tt.input["acr"].(string); ok && e.ACR != acr {
				t.Errorf("ACR = %q, want %q", e.ACR, acr)
			}
			if extID, ok := tt.input["ext_id"].(string); ok && e.ExtID != extID {
				t.Errorf("ExtID = %q, want %q", e.ExtID, extID)
			}
		})
	}
}

func TestSessionEstablishedInvalidMetadata(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"event_timestamp": int64(-1)})
	var e SessionEstablishedEvent
	if err := json.Unmarshal(data, &e); err == nil {
		t.Error("expected error for negative event_timestamp, got nil")
	}
}

func TestSessionEstablishedParser(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"fp_ua":           "abc123",
		"event_timestamp": int64(1615304991),
	})

	e, err := parseSessionEstablishedEvent(data)
	if err != nil {
		t.Fatalf("parseSessionEstablishedEvent: %v", err)
	}
	if e.Type() != EventTypeSessionEstablished {
		t.Errorf("Type() = %q, want %q", e.Type(), EventTypeSessionEstablished)
	}
}
