// Package caepext registers custom CAEP event type parsers not yet included
// in the upstream caep.dev/secevent library.
package caepext

import (
	"encoding/json"
	"fmt"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
)

const EventTypeRiskLevelChange event.EventType = "https://schemas.openid.net/secevent/caep/event-type/risk-level-change"

// validRiskLevels are the permitted values for current_level and previous_level
// per Section 3.8.1 of the CAEP specification.
var validRiskLevels = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true}

// RiskLevelChangeEvent represents a CAEP risk-level-change event.
type RiskLevelChangeEvent struct {
	event.BaseEvent
	payload map[string]any
}

func (e *RiskLevelChangeEvent) Validate() error {
	if _, ok := e.payload["principal"].(string); !ok {
		return fmt.Errorf("risk-level-change: missing required claim: principal")
	}

	currentLevel, ok := e.payload["current_level"].(string)
	if !ok {
		return fmt.Errorf("risk-level-change: missing required claim: current_level")
	}
	if !validRiskLevels[currentLevel] {
		return fmt.Errorf("risk-level-change: current_level must be LOW, MEDIUM, or HIGH; got %q", currentLevel)
	}

	if prevLevel, ok := e.payload["previous_level"].(string); ok {
		if !validRiskLevels[prevLevel] {
			return fmt.Errorf("risk-level-change: previous_level must be LOW, MEDIUM, or HIGH; got %q", prevLevel)
		}
	}

	return nil
}

func (e *RiskLevelChangeEvent) Payload() any {
	return e.payload
}

func (e *RiskLevelChangeEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.payload)
}

func (e *RiskLevelChangeEvent) UnmarshalJSON(data []byte) error {
	e.SetType(EventTypeRiskLevelChange)

	return json.Unmarshal(data, &e.payload)
}

func parseRiskLevelChangeEvent(data []byte) (event.Event, error) {
	var e RiskLevelChangeEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, event.NewError(event.ErrCodeParseError,
			"failed to parse risk-level-change event", "", err.Error())
	}

	return &e, nil
}

func init() {
	event.RegisterEventParser(EventTypeRiskLevelChange, parseRiskLevelChangeEvent)
}
