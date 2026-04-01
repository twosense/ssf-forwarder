package main

import (
	"fmt"
	"log/slog"

	"github.com/twosense/ssf-forwarder/internal/config"
	"github.com/twosense/ssf-forwarder/internal/sink"
)

func buildSinks(sinkCfgs []config.SinkConfig) ([]sink.Sink, error) {
	sinks := make([]sink.Sink, 0, len(sinkCfgs))

	for i, sc := range sinkCfgs {
		switch sc.Type {
		case "webhook":
			ws, err := sink.NewWebhookSink(sc.URL, sc.Headers, sc.BodyTemplate)
			if err != nil {
				return nil, fmt.Errorf("sinks[%d]: %w", i, err)
			}

			sinks = append(sinks, ws)
		case "log":
			sinks = append(sinks, sink.NewLogSink(slog.Default()))
		default:
			return nil, fmt.Errorf("sinks[%d]: unsupported type %q", i, sc.Type)
		}
	}

	return sinks, nil
}
