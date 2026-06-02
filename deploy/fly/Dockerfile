# Fly.io image: remotr-server + bundled bootstrap config-repo.
# Build context: repository root (see deploy/fly/fly.toml).

ARG GO_VERSION=1.26-alpine
ARG ALPINE_VERSION=3.21

FROM golang:${GO_VERSION} AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd/remotr-server ./cmd/remotr-server
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" \
    -o /out/remotr-server ./cmd/remotr-server

FROM alpine:${ALPINE_VERSION}
RUN apk add --no-cache ca-certificates git
COPY --from=build /out/remotr-server /usr/local/bin/remotr-server
COPY docker/remotr-server/entrypoint.sh /usr/local/bin/remotr-server-entrypoint
COPY deploy/fly/config-repo /config-repo
RUN chmod 755 /usr/local/bin/remotr-server-entrypoint
EXPOSE 8443
ENTRYPOINT ["/usr/local/bin/remotr-server-entrypoint"]
