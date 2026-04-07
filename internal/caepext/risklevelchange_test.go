package caepext

import (
	"encoding/json"
	"testing"
)

func TestRiskLevelChangeValidate(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		wantErr bool
	}{
		{
			name: "valid minimal",
			payload: map[string]any{
				"current_level": "LOW",
				"principal":     "USER",
			},
		},
		{
			name: "valid with optional fields",
			payload: map[string]any{
				"current_level":  "HIGH",
				"previous_level": "MEDIUM",
				"principal":      "DEVICE",
				"risk_reason":    "PASSWORD_FOUND_IN_DATA_BREACH",
			},
		},
		{
			name:    "missing principal",
			payload: map[string]any{"current_level": "LOW"},
			wantErr: true,
		},
		{
			name:    "missing current_level",
			payload: map[string]any{"principal": "USER"},
			wantErr: true,
		},
		{
			name:    "current_level invalid",
			payload: map[string]any{"current_level": "medium", "principal": "USER"},
			wantErr: true,
		},
		{
			name:    "previous_level invalid",
			payload: map[string]any{"current_level": "LOW", "previous_level": "high", "principal": "USER"},
			wantErr: true,
		},
		{
			name:    "principal not a string",
			payload: map[string]any{"current_level": "LOW", "principal": 42},
			wantErr: true,
		},
		{
			name:    "current_level not a string",
			payload: map[string]any{"current_level": 1, "principal": "USER"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("marshaling payload: %v", err)
			}

			var e RiskLevelChangeEvent
			if err := json.Unmarshal(data, &e); err != nil {
				t.Fatalf("unmarshaling event: %v", err)
			}

			err = e.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
