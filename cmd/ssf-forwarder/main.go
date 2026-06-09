package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"

	_ "github.com/twosense/ssf-forwarder/internal/caepext" // Register custom CAEP event parsers
	"github.com/twosense/ssf-forwarder/internal/config"
)

func main() {
	defaultConfigPath := os.Getenv("SSF_FORWARDER_CONFIG_PATH")
	if defaultConfigPath == "" {
		defaultConfigPath = "config.yaml"
	}

	args := os.Args[1:]
	command := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet(command, flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	cfg, err := config.Load(*configPath)
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
	default:
		slog.Error("unknown command", "command", command, "valid", "serve, register, deregister")
		os.Exit(2)
	}
}
