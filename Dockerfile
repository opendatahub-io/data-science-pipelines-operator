# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG FIPS_ENABLED=1

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

# Build
# FIPS_ENABLED=1 (default): FIPS-compliant build with strictfipsruntime (requires CGO)
# FIPS_ENABLED=0: Non-FIPS build for local development on Apple Silicon (pure Go, no CGO)
USER root
RUN if [ "${FIPS_ENABLED}" != "0" ] && [ "${FIPS_ENABLED}" != "1" ]; then \
      echo "ERROR: FIPS_ENABLED must be '0' or '1', got '${FIPS_ENABLED}'" && exit 1; \
    elif [ "${FIPS_ENABLED}" = "1" ]; then \
      CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GO111MODULE=on \
        GOEXPERIMENT=strictfipsruntime go build -tags strictfipsruntime -a -o manager main.go; \
    else \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GO111MODULE=on \
        go build -a -o manager main.go; \
    fi

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
WORKDIR /
COPY --from=builder /workspace/manager .
COPY config/internal config/internal

ENTRYPOINT ["/manager"]
