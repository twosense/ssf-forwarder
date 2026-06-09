package ssfadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestFindOurStream(t *testing.T) {
	mine := StreamConfig{StreamID: "1", Description: StreamDescription}
	other := StreamConfig{StreamID: "2", Description: "not-ours"}

	t.Run("none", func(t *testing.T) {
		got, err := FindOurStream([]StreamConfig{other})
		if err != nil || got != nil {
			t.Fatalf("got %+v, err %v", got, err)
		}
	})
	t.Run("one", func(t *testing.T) {
		got, err := FindOurStream([]StreamConfig{other, mine})
		if err != nil || got == nil || got.StreamID != "1" {
			t.Fatalf("got %+v, err %v", got, err)
		}
	})
	t.Run("multiple", func(t *testing.T) {
		_, err := FindOurStream([]StreamConfig{mine, mine})
		if err == nil {
			t.Fatalf("expected error for multiple matches")
		}
	})
}

func TestReconcileCreatesWhenAbsent(t *testing.T) {
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]StreamConfig{})
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(StreamConfig{StreamID: "created", Description: StreamDescription})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	c := testClient(t, srv)
	action, stream, err := Reconcile(context.Background(), c, "https://lb/events", []string{"e1"})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if action != ActionCreated || stream.StreamID != "created" {
		t.Fatalf("action %q stream %+v", action, stream)
	}
}

func TestReconcileNoopWhenMatching(t *testing.T) {
	existing := StreamConfig{
		StreamID:        "x",
		Description:     StreamDescription,
		Delivery:        DeliveryConfig{Method: "urn:ietf:rfc:8935", EndpointURL: "https://lb/events"},
		EventsRequested: []string{"e1", "e2"},
	}
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s; reconcile must not write when matching", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]StreamConfig{existing})
	})
	c := testClient(t, srv)
	// Event order differs but set is the same -> still a no-op.
	action, _, err := Reconcile(context.Background(), c, "https://lb/events", []string{"e2", "e1"})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if action != ActionUnchanged {
		t.Fatalf("action: got %q want unchanged", action)
	}
}

func TestReconcileUpdatesOnURLChange(t *testing.T) {
	existing := StreamConfig{
		StreamID:        "x",
		Description:     StreamDescription,
		Delivery:        DeliveryConfig{Method: "urn:ietf:rfc:8935", EndpointURL: "https://old/events"},
		EventsRequested: []string{"e1"},
	}
	var updateBody StreamConfig
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]StreamConfig{existing})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&updateBody)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(StreamConfig{StreamID: "x", Description: StreamDescription,
				Delivery: DeliveryConfig{Method: "urn:ietf:rfc:8935", EndpointURL: "https://new/events"}})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	c := testClient(t, srv)
	action, stream, err := Reconcile(context.Background(), c, "https://new/events", []string{"e1"})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if action != ActionUpdated {
		t.Fatalf("action: got %q want updated", action)
	}
	if updateBody.StreamID != "x" {
		t.Fatalf("update must target existing stream id, got %q", updateBody.StreamID)
	}
	if stream.Delivery.EndpointURL != "https://new/events" {
		t.Fatalf("updated endpoint: %q", stream.Delivery.EndpointURL)
	}
}
