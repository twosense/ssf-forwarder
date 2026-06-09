//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestStreamLifecycleAcrossInstances exercises the horizontal-scaling workflow:
// a single `register` creates the stream, two `serve` instances run with
// auto_register off (boot safeguard passes, neither re-registers), each
// instance independently forwards a pushed SET, and `deregister` removes the
// stream. Throughout, exactly one stream exists on the transmitter — the
// guarantee that lets multiple instances sit behind one load balancer.
func TestStreamLifecycleAcrossInstances(t *testing.T) {
	transmitter := newFakeTransmitter(t)
	sink := newTestSink(t)
	metadataURL := transmitter.server.URL + "/metadata"
	eventTypes := []string{"https://schemas.openid.net/secevent/ssf/event-type/verification"}

	// The push URL the stream advertises stands in for the load balancer in
	// front of the instances. The test pushes directly to each instance below,
	// so this address is never bound.
	publicURL := fmt.Sprintf("http://127.0.0.1:%d", freePort(t))

	addrA := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	addrB := fmt.Sprintf("127.0.0.1:%d", freePort(t))

	autoRegisterOff := false
	makeConfig := func(listenAddr string) string {
		return writeConfig(t, forwarderConfig{
			metadataURL:  metadataURL,
			sinkURL:      sink.server.URL,
			publicURL:    publicURL,
			listenAddr:   listenAddr,
			eventTypes:   eventTypes,
			autoRegister: &autoRegisterOff,
		})
	}

	// 1. register creates exactly one stream pointing at the load balancer URL.
	runForwarder(t, makeConfig(addrA), "register")

	if got := transmitter.streamCount(); got != 1 {
		t.Fatalf("after register: stream count = %d, want 1", got)
	}
	// The forwarder advertises public_url + endpoint as the stream's push URL.
	wantPushURL := publicURL + "/events"
	if got := transmitter.getPushURL(); got != wantPushURL {
		t.Fatalf("after register: push URL = %q, want %q", got, wantPushURL)
	}

	// 2. Two serve instances start with auto_register off. The boot safeguard
	//    finds the existing stream with a matching URL, so neither registers.
	startForwarder(t, makeConfig(addrA))
	stopB := startForwarder(t, makeConfig(addrB))
	waitForServer(t, "http://"+addrA+"/events", 5*time.Second)
	waitForServer(t, "http://"+addrB+"/events", 5*time.Second)

	if got := transmitter.streamCount(); got != 1 {
		t.Fatalf("after starting two serve instances: stream count = %d, want 1 (no instance should re-register)", got)
	}

	// 3. Each instance independently forwards a pushed SET to the sink.
	for _, addr := range []string{addrA, addrB} {
		token := transmitter.signSET(t)
		pushSET(t, "http://"+addr+"/events", token)

		received := sink.waitForToken(t, 5*time.Second)
		if received != token {
			t.Errorf("instance %s: sink received unexpected token\ngot:  %s\nwant: %s", addr, received, token)
		}
	}

	// 4. Stopping one instance must not delete the shared stream — only the
	//    register/deregister lifecycle owns it. This is the guarantee that lets
	//    an instance restart behind the load balancer without disrupting the
	//    others. stopB blocks until the process has fully exited.
	stopB()
	if got := transmitter.streamCount(); got != 1 {
		t.Fatalf("after stopping one serve instance: stream count = %d, want 1 (serve must not delete the shared stream)", got)
	}

	// 5. deregister removes the stream.
	runForwarder(t, makeConfig(addrA), "deregister")

	if got := transmitter.streamCount(); got != 0 {
		t.Fatalf("after deregister: stream count = %d, want 0", got)
	}
}

// pushSET posts a SET to a forwarder's event endpoint and fails the test unless
// it is accepted with 202.
func pushSET(t *testing.T, eventsURL, token string) {
	t.Helper()
	resp, err := http.Post(eventsURL, "application/secevent+jwt", strings.NewReader(token))
	if err != nil {
		t.Fatalf("pushing SET to %s: %v", eventsURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("pushing SET to %s: status %d, want 202", eventsURL, resp.StatusCode)
	}
}
