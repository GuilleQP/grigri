# Static Go binary on distroless: no libc, no shell. A static binary survives
# Substrate snapshot/restore (kagent's Python runtime does not).
FROM --platform=$BUILDPLATFORM golang:1.27 AS builder
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /app ./cmd/kagent-harness

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app /app
USER 65532:65532
ENTRYPOINT ["/app"]
