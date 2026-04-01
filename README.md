# SSF Forwarder

A lightweight [Shared Signals Framework](https://openid.net/specs/openid-sharedsignals-framework-1_0.html) receiver that validates incoming Security Event Tokens (SETs) and forwards them to one or more sinks.

- Push delivery only (no polling)
- Single transmitter per config file
- Optional request rewriting via Go templates

## Requirements

- Go 1.24+
- A running SSF transmitter that supports push delivery

## Installation

```sh
go install github.com/twosense/ssf-forwarder/cmd/ssf-forwarder@latest
```

Or build from source:

```sh
git clone https://github.com/twosense/ssf-forwarder
cd ssf-forwarder
go build -o ssf-forwarder ./cmd/ssf-forwarder
```

## Configuration

Create a YAML config file. The only required fields are `receiver.public_url`, `transmitter.metadata_url`, `transmitter.auth`, and at least one sink.

```yaml
receiver:
  public_url: "https://receiver.example.com"   # externally reachable URL for this service
  listen_addr: ":8080"                         # default: :8080
  endpoint: "/events"                          # default: /events

transmitter:
  metadata_url: "https://transmitter.example.com/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "your-token-here"
  events_requested:
    - "https://schemas.openid.net/secevent/caep/event-type/session-revoked"
    - "https://schemas.openid.net/secevent/caep/event-type/credential-change"

sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
```

The service registers `public_url + endpoint` as the push delivery URL when it connects to the transmitter. Make sure that address is reachable by the transmitter.

### Authentication

**Bearer token:**

```yaml
transmitter:
  auth:
    type: bearer
    token: "your-token-here"
```

**OAuth2 client credentials:**

```yaml
transmitter:
  auth:
    type: oauth2
    token_url: "https://auth.example.com/token"
    client_id: "your-client-id"
    client_secret: "your-client-secret"
```

### Webhook sink options

By default, the raw SET (the JWT string) is POST-ed to the webhook URL, with the original `Content-Type` header forwarded.

**Add or override headers:**

```yaml
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
    headers:
      Authorization: "Bearer sink-token"
      X-Source: "ssf-forwarder"
```

**Rewrite the request body with a Go template:**

The template has access to `.RawToken` (the raw JWT string) and `.Claims` (a map of the decoded JWT payload claims).

```yaml
sinks:
  - type: webhook
    url: "https://webhook.example.com/events"
    body_template: |
      {"token": "{{.RawToken}}", "issuer": "{{index .Claims "iss"}}"}
```

When a `body_template` is set, the outgoing `Content-Type` is set to `application/json`.

**Multiple sinks:**

All sinks receive every event. Delivery to each sink is attempted concurrently.

```yaml
sinks:
  - type: webhook
    url: "https://first.example.com/events"
  - type: webhook
    url: "https://second.example.com/events"
    headers:
      Authorization: "Bearer other-token"
```

### Log sink

The `log` sink prints structured information about each received SET to stdout. Useful for debugging or as an audit trail alongside other sinks.

```yaml
sinks:
  - type: log
```

Each event is logged with the following fields: `issuer`, `jti`, `iat`, `event_types`, and `txn` (if present).

## Usage

```sh
ssf-forwarder --config config.yaml
```

The `--config` flag defaults to `config.yaml` in the current directory.

On startup, the service:
1. Fetches transmitter metadata from `metadata_url`
2. Registers a push stream with the transmitter (or reuses an existing one)
3. Starts listening for incoming SETs

On shutdown (SIGINT/SIGTERM), the stream is deleted from the transmitter before the process exits.

## Development

```sh
go test ./...
go vet ./...
```
