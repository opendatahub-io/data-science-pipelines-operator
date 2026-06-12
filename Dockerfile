# Build the manager binary
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/go-toolset:1.26.3@sha256:d36470d5258da00f618b7aca9bdaab8e05134aa938bd6c42d9bd17d50ed45e76 AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN GOTOOLCHAIN=local go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

# Build
USER root
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOFIPS140=v1.0.0 \
    go build -tags no_openssl -a -o manager main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ENV GODEBUG=fips140=auto

WORKDIR /
COPY --from=builder /workspace/manager .
COPY config/internal config/internal

ARG USER=65532
USER ${USER}:${USER}

ENTRYPOINT ["/manager"]
