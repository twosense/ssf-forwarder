# Deploying with Docker

The Docker image for `ssf-forwarder` is available at `ghcr.io/twosense/ssf-forwarder:latest`. You can use it with any platforms that support OCI images, but we have provided some examples below.

## Docker

```sh
docker run --rm \
  -v /path/to/config.yaml:/etc/ssf-forwarder/config.yaml:ro \
  -p 8080:8080 \
  ghcr.io/twosense/ssf-forwarder:latest
```

The default config path inside the container is `/etc/ssf-forwarder/config.yaml`. Override it with `--config`:

```sh
docker run --rm \
  -v /path/to/config.yaml:/config.yaml:ro \
  -p 8080:8080 \
  ghcr.io/twosense/ssf-forwarder:latest --config /config.yaml
```

## Docker Compose

Copy the example config and fill in your values:

```sh
cp config.example.yaml config.yaml
```

Then start the service:

```sh
docker compose up
```

`config.yaml` is gitignored to avoid accidentally committing credentials.

## Running multiple instances

Set `auto_register: false` and point every instance's `public_url` at your load balancer, then register the stream once before starting the services:

```sh
docker compose run --rm ssf-forwarder register
docker compose up -d
```

The config path is set via the `SSF_FORWARDER_CONFIG_PATH` env var baked into the image (`/etc/ssf-forwarder/config.yaml`), so subcommands pick it up automatically — no `--config` needed.

`register` is idempotent — re-run it after changing `public_url` or `events_requested`. To remove the stream:

```sh
docker compose run --rm ssf-forwarder deregister
```
