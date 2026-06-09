FROM golang:1.26.1-alpine3.22 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /ssf-forwarder ./cmd/ssf-forwarder


FROM alpine:3.22

RUN apk --no-cache add ca-certificates

RUN adduser -D -u 1000 app
USER app

COPY --from=builder /ssf-forwarder /usr/local/bin/ssf-forwarder

ENV SSF_FORWARDER_CONFIG_PATH=/etc/ssf-forwarder/config.yaml

ENTRYPOINT ["ssf-forwarder"]
