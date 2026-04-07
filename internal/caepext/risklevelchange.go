// Package caepext registers custom CAEP event type parsers not yet included
// in the upstream caep.dev/secevent library.
package caepext

import (
	"encoding/json"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
	"github.com/sgnl-ai/caep.dev/secevent/pkg/schemes/caep"
)

const EventTypeRiskLevelChange event.EventType = "https://schemas.openid.net/secevent/caep/event-type/risk-level-change"

// RiskLevel represents the risk level values defined in Section 3.8.1 of the CAEP specification.
type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "LOW"
	RiskLevelMedium RiskLevel = "MEDIUM"
	RiskLevelHigh   RiskLevel = "HIGH"
)

// Principal identifies the type of entity involved in a risk event.
// The spec defines well-known values but permits any string.
type Principal string

const (
	PrincipalUser    Principal = "USER"
	PrincipalDevice  Principal = "DEVICE"
	PrincipalSession Principal = "SESSION"
	PrincipalTenant  Principal = "TENANT"
	PrincipalOrgUnit Principal = "ORG_UNIT"
	PrincipalGroup   Principal = "GROUP"
)

// RiskLevelChangePayload holds the event-specific claims for a risk-level-change event.
type RiskLevelChangePayload struct {
	CurrentLevel  RiskLevel  `json:"current_level"`
	PreviousLevel *RiskLevel `json:"previous_level,omitempty"`
	Principal     Principal  `json:"principal"`
	RiskReason    string     `json:"risk_reason,omitempty"`
}

// RiskLevelChangeEvent represents a CAEP risk-level-change event.
type RiskLevelChangeEvent struct {
	caep.BaseCAEPEvent
	RiskLevelChangePayload
}

var validRiskLevels = map[RiskLevel]bool{
	RiskLevelLow:    true,
	RiskLevelMedium: true,
	RiskLevelHigh:   true,
}

func (e *RiskLevelChangeEvent) Validate() error {
	if err := e.ValidateMetadata(); err != nil {
		return err
	}

	if e.Principal == "" {
		return event.NewError(event.ErrCodeMissingField, "missing required claim: principal", "principal", "")
	}

	if e.CurrentLevel == "" {
		return event.NewError(event.ErrCodeMissingField, "missing required claim: current_level", "current_level", "")
	}

	if !validRiskLevels[e.CurrentLevel] {
		return event.NewError(event.ErrCodeInvalidValue, "current_level must be LOW, MEDIUM, or HIGH", "current_level", string(e.CurrentLevel))
	}

	if e.PreviousLevel != nil && !validRiskLevels[*e.PreviousLevel] {
		return event.NewError(event.ErrCodeInvalidValue, "previous_level must be LOW, MEDIUM, or HIGH", "previous_level", string(*e.PreviousLevel))
	}

	return nil
}

func (e *RiskLevelChangeEvent) Payload() any {
	payload := e.RiskLevelChangePayload

	if e.Metadata != nil {
		return struct {
			RiskLevelChangePayload
			*caep.EventMetadata
		}{
			RiskLevelChangePayload: payload,
			EventMetadata:          e.Metadata,
		}
	}

	return payload
}

func (e *RiskLevelChangeEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Payload())
}

func (e *RiskLevelChangeEvent) UnmarshalJSON(data []byte) error {
	var payload struct {
		RiskLevelChangePayload
		*caep.EventMetadata
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return event.NewError(event.ErrCodeParseError, "failed to parse risk-level-change event data", "", err.Error())
	}

	e.SetType(EventTypeRiskLevelChange)
	e.RiskLevelChangePayload = payload.RiskLevelChangePayload
	e.Metadata = payload.EventMetadata

	return e.Validate()
}

func parseRiskLevelChangeEvent(data []byte) (event.Event, error) {
	var e RiskLevelChangeEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, event.NewError(event.ErrCodeParseError, "failed to parse risk-level-change event", "", err.Error())
	}

	return &e, nil
}

func init() {
	event.RegisterEventParser(EventTypeRiskLevelChange, parseRiskLevelChangeEvent)
}
