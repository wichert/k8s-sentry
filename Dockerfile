# syntax=docker/dockerfile:1.4
### builder ###
FROM golang:1.20-bullseye AS builder

WORKDIR /app
# Copy the Go Modules
COPY --link go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY . .
# Build
ARG GOOS=linux
ARG GOARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags="-s -w" -trimpath -tags timetzdata -o k8s-sentry

### runner ###
FROM gcr.io/distroless/static-debian11:nonroot

LABEL org.opencontainers.image.authors="Kohei Ota <kela@inductor.me>"
LABEL org.opencontainers.image.url="https://github.com/inductor/k8s-sentry"
LABEL org.opencontainers.image.source="https://github.com/inductor/k8s-sentry/blob/main/Dockerfile"
COPY --from=builder /app/k8s-sentry /

CMD ["/k8s-sentry"]
