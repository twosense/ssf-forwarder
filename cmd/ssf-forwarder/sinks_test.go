package main

import (
	"strings"
	"testing"

	"github.com/twosense/ssf-forwarder/internal/config"
)

func TestBuildSinks(t *testing.T) {
	tests := []struct {
		name    string
		cfgs    []config.SinkConfig
		wantN   int
		wantErr string
	}{
		{
			name:  "webhook sink",
			cfgs:  []config.SinkConfig{{Type: "webhook", URL: "https://example.com/hook"}},
			wantN: 1,
		},
		{
			name:  "log sink",
			cfgs:  []config.SinkConfig{{Type: "log"}},
			wantN: 1,
		},
		{
			name: "multiple sinks",
			cfgs: []config.SinkConfig{
				{Type: "log"},
				{Type: "webhook", URL: "https://example.com/hook"},
			},
			wantN: 2,
		},
		{
			name:    "unsupported type",
			cfgs:    []config.SinkConfig{{Type: "kafka"}},
			wantErr: `unsupported type "kafka"`,
		},
		{
			name:    "webhook with invalid body template",
			cfgs:    []config.SinkConfig{{Type: "webhook", URL: "https://example.com", BodyTemplate: "{{.Unclosed"}},
			wantErr: "parsing body_template",
		},
		{
			name:  "empty slice returns empty result",
			cfgs:  nil,
			wantN: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange — cfgs already set above

			// Act
			sinks, err := buildSinks(tc.cfgs)

			// Assert
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(sinks) != tc.wantN {
				t.Errorf("len(sinks) = %d, want %d", len(sinks), tc.wantN)
			}
		})
	}
}
