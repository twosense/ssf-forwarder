package main

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantCommand    string
		wantConfigPath string
		wantErr        string
	}{
		{
			name:           "no args defaults to serve",
			args:           nil,
			wantCommand:    "serve",
			wantConfigPath: "default.yaml",
		},
		{
			name:           "explicit serve",
			args:           []string{"serve"},
			wantCommand:    "serve",
			wantConfigPath: "default.yaml",
		},
		{
			name:           "register with config flag",
			args:           []string{"register", "-config", "custom.yaml"},
			wantCommand:    "register",
			wantConfigPath: "custom.yaml",
		},
		{
			name:           "deregister",
			args:           []string{"deregister"},
			wantCommand:    "deregister",
			wantConfigPath: "default.yaml",
		},
		{
			name:           "config flag without command runs serve",
			args:           []string{"-config", "custom.yaml"},
			wantCommand:    "serve",
			wantConfigPath: "custom.yaml",
		},
		{
			name:    "command after flags is an error, not silently ignored",
			args:    []string{"-config", "custom.yaml", "register"},
			wantErr: "must come before",
		},
		{
			name:    "unknown command",
			args:    []string{"bogus"},
			wantErr: "unknown command",
		},
		{
			name:    "trailing positional after command",
			args:    []string{"register", "extra"},
			wantErr: "must come before",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, configPath, err := parseArgs(tt.args, "default.yaml")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseArgs(%v) = (%q, %q, nil), want error containing %q", tt.args, command, configPath, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseArgs(%v) error = %q, want it to contain %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs(%v): %v", tt.args, err)
			}
			if command != tt.wantCommand {
				t.Errorf("command = %q, want %q", command, tt.wantCommand)
			}
			if configPath != tt.wantConfigPath {
				t.Errorf("configPath = %q, want %q", configPath, tt.wantConfigPath)
			}
		})
	}
}
