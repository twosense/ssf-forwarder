//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const dockerImage = "ssf-forwarder:e2e-test"

var (
	binaryPath string
	useDocker  = os.Getenv("E2E_DOCKER") == "1"
)

func TestMain(m *testing.M) {
	var cleanup func()

	if useDocker {
		cmd := exec.Command("docker", "build", "-t", dockerImage, "../..")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "docker build failed: %v\n%s\n", err, out)
			os.Exit(1)
		}
		cleanup = func() {}
	} else {
		dir, err := os.MkdirTemp("", "ssf-forwarder-e2e-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
			os.Exit(1)
		}
		binaryPath = filepath.Join(dir, "ssf-forwarder")
		cmd := exec.Command("go", "build", "-o", binaryPath, "github.com/twosense/ssf-forwarder/cmd/ssf-forwarder")
		if out, err := cmd.CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			fmt.Fprintf(os.Stderr, "binary build failed: %v\n%s\n", err, out)
			os.Exit(1)
		}
		cleanup = func() { os.RemoveAll(dir) }
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}
