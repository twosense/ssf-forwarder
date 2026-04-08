package caepext

import (
	"encoding/json"

	"github.com/sgnl-ai/caep.dev/secevent/pkg/event"
	"github.com/sgnl-ai/caep.dev/secevent/pkg/schemes/caep"
)

const EventTypeSessionEstablished event.EventType = "https://schemas.openid.net/secevent/caep/event-type/session-established"

// SessionEstablishedPayload holds the event-specific claims for a session-established event.
// All claims are optional per Section 3.6.1 of the CAEP specification.
type SessionEstablishedPayload struct {
	FpUA  string   `json:"fp_ua,omitempty"`
	ACR   string   `json:"acr,omitempty"`
	AMR   []string `json:"amr,omitempty"`
	ExtID string   `json:"ext_id,omitempty"`
}

// SessionEstablishedEvent represents a CAEP session-established event.
type SessionEstablishedEvent struct {
	caep.BaseCAEPEvent
	SessionEstablishedPayload
}

func (e *SessionEstablishedEvent) Validate() error {
	return e.ValidateMetadata()
}

func (e *SessionEstablishedEvent) Payload() any {
	payload := e.SessionEstablishedPayload

	if e.Metadata != nil {
		return struct {
			SessionEstablishedPayload
			*caep.EventMetadata
		}{
			SessionEstablishedPayload: payload,
			EventMetadata:             e.Metadata,
		}
	}

	return payload
}

func (e *SessionEstablishedEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Payload())
}

func (e *SessionEstablishedEvent) UnmarshalJSON(data []byte) error {
	var payload struct {
		SessionEstablishedPayload
		*caep.EventMetadata
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return event.NewError(event.ErrCodeParseError, "failed to parse session-established event data", "", err.Error())
	}

	e.SetType(EventTypeSessionEstablished)
	e.SessionEstablishedPayload = payload.SessionEstablishedPayload
	e.Metadata = payload.EventMetadata

	return e.Validate()
}

func parseSessionEstablishedEvent(data []byte) (event.Event, error) {
	var e SessionEstablishedEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, event.NewError(event.ErrCodeParseError, "failed to parse session-established event", "", err.Error())
	}

	return &e, nil
}

func init() {
	event.RegisterEventParser(EventTypeSessionEstablished, parseSessionEstablishedEvent)
}
