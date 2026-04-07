// Package caepext registers custom CAEP event type parsers not yet included
// in the upstream caep.dev/secevent library.
package caepext

import (
	"encoding/json"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
)

const EventTypeRiskLevelChange event.EventType = "https://schemas.openid.net/secevent/caep/event-type/risk-level-change"

// RiskLevelChangeEvent represents a CAEP risk-level-change event.
type RiskLevelChangeEvent struct {
	event.BaseEvent
	payload map[string]any
}

func (e *RiskLevelChangeEvent) Validate() error {
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
