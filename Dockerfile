# syntax=docker/dockerfile:1.6

# ─── build stage ────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .

ARG VERSION=docker
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build \
      -trimpath \
      -ldflags="-s -w -X github.com/mizcausevic-dev/kg-utm-attribution/internal/server.Version=${VERSION}" \
      -o /out/kg-utm-attribution \
      ./cmd/kg-utm-attribution

# ─── runtime stage ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/kg-utm-attribution /kg-utm-attribution

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/kg-utm-attribution"]
