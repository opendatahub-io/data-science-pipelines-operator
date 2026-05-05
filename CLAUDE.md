# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

Data Science Pipelines Operator (DSPO) is a Kubernetes operator built with kubebuilder/controller-runtime that manages `DataSciencePipelinesApplication` (DSPA) custom resources on OpenShift. Each DSPA deploys a namespace-scoped Kubeflow Pipelines v2 stack: API server, persistence agent, scheduled workflow controller, Argo workflow controller, MLMD, and optional MariaDB/Minio.

## Build and Test Commands

```bash
make build                    # Build the manager binary
make manifests                # Regenerate CRDs, RBAC, webhooks from kubebuilder markers
make generate                 # Regenerate DeepCopy methods after API type changes
make generate manifests       # Both, after API changes

make unittest                 # Unit tests only (fast, no TLS certs needed)
make functest                 # Functional tests (needs envtest + TLS certs)
make test                     # All tests (unit + functional)

# Run a single test file or specific test (use build tags):
KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.34.0 --bin-dir bin -p path)" \
  go test ./controllers/ -v --tags=test_unit -run TestExtractParams

make fmt                      # go fmt
make vet                      # go vet
golangci-lint run ./...       # Lint (config in .golangci.yaml)
pre-commit run --all-files    # All pre-commit checks
```

## Test Build Tags

Tests are gated by build tags, not by directory alone:
- `test_unit` — unit tests in `controllers/` (no envtest, no cluster)
- `test_functional` — functional tests in `controllers/` (use envtest with a real API server)
- `test_integration` — integration tests in `tests/` (need a running cluster)
- `test_all` — unit + functional combined

Unit test files use `//go:build test_all || test_unit`. Functional tests use `//go:build test_all || test_functional`. Always include the appropriate tag when adding new test files.

## Architecture

### Reconcile Loop

`DSPAReconciler.Reconcile()` in `controllers/dspipeline_controller.go` is the single entry point. The flow is:

1. **ExtractParams** (`dspipeline_params.go`) — resolves all CR fields, operator config (viper), secrets, configmaps, and TLS certs into a `DSPAParams` struct. Failure here blocks the entire reconcile.
2. **Health checks** — verifies DB and object store connectivity before deploying workloads.
3. **Component reconciliation** — in order: Common, Webhook (if kubernetes pipeline store), ManagedPipelines validation, APIServer, PersistenceAgent, ScheduledWorkflow, WorkflowController, MLMD (last, because it depends on TLS secrets created by earlier components).
4. **Status update** — deferred; sets conditions and publishes Prometheus metrics.

Each component reconciler (e.g., `ReconcileAPIServer` in `apiserver.go`) applies Go templates from `config/internal/<component>/` via manifestival.

### Key Packages

- `api/v1/` — CRD types (`DataSciencePipelinesApplication`, `DSPASpec`, `DSPAStatus`)
- `controllers/` — reconciler, per-component files, params extraction
- `controllers/config/` — operator defaults, config field names, template loading
- `controllers/dspastatus/` — status condition builder
- `controllers/util/` — shared helpers (secret/configmap retrieval, label selectors)
- `config/internal/` — Go templates (`.yaml.tmpl`) for Kubernetes manifests, organized by component

### Manifest Templating

Kubernetes resources are defined as Go templates in `config/internal/<component>/`. The reconciler renders them with `DSPAParams` fields via manifestival, then applies owner references and `dsp-version` labels. When adding a new template, place it in the correct component subdirectory and reference it from the component's reconciler.

### Operator Configuration

The operator reads its config via viper from a config file (`--config` flag) and environment variables. Image references come from `IMAGES_*` env vars (e.g., `IMAGES_APISERVER`). The `config/base/params.env` file provides defaults. `controllers/config/defaults.go` defines fallback values and required fields.

### Cache Optimization

The controller manager uses scoped informer caches (see `main.go`): ConfigMap and Secret data payloads are stripped from the cache and read via direct API calls. Operator-managed resources are filtered by `dsp-version` label. Pod watches are scoped to `component=data-science-pipelines` label.
