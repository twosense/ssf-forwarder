package caepext

import (
	"encoding/json"
	"testing"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/schemes/caep"
)

func TestRiskLevelChangeValidate(t *testing.T) {
	prevLow := RiskLevelLow
	prevInvalid := RiskLevel("medium")
	negTS := int64(-1)

	meta := caep.NewEventMetadata()
	meta.EventTimestamp = &negTS

	tests := []struct {
		name    string
		event   RiskLevelChangeEvent
		wantErr bool
	}{
		{
			name:  "valid minimal",
			event: RiskLevelChangeEvent{RiskLevelChangePayload: RiskLevelChangePayload{CurrentLevel: RiskLevelLow, Principal: PrincipalUser}},
		},
		{
			name: "valid with optional fields",
			event: RiskLevelChangeEvent{RiskLevelChangePayload: RiskLevelChangePayload{
				CurrentLevel:  RiskLevelHigh,
				PreviousLevel: &prevLow,
				Principal:     PrincipalDevice,
				RiskReason:    "PASSWORD_FOUND_IN_DATA_BREACH",
			}},
		},
		{
			name:    "missing principal",
			event:   RiskLevelChangeEvent{RiskLevelChangePayload: RiskLevelChangePayload{CurrentLevel: RiskLevelLow}},
			wantErr: true,
		},
		{
			name:    "missing current_level",
			event:   RiskLevelChangeEvent{RiskLevelChangePayload: RiskLevelChangePayload{Principal: PrincipalUser}},
			wantErr: true,
		},
		{
			name:    "invalid current_level",
			event:   RiskLevelChangeEvent{RiskLevelChangePayload: RiskLevelChangePayload{CurrentLevel: "medium", Principal: PrincipalUser}},
			wantErr: true,
		},
		{
			name: "invalid previous_level",
			event: RiskLevelChangeEvent{RiskLevelChangePayload: RiskLevelChangePayload{
				CurrentLevel:  RiskLevelLow,
				PreviousLevel: &prevInvalid,
				Principal:     PrincipalUser,
			}},
			wantErr: true,
		},
		{
			name: "invalid metadata timestamp",
			event: func() RiskLevelChangeEvent {
				e := RiskLevelChangeEvent{RiskLevelChangePayload: RiskLevelChangePayload{CurrentLevel: RiskLevelLow, Principal: PrincipalUser}}
				e.Metadata = meta
				return e
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRiskLevelChangeRoundtrip(t *testing.T) {
	data, err := json.Marshal(map[string]any{
		"current_level":   "HIGH",
		"previous_level":  "LOW",
		"principal":       "DEVICE",
		"risk_reason":     "SUSPICIOUS_ACTIVITY",
		"event_timestamp": int64(1615304991),
	})
	if err != nil {
		t.Fatalf("marshaling input: %v", err)
	}

	var e RiskLevelChangeEvent
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if e.Type() != EventTypeRiskLevelChange {
		t.Errorf("Type() = %q, want %q", e.Type(), EventTypeRiskLevelChange)
	}
	if e.CurrentLevel != RiskLevelHigh {
		t.Errorf("CurrentLevel = %q, want %q", e.CurrentLevel, RiskLevelHigh)
	}
	if e.PreviousLevel == nil || *e.PreviousLevel != RiskLevelLow {
		t.Errorf("PreviousLevel = %v, want %q", e.PreviousLevel, RiskLevelLow)
	}
	if e.Principal != PrincipalDevice {
		t.Errorf("Principal = %q, want %q", e.Principal, PrincipalDevice)
	}
	if e.RiskReason != "SUSPICIOUS_ACTIVITY" {
		t.Errorf("RiskReason = %q, want %q", e.RiskReason, "SUSPICIOUS_ACTIVITY")
	}
}

func TestRiskLevelChangeUnmarshalValidates(t *testing.T) {
	invalid, _ := json.Marshal(map[string]any{"current_level": "medium", "principal": "USER"})
	var e RiskLevelChangeEvent
	if err := json.Unmarshal(invalid, &e); err == nil {
		t.Error("expected error for invalid current_level, got nil")
	}
}

func TestRiskLevelChangeParser(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"current_level": "LOW",
		"principal":     "USER",
	})

	e, err := parseRiskLevelChangeEvent(data)
	if err != nil {
		t.Fatalf("parseRiskLevelChangeEvent: %v", err)
	}
	if e.Type() != EventTypeRiskLevelChange {
		t.Errorf("Type() = %q, want %q", e.Type(), EventTypeRiskLevelChange)
	}
}
