# AIPipelines module integration and E2E tests

DSPO owns AIPipelines reconciliation and operand lifecycle testing. The platform
operator separately owns module bootstrap, AIPipelines CR lifecycle and spec
projection, status aggregation, DAG ordering, and platform-service resilience.
These tests drive the projected AIPipelines CR and platform-version handshake
directly; they do not claim to test platform orchestration.

The acceptance boundary is RHOAIENG-31299 and the E2E Testing Responsibilities
document. Add links to the approved versions of both documents before team
sign-off. A passing test run is evidence for the module-owned rows below, while
resolving the tracking task remains the team's acknowledgement of the ownership
change.

## Traceability

| Required responsibility | Automated coverage | Execution and evidence status |
| --- | --- | --- |
| Deployment health | `TestAIPipelinesLifecycle/reconciles_platform_configuration_and_operand_health` waits for every DSPA Deployment to observe its generation and reach Available. It deletes the API-server Deployment, requires `APIServerReady=False`, then verifies DSPO recreates it and status returns to Ready. | Runs in the modular Kind workflow. A live passing run is still required for sign-off. |
| StatefulSet and DaemonSet health | AIPipelines currently creates no StatefulSets or DaemonSets. Cleanup inventory includes both kinds so newly introduced owned workloads cannot be silently left behind. | Not applicable to the current manifests. Revisit when an operand changes workload kind. |
| Service reachability | An Argo Workflow connects through every DSPA Service intended for workflow access: both API aliases, both MinIO aliases, workflow-controller metrics, MLMD Envoy, and both MLMD gRPC aliases. API experiment creation exercises the API-to-MariaDB path, which is intentionally restricted by NetworkPolicy. | Runs in the modular Kind workflow. |
| Routes and Ingresses | The module creates no Ingress. The existing integration suite exercises an API Route when invoked with `--endpoint-type route`; Kind cannot validate OpenShift routing. | A passing modular OpenShift run is required separately; it is not proven by Kind. |
| Webhook behavior | `tests/webhook_test.go` creates and updates valid PipelineVersion resources and requires admission to reject an immutable spec update. | Runs through the existing Kubernetes pipeline-store suite in modular CI. |
| Module CR reconciliation | The lifecycle suite changes `argoWorkflowsControllers.managementState`, verifies shared and DSPA Argo resources are removed/recreated, changes a DSPA field, and verifies the rendered operand ConfigMap changes. | Runs in modular Kind CI. |
| Status and observedGeneration | Module helpers require current `observedGeneration` on status and conditions. Tests cover ready, invalid platform configuration, blocked Argo removal, and recovery. DSPA tests cover an unavailable API-server Deployment and recovery. | Unit/envtest tests cover deterministic status construction; modular Kind covers live controllers. |
| PlatformObject, singleton and projected fields | `api/aipipelines/v1alpha1` uses the platform validation helper and envtest CRD validation. The live suite rejects a second singleton and proves the projected Argo management field takes precedence over the conflicting legacy operator setting. | Unit, functional, and modular Kind jobs. |
| Cleanup and finalizers | Lifecycle tests hold an owned Argo ConfigMap during actual AIPipelines deletion, require the module cleanup finalizer to remain, release it, and verify shared assets disappear while Workflow data remains. DSPA deletion verifies its non-owned ClusterRoleBinding is finalized and all captured owned resources are garbage-collected. | Runs in modular Kind CI. |
| Upgrade and downgrade | The upgrade test rolls the manager image and caller-supplied changed `RELATED_IMAGE_*` values baseline → candidate → baseline. It requires a DSPA Deployment image to change and return, checks `.status.releases[name=platform]`, and preserves experiment data, S3 bytes, Workflow UID, PVC UIDs, and credentials. | Explicit modular workflow dispatch only. A selected baseline and a passing run are required for sign-off. |
| User flows | The existing integration suite runs pipeline upload/execution, artifacts, external storage/database, Kubernetes pipeline storage, and MLflow with modular mode enabled. | Runs before the module lifecycle test in the modular workflow. |

## Run functional coverage

```bash
make aipipelines-functional-test
```

This starts envtest API-server and etcd processes. It validates the real CRD,
controller watches, cache constraints, status transitions, shared-resource
adoption and shipped RBAC. Envtest does not run pods, networking, deployment
controllers, or Kubernetes garbage collection.

## Run modular Kind coverage

Use a dedicated cluster. The suite changes the cluster-scoped singleton and the
platform handshake, and refuses fixtures without the
`testing.opendatahub.io/aipipelines=true` label.

```bash
export GIT_WORKSPACE="$PWD"
# Supply REGISTRY_ADDRESS and the existing Kind prerequisites.
bash .github/scripts/tests/tests.sh --kind --modular
```

For an already prepared dedicated cluster:

```bash
export KUBECONFIG=/path/to/dedicated-test-kubeconfig
export APPLICATIONS_NAMESPACE=opendatahub
make aipipelines-e2e-test
```

The fixture lives in `.github/resources/aipipelines`. Setup enables
`DSPO_ENABLEAIPIPELINESMODULECONTROLLER=true` and deliberately sets the legacy
Argo configuration to `Removed` while the AIPipelines spec requests `Managed`.
Tests are sequential and must not share the cluster with a platform controller
that reconciles the same singleton.

## Run modular OpenShift Route coverage

```bash
bash .github/scripts/tests/tests.sh \
  --openshift-ci \
  --modular \
  --endpoint-type route
```

Record the cluster/version and passing job URL as sign-off evidence. This is a
separate gate because a Kind cluster has no OpenShift router.

## Run upgrade and downgrade coverage

Use an earlier modular manager image and provide the baseline values of all
changed `RELATED_IMAGE_*` variables. At least one supplied value must resolve to
a DSPA Deployment image so the test proves an operand rollout rather than only
an operator restart.

```bash
export KUBECONFIG=/path/to/dedicated-test-kubeconfig
export APPLICATIONS_NAMESPACE=opendatahub
export AIPIPELINES_BASELINE_IMAGE=registry.example.org/dspo:baseline
export AIPIPELINES_BASELINE_VERSION=3.6.0
export AIPIPELINES_CANDIDATE_VERSION=3.7.0
export AIPIPELINES_BASELINE_RELATED_IMAGES='{
  "RELATED_IMAGE_ODH_ML_PIPELINES_API_SERVER_V2_IMAGE":"registry.example.org/api-server:baseline"
}'
make aipipelines-upgrade-test
```

The candidate values come from the installed candidate DSPO Deployment. The
test uses the candidate checkout's CRDs and RBAC, so it covers module controller
and operand compatibility, data preservation, and platform-version transitions;
it does not replace an OLM bundle/CRD-conversion upgrade test owned by the
packaging or platform layer. The modular workflow exposes the same four values
as dispatch inputs and rejects partial input sets before creating a cluster.

## Sign-off checklist

- Link and record review of RHOAIENG-31299 and the approved E2E Testing
  Responsibilities document.
- Attach passing functional and modular Kind runs.
- Attach a passing upgrade/downgrade run with exact baseline/candidate manager
  and related-image references.
- Attach a passing modular OpenShift Route run.
- Record failures or accepted exclusions. If completion is later than RHOAI
  3.6 EA1, request additional time in the tracking issue.

Do not resolve the tracking task based only on compilation or newly added tests;
resolution represents team acknowledgement plus the live evidence above.
