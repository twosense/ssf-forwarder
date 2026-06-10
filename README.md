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
  auto_register: true                          # default: true; set false to manage stream registration out of band

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

By default, all sinks receive every event (see [Filtering events](#filtering-events) to scope a sink to a subset). Delivery to each sink is attempted concurrently.

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

### Filtering events

By default, every sink receives every event. Add a `filters` list to a sink to forward only the events it should receive. Each filter is an [`expr`](https://expr-lang.org/) expression that must evaluate to a boolean, and a SET is forwarded to the sink only if **all** of its filters return `true`. A sink with no `filters` (or an empty list) receives everything.

Each expression is evaluated against three variables:

| Variable | Type | Description |
|---|---|---|
| `event_type` | string | The event's type URI (the key under the SET's `events` claim) |
| `event` | map | The event's payload, e.g. `event.current_level` |
| `claims` | map | The full decoded SET, e.g. `claims.iss`, `claims.sub_id.sub` |

For example, to forward only risk-level-change events where the risk level is `HIGH` to a webhook, while still logging everything:

```yaml
sinks:
  - type: webhook
    url: "https://davinci.example.com/events"
    filters:
      - 'event_type == "https://schemas.openid.net/secevent/caep/event-type/risk-level-change"'
      - 'event.current_level == "HIGH"'
  - type: log   # no filters: receives every event
```

Filters within a sink are evaluated in order and short-circuit at the first one that returns `false`, so put the `event_type` check first.

A SET almost always carries a single event. In the rare case one carries multiple, `event` and `event_type` are unavailable (a warning is logged) — use `claims.events` to address them directly.

Filters are compiled when the config loads, so a malformed expression, or one that doesn't return a boolean, fails startup. At runtime a missing field evaluates to `false` rather than erroring; if a filter references a field that isn't present in the SET, a warning is logged so a typo'd path doesn't silently drop events.

## Usage

```sh
ssf-forwarder --config config.yaml
```

The `--config` flag defaults to `config.yaml` in the current directory.

On startup, the service:
1. Fetches transmitter metadata from `metadata_url`
2. If `auto_register` is `true` (the default), registers a push stream with the transmitter. If one is already registered it reuses that stream and logs a warning, since that usually means either a previous run shut down uncleanly or another instance is already running
3. Starts listening for incoming SETs

On shutdown (SIGINT/SIGTERM), if `auto_register` is `true`, the stream is deleted from the transmitter before the process exits.

### Subcommands

**`ssf-forwarder register`** — registers (or updates) the stream for `public_url + endpoint`. Idempotent; safe to re-run. Re-run after changing `public_url` or `events_requested` to update the existing stream in place.

**`ssf-forwarder deregister`** — deletes the stream. Idempotent.

**`ssf-forwarder` / `ssf-forwarder serve`** — runs the receiver (default).

### Running multiple instances (horizontal scaling)

Point a load balancer at every instance and use its address as the shared `public_url`. Opt every instance out of boot registration — leave even one instance auto-registering and it will delete the shared stream when it shuts down or restarts, cutting off delivery to the others:

```yaml
receiver:
  public_url: "https://ssf-forwarder.example.com"  # the load balancer
  auto_register: false
```

Register the stream once during provisioning (safe to re-run; updates in place if the URL or event types changed):

```sh
ssf-forwarder register
```

Then start the instances normally (`ssf-forwarder`). Each warns on boot if no stream is registered for its `public_url`, but keeps serving. To tear the integration down, run `ssf-forwarder deregister` once.

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
