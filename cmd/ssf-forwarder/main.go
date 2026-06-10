package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	_ "github.com/twosense/ssf-forwarder/internal/caepext" // Register custom CAEP event parsers
	"github.com/twosense/ssf-forwarder/internal/config"
)

const usage = `usage: ssf-forwarder [command] [flags]

Commands:
  serve       receive and forward events (default)
  register    register the stream with the transmitter
  deregister  delete the registered stream

Flags:
  -config path
        path to config file (default $SSF_FORWARDER_CONFIG_PATH or config.yaml)
`

// parseArgs resolves the subcommand and flags from the command line. The
// subcommand must come before any flags, matching the convention of the go
// tool itself; a positional argument left over after flag parsing is an error
// rather than being silently ignored.
func parseArgs(args []string, defaultConfigPath string) (command, configPath string, err error) {
	command = "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}

	switch command {
	case "serve", "register", "deregister":
	default:
		return "", "", fmt.Errorf("unknown command %q", command)
	}

	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // main reports errors and usage itself
	configFlag := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	if fs.NArg() > 0 {
		return "", "", fmt.Errorf("unexpected argument %q: the command must come before any flags", fs.Arg(0))
	}

	return command, *configFlag, nil
}

func main() {
	defaultConfigPath := os.Getenv("SSF_FORWARDER_CONFIG_PATH")
	if defaultConfigPath == "" {
		defaultConfigPath = "config.yaml"
	}

	command, configPath, err := parseArgs(os.Args[1:], defaultConfigPath)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Print(usage)
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssf-forwarder: %v\n\n%s", err, usage)
		os.Exit(2)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("loading config", "err", err)
		os.Exit(1)
	}

	switch command {
	case "serve":
		runServe(cfg)
	case "register":
		runRegister(cfg)
	case "deregister":
		runDeregister(cfg)
	}
}
