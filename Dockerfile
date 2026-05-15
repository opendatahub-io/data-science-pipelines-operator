# Build the manager binary
ARG BUILDER_ARCH=multiarch
FROM registry.access.redhat.com/ubi9/go-toolset:1.26.3@sha256:45bd06d392349dfdb09073514cfebcc5082db33301347b51dcae8e5516a89126 AS go-toolset-multiarch
FROM registry.access.redhat.com/ubi9/go-toolset:1.26.3@sha256:3880436381044c6d89c2311f2a7dc8d64224668bf84b43810982b5a9ddd851d0 AS go-toolset-arm64
FROM go-toolset-${BUILDER_ARCH} AS builder
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


WORKDIR /
COPY --from=builder /workspace/manager .
COPY config/internal config/internal

ARG USER=65532
USER ${USER}:${USER}

ENTRYPOINT ["/manager"]
