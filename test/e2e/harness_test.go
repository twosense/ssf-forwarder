//go:build e2e

package e2e

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// forwarderCmd builds the command that runs the forwarder with the given
// config. Extra args (e.g. a "register" subcommand) are passed through.
//
// In binary mode the compiled binary is run directly with --config. In Docker
// mode (E2E_DOCKER=1) the pre-built image is run with --network host so the
// container shares the host's network stack and can reach 127.0.0.1 services;
// the config is mounted at the path the image reads by default. Docker mode is
// only supported on Linux.
func forwarderCmd(configPath string, args ...string) *exec.Cmd {
	if useDocker {
		dockerArgs := []string{
			"run", "--rm",
			"--network", "host",
			"-v", configPath + ":/etc/ssf-forwarder/config.yaml:ro",
			dockerImage,
		}
		dockerArgs = append(dockerArgs, args...)
		return exec.Command("docker", dockerArgs...)
	}
	binArgs := append(append([]string{}, args...), "--config", configPath)
	return exec.Command(binaryPath, binArgs...)
}

// startForwarder launches the forwarder in serve mode with the given config
// file. It returns a stop function that sends SIGTERM and waits for exit; the
// same function is also registered as a cleanup, so calling it is optional and
// safe to repeat.
func startForwarder(t *testing.T, configPath string) (stop func()) {
	t.Helper()

	cmd := forwarderCmd(configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting forwarder: %v", err)
	}

	var once sync.Once
	stop = func() {
		once.Do(func() {
			cmd.Process.Signal(syscall.SIGTERM)
			cmd.Wait()
		})
	}
	t.Cleanup(stop)
	return stop
}

// runForwarder runs a one-shot forwarder subcommand (e.g. register, deregister)
// to completion and fails the test if it exits non-zero.
func runForwarder(t *testing.T, configPath string, args ...string) {
	t.Helper()

	out, err := forwarderCmd(configPath, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("forwarder %v failed: %v\n%s", args, err, out)
	}
}

// waitForServer polls url until the server responds or the timeout elapses.
func waitForServer(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for server at %s", url)
}

// freePort finds and returns a free TCP port on localhost.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// forwarderConfig describes the config.yaml written for a forwarder under test.
type forwarderConfig struct {
	metadataURL string
	sinkURL     string
	publicURL   string
	listenAddr  string
	eventTypes  []string
	filters     []string
	// autoRegister, when non-nil, sets receiver.auto_register explicitly.
	// When nil the field is omitted and the forwarder applies its default.
	autoRegister *bool
}

// writeConfig writes a forwarder config.yaml to a temp file and returns its path.
// The file is world-readable so the Docker container user can read it when mounted.
func writeConfig(t *testing.T, cfg forwarderConfig) string {
	t.Helper()

	autoRegisterBlock := ""
	if cfg.autoRegister != nil {
		autoRegisterBlock = fmt.Sprintf("  auto_register: %t\n", *cfg.autoRegister)
	}

	var eventsBlock strings.Builder
	for _, et := range cfg.eventTypes {
		fmt.Fprintf(&eventsBlock, "    - %s\n", et)
	}

	var filtersBlock strings.Builder
	if len(cfg.filters) > 0 {
		filtersBlock.WriteString("    filters:\n")
		for _, f := range cfg.filters {
			fmt.Fprintf(&filtersBlock, "      - '%s'\n", f)
		}
	}

	content := fmt.Sprintf(`receiver:
  public_url: %q
  listen_addr: %q
  endpoint: /events
%s
transmitter:
  metadata_url: %q
  auth:
    type: bearer
    token: test-token
  events_requested:
%s
sinks:
  - type: webhook
    url: %q
%s`, cfg.publicURL, cfg.listenAddr, autoRegisterBlock, cfg.metadataURL, eventsBlock.String(), cfg.sinkURL, filtersBlock.String())

	f, err := os.CreateTemp("", "ssf-forwarder-config-*.yaml")
	if err != nil {
		t.Fatalf("creating config temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	f.Close()

	// World-readable so the container user (uid 1000) can read the mounted file.
	if err := os.Chmod(f.Name(), 0644); err != nil {
		t.Fatalf("chmod config: %v", err)
	}

	return f.Name()
}
