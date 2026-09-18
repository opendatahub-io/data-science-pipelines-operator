# AIPipelines module testing

DSPO owns the reconciliation and operand tests for AIPipelines. The platform
operator owns bootstrap, DSC lifecycle/projection, status aggregation, and DAG
ordering. This suite drives DSPO's AIPipelines CR and platform handshake directly;
it does not install or test the platform operator.

Scope reviewed: `bc283de12eb718f593150bcc99f3e3ee13248057` (inclusive) through
`odh/main` at `94e3f7ae53b6ec8541359fbdc180860caa1653cc`, plus this branch's
platform configuration, image resolution, status, and manifest changes through
`c822302e`. The responsibilities in the task description are the acceptance
boundary. The internal RHOAIENG-31299 issue and E2E Testing Responsibilities
document still need to be linked and checked before team sign-off.

The [platform testing guide](https://github.com/opendatahub-io/odh-platform-utilities/blob/main/docs/integration-testing.md)
was consulted for context. Its DSC-oriented harness is not required here: the
task permits equivalent module-owned tests, and this repo already has a Kind
fixture, controller-runtime clients, envtest, and pipeline user-flow tests.
The API tests already use `common/validation.ValidatePlatformObject` from the
pinned `odh-platform-utilities` dependency.

## Coverage and ownership

| Responsibility | Proof in DSPO |
| --- | --- |
| PlatformObject, schema, singleton, defaults | Existing API validation tests plus the envtest and live API rejection cases |
| Shared Argo installation, legacy adoption, scoped cache, shipped RBAC | `TestAIPipelinesControllerLifecycle` runs both module controllers with the generated and supplemental Argo roles, a stripped ConfigMap cache, and filtered namespaced assets |
| Module status, observedGeneration, release gating | Envtest and live tests assert current-generation conditions, invalid-handshake recovery, blocked removal, and preservation of the previous release; envtest also tests deployment availability loss/recovery |
| Operand health and resilience | Live lifecycle test waits for all eight DSPA Deployments to complete rollout, checks DSPA readiness, replaces an API-server pod, and runs workflows before/after reconfiguration |
| Services reachable | Workflow pods make HTTP requests through the API-server and Minio Services; API and upgrade assertions also exercise service-selected port forwarding |
| Webhook functional | The existing Kubernetes pipeline-storage suite now accepts valid creates/metadata edits and rejects an immutable PipelineVersion spec edit through admission |
| Spec projection | Live lifecycle changes `Managed -> Removed -> Managed`, checks all shared and namespaced Argo resources, and changes DSPA sample-pipeline configuration |
| Platform configuration and related images | Envtest tests handshake watches; routine live lifecycle and upgrade tests assert ConfigMap-only propagation to module releases and DSPA sample metadata. Existing `controllers/config` and params tests cover related-image resolution, standalone gating, explicit overrides, and forwarded managed-pipeline images |
| Cleanup | Envtest and live tests hold an asset finalizer to block removal. Live tests delete/recreate the module and delete the DSPA, checking ownership, finalization, garbage collection of captured operands, and absence of workload pods |
| Upgrade/downgrade | Separate live test rolls baseline DSPO image -> candidate -> baseline, verifies releases and new workflow execution, and reads back the same experiment, S3 object bytes, Workflow UID, PVC UIDs, and storage credentials |
| Pipeline user flows | The existing Kind integration suite runs in both standalone and modular modes: pipeline upload/run, artifacts, external database/storage, Kubernetes pipeline storage, and MLflow |

Current operands use Deployments; the module does not create StatefulSets,
DaemonSets, or Ingresses. OpenShift Routes remain covered by the existing
integration suite's `endpointType=route` entry point on a real OpenShift cluster.
Kind installs the Route CRD for compatibility but has no OpenShift router, so
Kind success is not evidence of Route reachability.

Cleanup follows the controller's data ownership rules: module removal deletes
shared controller assets, preserves Argo CRDs and workflow data, and leaves DSPAs
under their own lifecycle. `argoWorkflowsControllers.managementState=Removed`
also removes each DSPA's bundled Argo controller. Deleting a DSPA then removes its
owned operands. The test must not delete CRDs to make cleanup assertions pass.

## Run without a cluster

```bash
make aipipelines-functional-test
```

This starts isolated API-server/etcd processes and never reads your kubeconfig.
It simulates only the platform-owned DSPO Deployment's status: envtest does not
run pods or Kubernetes garbage collection. The regular `make functest` CI job
includes this test via the existing `test_functional` build tag.

## Run live tests

Use a dedicated cluster with DSPO built from the checkout, modular mode enabled,
and no platform operator reconciling the singleton. Both the singleton and its
handshake ConfigMap must carry `testing.opendatahub.io/aipipelines=true`.
The suite refuses unmarked fixtures. It creates unique test namespaces and
restores the module spec and handshake after each test. Tests are sequential;
do not run lifecycle and upgrade commands concurrently.

The existing setup script installs and configures the fixture, runs the existing
pipeline tests in modular mode, then runs the module lifecycle suite:

```bash
export GIT_WORKSPACE="$PWD"
# Supply REGISTRY_ADDRESS and the other existing Kind setup prerequisites.
bash .github/scripts/tests/tests.sh --kind --modular
```

For an already prepared dedicated cluster:

```bash
export KUBECONFIG=/path/to/dedicated-test-kubeconfig
export APPLICATIONS_NAMESPACE=opendatahub
make aipipelines-e2e-test
```

The fixture is in `.github/resources/aipipelines/module.yaml`. Setup enables
`DSPO_ENABLEAIPIPELINESMODULECONTROLLER=true` and deliberately configures the
legacy Argo environment setting to `Removed` while the module requests `Managed`.
This verifies that the module spec is authoritative. Shared Argo resources are
installed by DSPO, not preinstalled by the setup script.

The modular Kind workflow mirrors the existing standalone and BYO Argo workflow
layout without changing their jobs. Failures propagate to the modular job result
and export Kind diagnostics. Use `--openshift-ci --modular` with the existing
`--endpoint-type route` option for OpenShift endpoint validation; the
module-specific fixture itself uses internal Services.

## Upgrade and downgrade

Install the candidate image and use an earlier **modular** DSPO image as the
baseline. The test uses the candidate's installed CRDs/RBAC and configured
operand images throughout; it tests DSPO controller image compatibility and
platform-version transitions, not OLM upgrades, CRD conversion, or an entire
platform/operand release upgrade. An older standalone-only image is not a valid
baseline.

```bash
export KUBECONFIG=/path/to/dedicated-test-kubeconfig
export AIPIPELINES_BASELINE_IMAGE=registry.example.org/dspo:baseline
export AIPIPELINES_BASELINE_VERSION=3.6.0
export AIPIPELINES_CANDIDATE_VERSION=3.7.0
make aipipelines-upgrade-test
```

Inputs are mandatory for this target; missing inputs fail rather than skip.
For CI, dispatch `kind-integration-modular.yml` with `baseline-image`,
`baseline-version`, and `candidate-version`. It runs the same test after the
lifecycle suite. Routine PR jobs run the lifecycle suite; they do not claim to
verify an upgrade without a selected baseline. The original candidate pod
template is restored even on failure.

## Sign-off evidence

Before resolving the tracking task, attach passing functional and modular Kind
runs, an upgrade/downgrade run with the selected baseline, and an OpenShift Route
run. Record exact source/image versions and any failures. Review the linked
internal requirements and confirm that the ownership-preserving cleanup and
operator-only version-roll scope above match the release's acceptance criteria.
If completion will miss 3.6 EA1, request additional time in the tracking issue.
Adding these tests or compiling them is not team sign-off or a live E2E pass.
