FROM golang:1.26.1-alpine3.21 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /ssf-forwarder ./cmd/ssf-forwarder


FROM alpine:3.21

RUN apk --no-cache add ca-certificates

RUN adduser -D -u 1000 app
USER app

COPY --from=builder /ssf-forwarder /usr/local/bin/ssf-forwarder

ENTRYPOINT ["ssf-forwarder"]
CMD ["--config", "/etc/ssf-forwarder/config.yaml"]
