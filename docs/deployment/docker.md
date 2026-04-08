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
