package caepext

import (
	"encoding/json"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
)

const EventTypeSessionPresented event.EventType = "https://schemas.openid.net/secevent/caep/event-type/session-presented"

// SessionPresentedEvent represents a CAEP session-presented event.
// All event-specific claims (fp_ua, ext_id) are optional per the spec.
type SessionPresentedEvent struct {
	event.BaseEvent
	payload map[string]any
}

func (e *SessionPresentedEvent) Validate() error {
	return nil
}

func (e *SessionPresentedEvent) Payload() any {
	return e.payload
}

func (e *SessionPresentedEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.payload)
}

func (e *SessionPresentedEvent) UnmarshalJSON(data []byte) error {
	e.SetType(EventTypeSessionPresented)

	return json.Unmarshal(data, &e.payload)
}

func parseSessionPresentedEvent(data []byte) (event.Event, error) {
	var e SessionPresentedEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, event.NewError(event.ErrCodeParseError,
			"failed to parse session-presented event", "", err.Error())
	}

	return &e, nil
}

func init() {
	event.RegisterEventParser(EventTypeSessionPresented, parseSessionPresentedEvent)
}
