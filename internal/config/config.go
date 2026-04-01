package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Receiver    ReceiverConfig    `yaml:"receiver"`
	Transmitter TransmitterConfig `yaml:"transmitter"`
	Sinks       []SinkConfig      `yaml:"sinks"`
}

type ReceiverConfig struct {
	ListenAddr string `yaml:"listen_addr"`
	PublicURL  string `yaml:"public_url"`
	Endpoint   string `yaml:"endpoint"`
}

type TransmitterConfig struct {
	MetadataURL     string   `yaml:"metadata_url"`
	Auth            AuthConfig `yaml:"auth"`
	EventsRequested []string `yaml:"events_requested"`
}

type AuthConfig struct {
	Type         string `yaml:"type"`
	Token        string `yaml:"token"`
	TokenURL     string `yaml:"token_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type SinkConfig struct {
	Type         string            `yaml:"type"`
	URL          string            `yaml:"url"`
	Headers      map[string]string `yaml:"headers"`
	BodyTemplate string            `yaml:"body_template"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cfg.applyDefaults()

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Receiver.PublicURL == "" {
		return fmt.Errorf("receiver.public_url is required")
	}

	if c.Transmitter.MetadataURL == "" {
		return fmt.Errorf("transmitter.metadata_url is required")
	}

	switch c.Transmitter.Auth.Type {
	case "bearer":
		if c.Transmitter.Auth.Token == "" {
			return fmt.Errorf("transmitter.auth.token is required for bearer auth")
		}
	case "oauth2":
		if c.Transmitter.Auth.TokenURL == "" {
			return fmt.Errorf("transmitter.auth.token_url is required for oauth2 auth")
		}
		if c.Transmitter.Auth.ClientID == "" {
			return fmt.Errorf("transmitter.auth.client_id is required for oauth2 auth")
		}
		if c.Transmitter.Auth.ClientSecret == "" {
			return fmt.Errorf("transmitter.auth.client_secret is required for oauth2 auth")
		}
	default:
		return fmt.Errorf("transmitter.auth.type must be 'bearer' or 'oauth2'")
	}

	if len(c.Sinks) == 0 {
		return fmt.Errorf("at least one sink is required")
	}

	for i, sink := range c.Sinks {
		switch sink.Type {
		case "webhook":
			if sink.URL == "" {
				return fmt.Errorf("sinks[%d].url is required", i)
			}
		case "log":
			// no required fields
		default:
			return fmt.Errorf("sinks[%d].type: unsupported sink type %q", i, sink.Type)
		}
	}

	return nil
}

func (c *Config) applyDefaults() {
	if c.Receiver.ListenAddr == "" {
		c.Receiver.ListenAddr = ":8080"
	}

	if c.Receiver.Endpoint == "" {
		c.Receiver.Endpoint = "/events"
	}
}
