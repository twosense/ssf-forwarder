//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestForwardsSETToWebhookSink(t *testing.T) {
	transmitter := newFakeTransmitter(t)
	sink := newTestSink(t)

	port := freePort(t)
	listenAddr := fmt.Sprintf("127.0.0.1:%d", port)
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	cfgPath := writeConfig(t, forwarderConfig{
		metadataURL: transmitter.server.URL + "/metadata",
		sinkURL:     sink.server.URL,
		publicURL:   publicURL,
		listenAddr:  listenAddr,
		eventTypes:  []string{"https://schemas.openid.net/secevent/ssf/event-type/verification"},
	})

	startForwarder(t, cfgPath)

	// Block until the forwarder registers its push stream with the transmitter.
	// This also confirms the forwarder started and reached the transmitter.
	transmitter.waitForRegistration(t, 15*time.Second)

	// The forwarder starts its HTTP server after stream registration,
	// so poll until it is accepting connections.
	waitForServer(t, publicURL+"/events", 5*time.Second)

	token := transmitter.signSET(t)

	resp, err := http.Post(
		transmitter.getPushURL(),
		"application/secevent+jwt",
		strings.NewReader(token),
	)
	if err != nil {
		t.Fatalf("pushing SET to forwarder: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("forwarder returned %d, want 202", resp.StatusCode)
	}

	received := sink.waitForToken(t, 5*time.Second)

	if received != token {
		t.Errorf("sink received unexpected token\ngot:  %s\nwant: %s", received, token)
	}
}

func TestForwardsRiskLevelChangeSETToWebhookSink(t *testing.T) {
	transmitter := newFakeTransmitter(t)
	sink := newTestSink(t)

	port := freePort(t)
	listenAddr := fmt.Sprintf("127.0.0.1:%d", port)
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	cfgPath := writeConfig(t, forwarderConfig{
		metadataURL: transmitter.server.URL + "/metadata",
		sinkURL:     sink.server.URL,
		publicURL:   publicURL,
		listenAddr:  listenAddr,
		eventTypes:  []string{"https://schemas.openid.net/secevent/caep/event-type/risk-level-change"},
	})

	startForwarder(t, cfgPath)

	transmitter.waitForRegistration(t, 15*time.Second)
	waitForServer(t, publicURL+"/events", 5*time.Second)

	token := transmitter.signRiskLevelChangeSET(t, "MEDIUM")

	resp, err := http.Post(
		transmitter.getPushURL(),
		"application/secevent+jwt",
		strings.NewReader(token),
	)
	if err != nil {
		t.Fatalf("pushing SET to forwarder: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("forwarder returned %d, want 202", resp.StatusCode)
	}

	received := sink.waitForToken(t, 5*time.Second)

	if received != token {
		t.Errorf("sink received unexpected token\ngot:  %s\nwant: %s", received, token)
	}
}

func TestFilterForwardsOnlyHighRiskLevelChange(t *testing.T) {
	transmitter := newFakeTransmitter(t)
	sink := newTestSink(t)

	port := freePort(t)
	listenAddr := fmt.Sprintf("127.0.0.1:%d", port)
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	cfgPath := writeConfig(t, forwarderConfig{
		metadataURL: transmitter.server.URL + "/metadata",
		sinkURL:     sink.server.URL,
		publicURL:   publicURL,
		listenAddr:  listenAddr,
		eventTypes:  []string{"https://schemas.openid.net/secevent/caep/event-type/risk-level-change"},
		filters: []string{
			`event_type == "https://schemas.openid.net/secevent/caep/event-type/risk-level-change"`,
			`event.current_level == "HIGH"`,
		},
	})

	startForwarder(t, cfgPath)
	transmitter.waitForRegistration(t, 15*time.Second)
	waitForServer(t, publicURL+"/events", 5*time.Second)

	// Push a HIGH risk-level-change SET — the filter should pass and forward it.
	highToken := transmitter.signRiskLevelChangeSET(t, "HIGH")

	resp, err := http.Post(
		transmitter.getPushURL(),
		"application/secevent+jwt",
		strings.NewReader(highToken),
	)
	if err != nil {
		t.Fatalf("pushing HIGH SET to forwarder: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("forwarder returned %d, want 202 (HIGH SET)", resp.StatusCode)
	}

	received := sink.waitForToken(t, 5*time.Second)
	if received != highToken {
		t.Errorf("sink received unexpected token for HIGH SET\ngot:  %s\nwant: %s", received, highToken)
	}

	// Push a LOW risk-level-change SET — the filter should drop it (current_level != "HIGH").
	lowToken := transmitter.signRiskLevelChangeSET(t, "LOW")

	resp, err = http.Post(
		transmitter.getPushURL(),
		"application/secevent+jwt",
		strings.NewReader(lowToken),
	)
	if err != nil {
		t.Fatalf("pushing LOW SET to forwarder: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("forwarder returned %d, want 202 (LOW SET)", resp.StatusCode)
	}

	sink.expectNoToken(t, 2*time.Second)
}
