package ssfadmin

import (
	"context"
	"fmt"
	"sort"

	"github.com/sgnl-ai/caep.dev/ssfreceiver/types"
)

// Action describes what Reconcile did.
type Action string

const (
	ActionCreated   Action = "created"
	ActionUpdated   Action = "updated"
	ActionUnchanged Action = "unchanged"
)

// FindOurStream returns the single stream owned by ssf-forwarder, or nil if none
// exists. It errors if more than one matching stream is found, since that
// requires manual cleanup.
func FindOurStream(streams []StreamConfig) (*StreamConfig, error) {
	var ours []StreamConfig
	for _, s := range streams {
		if s.Description == StreamDescription {
			ours = append(ours, s)
		}
	}
	switch len(ours) {
	case 0:
		return nil, nil
	case 1:
		return &ours[0], nil
	default:
		return nil, fmt.Errorf("found %d streams with description %q; manual cleanup required", len(ours), StreamDescription)
	}
}

// Reconcile ensures exactly one stream owned by ssf-forwarder exists with the
// given push URL and event types, creating or updating in place as needed.
func Reconcile(ctx context.Context, c *Client, pushURL string, eventTypes []string) (Action, *StreamConfig, error) {
	streams, err := c.List(ctx)
	if err != nil {
		return "", nil, err
	}

	existing, err := FindOurStream(streams)
	if err != nil {
		return "", nil, err
	}

	desired := StreamConfig{
		Description:     StreamDescription,
		Delivery:        DeliveryConfig{Method: string(types.DeliveryMethodPush), EndpointURL: pushURL},
		EventsRequested: eventTypes,
	}

	if existing == nil {
		created, err := c.Create(ctx, desired)
		return ActionCreated, created, err
	}

	if existing.Delivery.EndpointURL == pushURL && sameEventSet(existing.EventsRequested, eventTypes) {
		return ActionUnchanged, existing, nil
	}

	desired.StreamID = existing.StreamID
	updated, err := c.Update(ctx, desired)
	return ActionUpdated, updated, err
}

func sameEventSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

// SafeguardStatus is the outcome of a boot-time registration check.
type SafeguardStatus string

const (
	SafeguardOK          SafeguardStatus = "ok"
	SafeguardMissing     SafeguardStatus = "missing"
	SafeguardURLMismatch SafeguardStatus = "url_mismatch"
)

// SafeguardResult is the result of Check. RegisteredURL is set only when the
// status is SafeguardURLMismatch.
type SafeguardResult struct {
	Status        SafeguardStatus
	RegisteredURL string
}

// Check reports whether a stream owned by ssf-forwarder is registered for the
// given push URL. It never mutates state.
func Check(ctx context.Context, c *Client, pushURL string) (SafeguardResult, error) {
	streams, err := c.List(ctx)
	if err != nil {
		return SafeguardResult{}, err
	}

	existing, err := FindOurStream(streams)
	if err != nil {
		return SafeguardResult{}, err
	}

	if existing == nil {
		return SafeguardResult{Status: SafeguardMissing}, nil
	}
	if existing.Delivery.EndpointURL != pushURL {
		return SafeguardResult{Status: SafeguardURLMismatch, RegisteredURL: existing.Delivery.EndpointURL}, nil
	}
	return SafeguardResult{Status: SafeguardOK}, nil
}
