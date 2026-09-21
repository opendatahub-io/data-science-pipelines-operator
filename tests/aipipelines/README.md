# AIPipelines module integration and E2E tests

DSPO owns AIPipelines reconciliation and a **small** live-cluster lifecycle suite.
Operand user flows, deployment health, and API reachability are covered by the
existing DSP integration suite when run with `tests.sh --kind --modular`.

## Traceability

| Required responsibility | Automated coverage | Execution and evidence status |
| --- | --- | --- |
| Deployment health, API reachability, user flows | Existing `make integrationtest` / `tests.sh` modular path on the `test-dspa` DSPA | Modular Kind CI after `enable_aipipelines_module`. |
| Webhook behavior | `tests/webhook_test.go` rejects immutable PipelineVersion updates | Modular Kind integration suite. |
| Module status (`observedGeneration`, Ready, ProvisioningSucceeded, platform release) | `TestAIPipelinesLifecycle/reports_live_module_status` via `waitModule` | Modular Kind CI (`make aipipelines-e2e-test`). |
| Argo `managementState` Managed → Removed → Managed | `TestAIPipelinesLifecycle/projects_management_state` on integration DSPA; metrics Service name `ds-pipeline-workflow-controller-metrics-<dspa>` | Modular Kind CI. |
| Module deletion and shared-resource cleanup | `TestAIPipelinesLifecycle/finalizes_module_resources` (cleanup finalizer, shared assets, CRDs retained) | Modular Kind CI. |
| DSPA finalizer / owned resource GC | Same finalization subtest after module recreate | Modular Kind CI. |
| PlatformObject, singleton, status construction, RBAC | `make unittest`, `make functest`, and `api/aipipelines/v1alpha1` envtest tests | Not duplicated in Kind lifecycle suite. |
| Routes and Ingresses | Integration suite with `--endpoint-type route` on OpenShift | Separate OpenShift run. |
| Product upgrade | `opendatahub-io/data-science-pipelines` `UpgradePreparation` / `UpgradeVerification` | Out of scope for DSPO Kind tests. |

## Run modular Kind coverage

```bash
export GIT_WORKSPACE="$PWD"
# Supply REGISTRY_ADDRESS and the existing Kind prerequisites.
bash .github/scripts/tests/tests.sh --kind --modular
```

This runs the legacy integration suites with the module controller enabled, then
`make aipipelines-e2e-test` against the same cluster and the `test-dspa` DSPA.

For an already prepared dedicated cluster:

```bash
export KUBECONFIG=/path/to/dedicated-test-kubeconfig
export APPLICATIONS_NAMESPACE=opendatahub
export DSPANAMESPACE=test-dspa
export AIPIPELINES_DSPA_NAME=test-dspa
make aipipelines-e2e-test
```

The fixture lives in `.github/resources/aipipelines`. Setup enables
`DSPO_ENABLEAIPIPELINESMODULECONTROLLER=true` and deliberately sets the legacy
Argo configuration to `Removed` while the AIPipelines spec requests `Managed`.

## Run modular OpenShift Route coverage

```bash
bash .github/scripts/tests/tests.sh \
  --openshift-ci \
  --modular \
  --endpoint-type route
```

## Product upgrade coverage

Simulated operator rollbacks in Kind are not used for modular upgrade sign-off.
See [opendatahub-io/data-science-pipelines](https://github.com/opendatahub-io/data-science-pipelines)
for Jenkins upgrade suites. Document RHOAI downgrade as an accepted exclusion in
release notes when applicable.

## Sign-off checklist

- Link and record review of the approved E2E Testing Responsibilities document.
- Attach passing modular Kind runs (integration + `aipipelines-e2e-test`).
- Link KFP/data-science-pipelines evidence for post-upgrade modular verification
  when required for the release.
- Attach a passing modular OpenShift Route run when applicable.
- Record failures or accepted exclusions.
