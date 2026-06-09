// Package ssfadmin manages the lifecycle of an SSF stream by talking to the
// transmitter's configuration endpoint directly. It keys a stream to this
// deployment by a fixed description so that registration is idempotent and a
// changed push URL updates the existing stream instead of orphaning it.
package ssfadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sgnl-ai/caep.dev/ssfreceiver/auth"
	"github.com/sgnl-ai/caep.dev/ssfreceiver/types"
)

// StreamDescription identifies the stream owned by ssf-forwarder. It is a stable
// identity key: changing it would orphan a previously registered stream.
const StreamDescription = "twosense-ssf-forwarder"

// DeliveryConfig is the SSF stream delivery block.
type DeliveryConfig struct {
	Method      string `json:"method"`
	EndpointURL string `json:"endpoint_url"`
}

// StreamConfig is the subset of an SSF stream configuration ssf-forwarder needs.
// It serves as both the request body (create/update) and the decoded response.
type StreamConfig struct {
	StreamID        string         `json:"stream_id,omitempty"`
	Description     string         `json:"description,omitempty"`
	Delivery        DeliveryConfig `json:"delivery"`
	EventsRequested []string       `json:"events_requested,omitempty"`
}

// Client talks to a transmitter's SSF stream configuration endpoint.
type Client struct {
	httpClient     *http.Client
	configEndpoint string
	authorizer     auth.Authorizer
}

// NewClient fetches the transmitter metadata to discover the configuration
// endpoint and returns a Client ready to manage streams.
func NewClient(ctx context.Context, metadataURL string, authorizer auth.Authorizer) (*Client, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating metadata request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading metadata: %w", err)
	}

	var meta types.TransmitterMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("decoding metadata: %w", err)
	}

	endpoint := meta.GetConfigurationEndpoint()
	if endpoint == nil {
		return nil, fmt.Errorf("metadata missing configuration_endpoint")
	}

	return &Client{
		httpClient:     httpClient,
		configEndpoint: endpoint.String(),
		authorizer:     authorizer,
	}, nil
}

// List returns all streams the authenticated client can see.
func (c *Client) List(ctx context.Context) ([]StreamConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.configEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating list request: %w", err)
	}
	if err := c.authorizer.AddAuth(ctx, req); err != nil {
		return nil, fmt.Errorf("adding auth: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing streams: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list streams returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading list response: %w", err)
	}

	var streams []StreamConfig
	if err := json.Unmarshal(body, &streams); err == nil {
		return streams, nil
	}

	var single StreamConfig
	if err := json.Unmarshal(body, &single); err != nil {
		return nil, fmt.Errorf("decoding stream configuration: %w", err)
	}
	return []StreamConfig{single}, nil
}

func (c *Client) doJSON(ctx context.Context, method string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.configEndpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("creating %s request: %w", method, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.authorizer.AddAuth(ctx, req); err != nil {
		return nil, fmt.Errorf("adding auth: %w", err)
	}
	return c.httpClient.Do(req)
}
