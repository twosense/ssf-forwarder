package caepext

import (
	"encoding/json"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
	"github.com/sgnl-ai/caep.dev/secevent/pkg/schemes/caep"
)

const EventTypeSessionPresented event.EventType = "https://schemas.openid.net/secevent/caep/event-type/session-presented"

// SessionPresentedPayload holds the event-specific claims for a session-presented event.
// All claims are optional per Section 3.7.1 of the CAEP specification.
type SessionPresentedPayload struct {
	FpUA  string `json:"fp_ua,omitempty"`
	ExtID string `json:"ext_id,omitempty"`
}

// SessionPresentedEvent represents a CAEP session-presented event.
type SessionPresentedEvent struct {
	caep.BaseCAEPEvent
	SessionPresentedPayload
}

func (e *SessionPresentedEvent) Validate() error {
	return e.ValidateMetadata()
}

func (e *SessionPresentedEvent) Payload() any {
	payload := e.SessionPresentedPayload

	if e.Metadata != nil {
		return struct {
			SessionPresentedPayload
			*caep.EventMetadata
		}{
			SessionPresentedPayload: payload,
			EventMetadata:           e.Metadata,
		}
	}

	return payload
}

func (e *SessionPresentedEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Payload())
}

func (e *SessionPresentedEvent) UnmarshalJSON(data []byte) error {
	var payload struct {
		SessionPresentedPayload
		*caep.EventMetadata
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return event.NewError(event.ErrCodeParseError, "failed to parse session-presented event data", "", err.Error())
	}

	e.SetType(EventTypeSessionPresented)
	e.SessionPresentedPayload = payload.SessionPresentedPayload
	e.Metadata = payload.EventMetadata

	return e.Validate()
}

func parseSessionPresentedEvent(data []byte) (event.Event, error) {
	var e SessionPresentedEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, event.NewError(event.ErrCodeParseError, "failed to parse session-presented event", "", err.Error())
	}

	return &e, nil
}

func init() {
	event.RegisterEventParser(EventTypeSessionPresented, parseSessionPresentedEvent)
}
