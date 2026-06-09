package ssfadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sgnl-ai/caep.dev/ssfreceiver/auth"
)

// newTestTransmitter returns an httptest server whose metadata document points
// its configuration_endpoint at <server>/streams, and a handler func the test
// installs for the /streams route.
func newTestTransmitter(t *testing.T, streamsHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/ssf-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                     "https://transmitter.example.com",
			"configuration_endpoint":     srv.URL + "/streams",
			"delivery_methods_supported": []string{"urn:ietf:rfc:8935"},
		})
	})
	mux.HandleFunc("/streams", streamsHandler)
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	authorizer, err := auth.NewBearer("secret")
	if err != nil {
		t.Fatalf("NewBearer: %v", err)
	}
	c, err := NewClient(context.Background(), srv.URL+"/.well-known/ssf-configuration", authorizer)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestListDecodesArray(t *testing.T) {
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization header: got %q", got)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"stream_id":        "abc",
				"description":      StreamDescription,
				"delivery":         map[string]any{"method": "urn:ietf:rfc:8935", "endpoint_url": "https://lb.example.com/events"},
				"events_requested": []string{"e1"},
			},
		})
	})
	c := testClient(t, srv)
	streams, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(streams) != 1 || streams[0].StreamID != "abc" || streams[0].Description != StreamDescription {
		t.Fatalf("unexpected streams: %+v", streams)
	}
	if streams[0].Delivery.EndpointURL != "https://lb.example.com/events" {
		t.Fatalf("endpoint: %q", streams[0].Delivery.EndpointURL)
	}
}

func TestListDecodesSingleObject(t *testing.T) {
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"stream_id":   "solo",
			"description": "someone-elses-stream",
			"delivery":    map[string]any{"method": "urn:ietf:rfc:8935", "endpoint_url": "https://x/events"},
		})
	})
	c := testClient(t, srv)
	streams, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(streams) != 1 || streams[0].StreamID != "solo" {
		t.Fatalf("unexpected streams: %+v", streams)
	}
}

func TestListEmpty(t *testing.T) {
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	c := testClient(t, srv)
	streams, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(streams) != 0 {
		t.Fatalf("expected no streams, got %+v", streams)
	}
}

func TestCreatePostsRequestAndDecodes(t *testing.T) {
	var gotBody StreamConfig
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"stream_id":   "new-id",
			"description": StreamDescription,
			"delivery":    map[string]any{"method": "urn:ietf:rfc:8935", "endpoint_url": gotBody.Delivery.EndpointURL},
		})
	})
	c := testClient(t, srv)
	out, err := c.Create(context.Background(), StreamConfig{
		Description:     StreamDescription,
		Delivery:        DeliveryConfig{Method: "urn:ietf:rfc:8935", EndpointURL: "https://lb/events"},
		EventsRequested: []string{"e1"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.StreamID != "new-id" {
		t.Fatalf("stream id: %q", out.StreamID)
	}
	if gotBody.StreamID != "" {
		t.Fatalf("create body should omit stream_id, got %q", gotBody.StreamID)
	}
	if gotBody.Description != StreamDescription {
		t.Fatalf("create body description: %q", gotBody.Description)
	}
}

func TestUpdatePutsRequestWithStreamID(t *testing.T) {
	var gotBody StreamConfig
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method: got %s want PUT", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"stream_id":   "abc",
			"description": StreamDescription,
			"delivery":    map[string]any{"method": "urn:ietf:rfc:8935", "endpoint_url": "https://new/events"},
		})
	})
	c := testClient(t, srv)
	out, err := c.Update(context.Background(), StreamConfig{
		StreamID:    "abc",
		Description: StreamDescription,
		Delivery:    DeliveryConfig{Method: "urn:ietf:rfc:8935", EndpointURL: "https://new/events"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if gotBody.StreamID != "abc" {
		t.Fatalf("update body must include stream_id, got %q", gotBody.StreamID)
	}
	if out.Delivery.EndpointURL != "https://new/events" {
		t.Fatalf("updated endpoint: %q", out.Delivery.EndpointURL)
	}
}

func TestDeleteSendsStreamID(t *testing.T) {
	var gotQuery string
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method: got %s want DELETE", r.Method)
		}
		gotQuery = r.URL.Query().Get("stream_id")
		w.WriteHeader(http.StatusNoContent)
	})
	c := testClient(t, srv)
	if err := c.Delete(context.Background(), "abc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotQuery != "abc" {
		t.Fatalf("delete stream_id: got %q", gotQuery)
	}
}

func TestDeleteEncodesStreamID(t *testing.T) {
	var gotQuery string
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method: got %s want DELETE", r.Method)
		}
		gotQuery = r.URL.Query().Get("stream_id")
		w.WriteHeader(http.StatusNoContent)
	})
	c := testClient(t, srv)
	if err := c.Delete(context.Background(), "a/b c"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotQuery != "a/b c" {
		t.Fatalf("delete stream_id round-trip: got %q want %q", gotQuery, "a/b c")
	}
}

func TestDeleteTreats404AsSuccess(t *testing.T) {
	srv := newTestTransmitter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method: got %s want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
	})
	c := testClient(t, srv)
	if err := c.Delete(context.Background(), "gone"); err != nil {
		t.Fatalf("Delete returned error on 404, want nil: %v", err)
	}
}
