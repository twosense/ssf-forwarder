# Recipe: Test `ssf-forwarder` with [caep.dev](https://caep.dev/)

[caep.dev](https://caep.dev/) is a service which provides resources and tools for developers to implement and test SSF transmitters and receivers. You can use their CAEP Transmitter tool to verify your deployment of `ssf-forwarder` is properly accessible.

## Step 1. Register with [caep.dev](https://caep.dev/)

Register for a caep.dev access token [here](https://caep.dev/register).

## Step 2. Configure `ssf-forwarder`

Place the following in your `config.yaml`, replacing `YOUR_ACCESS_TOKEN` with the access token from caep.dev:

```yaml
transmitter:
  metadata_url: "https://ssf.caep.dev/.well-known/ssf-configuration"
  auth:
    type: bearer
    token: "YOUR_ACCESS_TOKEN"
  events_requested:
    - "https://schemas.openid.net/secevent/caep/event-type/session-revoked"
    - "https://schemas.openid.net/secevent/caep/event-type/credential-change"

sinks:
  - type: log
```

## Step 3. Test it out!

Start `ssf-forwarder` according to your deployment method, then visit the [caep.dev transmitter](https://caep.dev/transmitter/events), paste your access token, then try sending an event.

If successful, you should see a log message like the following:

```
2026/04/01 22:15:30 INFO received SET issuer=https://ssf.caep.dev/ jti=MWUzNjJlOTMtOTQyZS00ODBiLWIzYzYtMjc0ZTg2YmQ5NmVl iat=1.775081729e+09 event_types=[https://schemas.openid.net/secevent/ssf/event-type/verification]
```
