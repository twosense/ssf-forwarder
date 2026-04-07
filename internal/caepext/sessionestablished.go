package caepext

import (
	"encoding/json"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
)

const EventTypeSessionEstablished event.EventType = "https://schemas.openid.net/secevent/caep/event-type/session-established"

// SessionEstablishedEvent represents a CAEP session-established event.
// All event-specific claims (fp_ua, acr, amr, ext_id) are optional per the spec.
type SessionEstablishedEvent struct {
	event.BaseEvent
	payload map[string]any
}

func (e *SessionEstablishedEvent) Validate() error {
	return nil
}

func (e *SessionEstablishedEvent) Payload() any {
	return e.payload
}

func (e *SessionEstablishedEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.payload)
}

func (e *SessionEstablishedEvent) UnmarshalJSON(data []byte) error {
	e.SetType(EventTypeSessionEstablished)

	return json.Unmarshal(data, &e.payload)
}

func parseSessionEstablishedEvent(data []byte) (event.Event, error) {
	var e SessionEstablishedEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, event.NewError(event.ErrCodeParseError,
			"failed to parse session-established event", "", err.Error())
	}

	return &e, nil
}

func init() {
	event.RegisterEventParser(EventTypeSessionEstablished, parseSessionEstablishedEvent)
}
