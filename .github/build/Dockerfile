ARG CI_BASE=registry.access.redhat.com/ubi8/go-toolset:1.20

FROM ${CI_BASE} as stage1

ARG GOLANGCI_LINT_VERSION=1.54.2
ARG REVIVE_VERSION=v1.2.5
ARG STATICCHECK_VERSION=v0.3.3
ARG INEFFASSIGN_VERSION=13ace05
ARG ERRCHECK_VERSION=v1.6.3
ARG GHCLI_VERSION=2.23.0
ARG SETUP_ENVTEST=release-0.16

ENV GOPATH=/build \
  GO111MODULE=on \
    PATH=/build/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

WORKDIR /build/src

USER root

RUN dnf -y install \
    golang \
    git

RUN go install sigs.k8s.io/controller-runtime/tools/setup-envtest@${SETUP_ENVTEST}
RUN go install github.com/mgechev/revive@${REVIVE_VERSION}
RUN go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}
RUN go install github.com/gordonklaus/ineffassign@${INEFFASSIGN_VERSION}
RUN go install github.com/kisielk/errcheck@${ERRCHECK_VERSION}

RUN curl -L -o /tmp/golangci-lint.tar.gz \
    https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz
RUN tar -C /tmp -xv --strip-components=1 -f /tmp/golangci-lint.tar.gz && \
    cp /tmp/golangci-lint /build/bin/

RUN curl -L -o /tmp/ghcli.tar.gz https://github.com/cli/cli/releases/download/v${GHCLI_VERSION}/gh_${GHCLI_VERSION}_linux_amd64.tar.gz
RUN tar -C /tmp -xv --strip-components=1 -f /tmp/ghcli.tar.gz && \
  cp /tmp/bin/gh /build/bin/

FROM ${CI_BASE}

ENV SUMMARY="Toolchain for running pre-commit hooks." \
    DESCRIPTION="Toolchain for running pre-commit hooks." \
    PATH=/build/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
  GO111MODULE=on \
  ENVTEST_K8S_VERSION=1.25.0

LABEL summary="$SUMMARY" \
    description="$DESCRIPTION" \
    io.k8s.description="$DESCRIPTION" \
    io.k8s.display-name="Pre-Commit Toolchain" \
    io.openshift.tags="pre-commit"

COPY --from=stage1 /build/bin /build/bin

USER root

RUN dnf -y install \
    python39 \
    python3-pip \
    golang \
    git \
    make && \
    /build/bin/setup-envtest use ${ENVTEST_K8S_VERSION} --bin-dir /build/bin -p path && \
    dnf clean all

WORKDIR /build/src

ENV KUBEBUILDER_ASSETS=/build/bin/k8s/${ENVTEST_K8S_VERSION}-linux-amd64

COPY requirements.txt /tmp/requirements.txt
RUN python3 -m venv /build

RUN . /build/bin/activate && \
    python3 -m pip install -U pip && \
    python3 -m pip install -r /tmp/requirements.txt
