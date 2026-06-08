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
		var s sink.Sink
		var label string

		switch sc.Type {
		case "webhook":
			ws, err := sink.NewWebhookSink(sc.URL, sc.Headers, sc.BodyTemplate, slog.Default())
			if err != nil {
				return nil, fmt.Errorf("sinks[%d]: %w", i, err)
			}
			s = ws
			label = fmt.Sprintf("sinks[%d] (webhook %s)", i, sc.URL)
		case "log":
			s = sink.NewLogSink(slog.Default())
			label = fmt.Sprintf("sinks[%d] (log)", i)
		default:
			return nil, fmt.Errorf("sinks[%d]: unsupported type %q", i, sc.Type)
		}

		if len(sc.Filters) > 0 {
			filters, err := sink.CompileFilters(sc.Filters)
			if err != nil {
				return nil, fmt.Errorf("sinks[%d]: %w", i, err)
			}
			s = sink.NewFilteredSink(s, filters, label, slog.Default())
		}

		sinks = append(sinks, s)
	}

	return sinks, nil
}
