//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"
)

// TestAutoRegisterContinuesWhenStreamExists verifies that an auto-registering
// instance which finds a stream already registered on boot warns but keeps
// running, rather than refusing to start. A leftover stream is expected after
// an unclean shutdown, so refusing would turn a recoverable state into an
// outage.
func TestAutoRegisterContinuesWhenStreamExists(t *testing.T) {
	transmitter := newFakeTransmitter(t)
	sink := newTestSink(t)
	metadataURL := transmitter.server.URL + "/metadata"
	eventTypes := []string{"https://schemas.openid.net/secevent/ssf/event-type/verification"}

	listenAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	publicURL := "http://" + listenAddr

	autoRegisterOff := false
	registerCfg := writeConfig(t, forwarderConfig{
		metadataURL:  metadataURL,
		sinkURL:      sink.server.URL,
		publicURL:    publicURL,
		listenAddr:   listenAddr,
		eventTypes:   eventTypes,
		autoRegister: &autoRegisterOff,
	})

	// Pre-seed a stream as if a previous run had registered and then crashed
	// without tearing it down.
	runForwarder(t, registerCfg, "register")
	if got := transmitter.streamCount(); got != 1 {
		t.Fatalf("after seeding stream: stream count = %d, want 1", got)
	}

	// Start an instance with auto_register on (the default). It should reconcile
	// against the existing stream, warn, and serve — not exit.
	autoCfg := writeConfig(t, forwarderConfig{
		metadataURL: metadataURL,
		sinkURL:     sink.server.URL,
		publicURL:   publicURL,
		listenAddr:  listenAddr,
		eventTypes:  eventTypes,
	})
	startForwarder(t, autoCfg)
	waitForServer(t, publicURL+"/events", 5*time.Second)

	if got := transmitter.streamCount(); got != 1 {
		t.Fatalf("after auto-register boot with existing stream: stream count = %d, want 1 (must reconcile, not duplicate)", got)
	}

	// It serves: a pushed SET is forwarded to the sink.
	token := transmitter.signSET(t)
	pushSET(t, publicURL+"/events", token)
	if received := sink.waitForToken(t, 5*time.Second); received != token {
		t.Errorf("sink received unexpected token\ngot:  %s\nwant: %s", received, token)
	}
}
