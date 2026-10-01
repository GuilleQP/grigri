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
# Distroless has no shell to mkdir with. /config receives the compiled
# AgentConfig; /data holds the session store (a Substrate DurableDir).
RUN mkdir -p /rootfs/config /rootfs/data

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app /app
COPY --from=builder --chown=65532:65532 /rootfs/ /
USER 65532:65532
ENTRYPOINT ["/app"]
