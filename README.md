# SSF Forwarder

A lightweight [Shared Signals Framework](https://openid.net/specs/openid-sharedsignals-framework-1_0.html) receiver based on [SGNL's `ssfreceiver` library](https://github.com/SGNL-ai/caep.dev/tree/main/ssfreceiver) that validates incoming Security Event Tokens (SETs) and forwards them to one or more sinks.

## Deployment

For easy deployment, see the [Docker deployment guide](./docs/deployment/docker.md).

### Recipes

We've created some guides for common use cases we call "recipes":

- [Forward CAEP Events to PingOne DaVinci](./docs/recipes/pingone-davinci/recipe.md)
- [Test `ssf-forwarder` with caep.dev](./docs/recipes/caep-dev/recipe.md)

## Requirements

- Go 1.26+
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

### Transmitter authentication

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

By default, the raw SET (the JWT string) is POST-ed to the webhook URL, with the original `Content-Type` header forwarded. The request is retried up to three times with exponential backoff.

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
    headers:
      "Content-Type": "application/json"
    body_template: |
      {"token": "{{.RawToken}}", "issuer": "{{index .Claims "iss"}}"}
```

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

## Supported Events

`ssf-forwarder` supports all CAEP event types defined in the [CAEP specification](https://openid.net/specs/openid-caep-1_0.html).

| Event type | URI |
|---|---|
| Session Revoked | `https://schemas.openid.net/secevent/caep/event-type/session-revoked` |
| Token Claims Change | `https://schemas.openid.net/secevent/caep/event-type/token-claims-change` |
| Credential Change | `https://schemas.openid.net/secevent/caep/event-type/credential-change` |
| Assurance Level Change | `https://schemas.openid.net/secevent/caep/event-type/assurance-level-change` |
| Device Compliance Change | `https://schemas.openid.net/secevent/caep/event-type/device-compliance-change` |
| Session Established | `https://schemas.openid.net/secevent/caep/event-type/session-established` |
| Session Presented | `https://schemas.openid.net/secevent/caep/event-type/session-presented` |
| Risk Level Change | `https://schemas.openid.net/secevent/caep/event-type/risk-level-change` |
| Verification | `https://schemas.openid.net/secevent/ssf/event-type/verification` |
| Stream Updated | `https://schemas.openid.net/secevent/ssf/event-type/stream-updated` |

## Development

```sh
go test ./...
go vet ./...
```

### E2E tests

The end-to-end tests in `test/e2e/` run the real compiled binary against an in-process fake transmitter and webhook sink. They are excluded from `go test ./...` by a build tag and must be run explicitly:

```sh
go test -tags e2e -count 1 ./test/e2e/...
```

The test builds the binary from source automatically — no extra setup required.

You can also run the E2E tests against the built Docker image. This requires host networking, so it will only work on Linux:

```sh
E2E_DOCKER=1 go test -tags e2e -count 1 ./test/e2e/...
```
