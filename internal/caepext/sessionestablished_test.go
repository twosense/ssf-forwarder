package caepext

import (
	"encoding/json"
	"testing"
)

func TestSessionEstablishedParse(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{
			name:    "empty payload",
			payload: map[string]any{},
		},
		{
			name: "all optional fields",
			payload: map[string]any{
				"fp_ua":           "abb0b6e7da81a42233f8f2b1a8ddb1b9a4c81611",
				"acr":             "AAL2",
				"amr":             []any{"otp"},
				"ext_id":          "12345",
				"event_timestamp": float64(1615304991),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("marshaling payload: %v", err)
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

			roundtripped, err := json.Marshal(&e)
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			var got, want map[string]any
			json.Unmarshal(roundtripped, &got)
			json.Unmarshal(data, &want)
			if len(got) != len(want) {
				t.Errorf("roundtrip field count: got %d, want %d", len(got), len(want))
			}
		})
	}
}

func TestSessionEstablishedRegistered(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"event_timestamp": float64(1615304991)})
	e, err := parseSessionEstablishedEvent(data)
	if err != nil {
		t.Fatalf("parseSessionEstablishedEvent: %v", err)
	}
	if e.Type() != EventTypeSessionEstablished {
		t.Errorf("Type() = %q, want %q", e.Type(), EventTypeSessionEstablished)
	}
}
