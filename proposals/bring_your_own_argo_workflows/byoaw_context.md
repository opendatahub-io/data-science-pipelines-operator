## Feature Overview
Data Science Pipelines currently deploys a standalone Argo Workflow Controller and its respective resources, including CRDs and other cluster-scoped manifests.  This could potentially cause conflicts on clusters that already have a separate Argo Workflows deployed on the cluster, so the intent of this feature is to handle and reconcile these situations appropriately.
This feature will implement a global configuration option to disable WorkflowControllers from being deployed alongside DataSciencePipelineApplications, and instead use user-provided Argo Workflows instead.  Consequently, this feature will also include documentation of supported versions between these “Bring your own” Argo installations and current versions of ODH/Data Science Pipelines, and improving our testing strategy around this as we validate this compatibility.

## Why do we need this feature
Potential users, who have their own Argo Workflows installation already running on their clusters, have noted that the current architecture of Data Science Pipelines would conflict with their environment, as DSPAs currently provision their own Argo Workflow Controller.  This would create a competition condition between the user-provided and DSP-provisioned AWF instances, and therefore this prevents the user from adopting DSP.  Adding the ability to disable DSP-provided WorkflowControllers and instead use a “Bring-your-own” instance removes this block.

## Feature Requirements
### High level requirements
* As a Cluster Administrator I want to be able to install ODH DSP in a cluster that has an existing Argo Workflows installation.
* As a Cluster Administrator I want to be able to globally enable and disable deploying Argo WorkflowControllers in a Data Science Project with a Data Science Pipelines Application installed.
* As a Cluster Administrator I want to be able to add or remove all Argo WorkflowControllers from managed Data Science Pipelines Applications by updating a platform-level configuration.
* As a Cluster Administrator I want to be able to upgrade my ODH cluster and the DSP component in a cluster that has Argo Workflows installation.
* As a Cluster Administrator I want to manage the lifecycle of my ODH and Argo Workflow installation independently.
* As a Cluster Administrator, I want to easily understand what versions of Argo are compatible with what versions of DSP

### Non-functional requirements
* Pre-existing Argo CRDs and CRs should not be removed when installing DSP
  * Removing the CRDs on DSP install would constitute a destructive installation side effect which needs to be avoided (breaks existing workflows)
  * If a diff in pre-existing and shipped Argo CRDs exists, need to update-in-place, assuming compatibility is supported
  * Includes Workflows, WorkflowTemplates, CronWorkflows, etc
* Version of supported Argo Workflows, and latest version of n-1 previous minor release, would need to be tracked and tested for compatibility upon new minor releases
  * Example: ensure an ArgoWF v3.4.18 release is still compatible while DSP is using v3.4.17
* Maintain a compatibility matrix of ArgoWF backend to DSP releases
* Add configuration mechanism to globally enable/disable deploying managed Argo WCs in DSPAs
* Add mechanism to DSPO to remove a subcomponent (such as Argo WC), rather than just removing management responsibilities of it
* Provide a migration plan for when DSP needs to upgrade to new ArgoWF version while using external ArgoWF
* Ensure that workflow runs on DSP using an external ArgoWF are only visible to users with access to the containing Project
* Update, improve and document a testing strategy for coverage of supported versions of Argo workflows for a given ODH version
* Update, improve and document a testing strategy for coverage of latest version of previous minor release of Argo Workflows for a given ODH version
* Get upstream community to add support and document multiple versions of Argo Workflows dependencies
* Documentation about the support and versions supported.
* Update the ODH and DSP operators to prevent creation of DSPAs with DSP-managed Workflow Controllers in cases where a pre-existing Argo Workflows installation is detected (P1: depends on feasibility of this detection mechanism)

### Supported Version Compatibility
The Kubeflow Pipelines backend has codebase dependencies with Argo Workflows libraries, which in turn have interactions with the deployed Argo Workflows pipeline engine via k8s interfaces (CRs, etc).  In turn, the Data Science Pipelines Application can be deployed with components that have AWF dependencies independent of the deployed Argo Workflows backend.  The consequence of this is that it is possible for the API Server to be out-of-sync or not fully compatible with the deployed Workflow Controller, especially one that is deployed by a user outside of a Data Science Pipelines Application stack.  Therefore, a compatibility matrix will need to be created, documented, tested, and maintained.

Current messaging states that there is no written guarantee that future releases of Argo Workflows are compatible with previous versions, even Z-streams.  However, community maintainers have stated they are working with this in mind and with the intention of introducing a written mandate that z-stream releases will not introduce breaking changes.  Additionally, Argo documentation states patch versions will only contain bug fixes and minor features, which would include breaking changes.  This will help broaden our support matrix and testing strategy so we should work upstream to cultivate and introduce this as quickly as possible.

With that said, there is also no guarantee that Minor releases of Argo Workflows will not introduce breaking changes.  In fact, we have seen multiple occasions where this happens (3.3 to 3.4 upgrade, for instance, required a very non-trivial PR that blocked upstream dependency upgrades for over a year.  In contrast, the 3.4 to 3.5 upgrade was straightforward with no introduced breaking changes. This suggests that minor AWF upgrades will always carry inherent risk and therefore should not be included in the support matrix, at least without extensive testing.

Given these conditions, an example compatibility matrix would look like the following table:

| **ODH Version** | **Supported ArgoWF Version, Current State** | **Supported Range of ArgoWF Versions, upstream z-stream stability mandate accepted** |
|-------------------|---------------------------------------------|--------------------------------------------------------------------------------------|
| 3.4.1             | 3.4.16                                      | 3.4.16                                                                               |
| 3.5.0             | 3.5.14, 3.5.10 - 3.5.13, …                  | 3.5.x                                                                                |
| 3.6.0             | 3.5.14                                      | 3.5.x - 3.5.y                                                                        |

### Out of scope
* Isolating a DSP ArgoWF WC from a vanilla cluster-scoped ArgoWF installation
* Using partial ArgoWF installs in combination with DSP-shipped Workflow Controller

### Upgrades/Migration
In this feature, because the user is providing their own Workflow Controller, there will need to be documentation written on the Upgrade procedure such that self-provided AWF installations remain in-sync with the version supported by ODH during upgrades of the platform operator and/or DSPO.  This should be simple - typically an AWF upgrade just involves re-applying manifests from a set of folders.  Regardless, documentation should point to these upstream procedures to simplify the upgrade process.

A migration plan should also be drafted (for switching the backing pipeline engine between user-provided and dspo-managed).  That is - if a DSPA has a WC but the user wishes to remove it and leverage their own ArgoWF, how are runs, versions, etc persisted between the two Argo Workflows instances? As it stands now, because DSP stores metadata and artifacts in MLMD and S3, respectively, these should be hot-swappable and run history/artifact lineage should be maintained.  The documentation produced should mention these conditions.

Documentation should also mention that users with self-managed Argo Workflows will be responsible for upgrading their ODH installations appropriately to stay in-support with Argo Workflows. That is - if a user has brought their own AWF installation and it goes out-of-support/EOL, the user will be responsible with upgrading ODH to a version that has DSP built on an AWF backend that is still in-support. This can be done by cross-referencing the support matrix proposed above. ODH will not be responsible for rectifying conditions where an out-of-support Argo Workflows version is installed alongside a supported version of ODH, nor will ODH block on upgrading if this condition is encountered. Consequently, this also means that shipped/included ArgoWorkflowControllers of the latest ODH release will support an Argo Workflows version that is still maintained and supported by the upstream Argo community.

### Multiple Workflow Controller Conflicts
We will need to account for possible situations where a cluster-scoped Workflow Controller has been deployed on a cluster, and then a DSPA is created without disabling the namespace-scoped Workflow Controller in the DSPA spec.

Open Questions to answer via SPIKE:
Should we attempt to detect this condition?
Should this just be handled in documentation as an unsupported configuration?

Conversely, if a WorkflowController already exists in a deployed DSPA and a user then deploys their own cluster-scoped Argo Workflow Controller, do we handle this the same way?  Should the DSPOs detect an incompatible environment and attempt to reconcile by removing WCs?  What are the consequences of this?

These detection features would be “Nice to haves” ie P1, but not necessary for the MVP of the feature

### Uninstall
Uninstallation of the component should remain consistent with the current procedure - deleting a DSPA should delete the included Workflow Controller, but should have no bearing on an onboard/user-provided WC.  Users that have disabled WC deployments via the global toggle switch, the main mechanism for  BYO Argos, also will remain unaffected - removing a DSPA that does not have a WC because it has not been deployed will still be removed in the same standard removal procedure.

### ODH/DSPO Implementation
DSPO already supports deployment of a Data Science Pipelines Application stack without a Workflow Controller, so no non-trivial code changes should be necessary.  This can be done by specifying spec.workflowController.deploy as false in the DSPAs

```---
apiVersion: datasciencepipelinesapplications.opendatahub.io/v1
kind: DataSciencePipelinesApplication
metadata:
  name: dspa
  namespace: dspa
spec:
  dspVersion: v2
  workflowController:
    deploy: false
  ...
  ```

With that said, for ODH installations with a large number of DSPAs it would be unsustainable to require editing every DSPA individually.  A global toggle mechanism must be implemented instead - one which would remove the Workflow Controller from ALL managed DSPAs.  This would be set in the DataScienceCluster CR (see example below) and would involve coordination with the Platform dev team for implementation.  Given that, documentation in the DSPA CRD will need to be added to notify users that it is an unsupported configuration to have individual WCs disabled if a user is providing their own Argo WorkflowController, and that the field is for development purposes only.

Example DataScienceCluster with WorkflowControllers globally disabled:
```---
kind: DataScienceCluster
...
spec:
  components:
  datasciencepipelines:
    managementState: Managed
    argoWorkflowsControllers:
      managementState: Removed
```

Another consequence of this would be that the DSPO will need to have functionality to remove sub-components such as the WorkflowController (but not backing data, such as run details, metrics, etc)  from an already-deployed DSPA.  Currently, setting deploy to false simply removes the management responsibility of the DSPA for that Workflow Controller - it will still exist assuming it was deployed at some point (deploy set to true).  See “Uninstall” section below for more details.

Because the Argo RBAC and CRDs are installed on the platform level (i.e. when DSPO is created), these would be left in place even if the “global switch” is toggled to remove all DSPA-owned WCs.  The DSP team would need to update the deployment/management mechanism, as updates made to these by a user to support bringing their own AWF would be overwritten by the platform operator.


## Test Plan Requirements
* Do not generate any code
* Create a high level test plan with sections
* Test plan should include the maintaining and validating changes against the compatibility matrix. The intent here is to cover an “N” and “N-1” version of Argo Workflows for verification of compatibility.
* Each Section is group of tests by type of tests with summary describing what types of tests are being covered and why
* Test Sections:
  * Cluster config
  * Negative functional tests
  * Positive functional tests
  * Security Tests
  * Boundary tests
  * Performance tests
  * Compatibility matrix tests
  * Miscellaneous Tests
  * Final Regression/Full E2E Tests
* Test Cases for `Cluster config` section:
    * [Kubernetes Native Mode](https://github.com/kubeflow/pipelines/tree/master/proposals/11551-kubernetes-native-api)
    * FIPS Mode
    * Disconnected Cluster
* Test Cases for `Negative functional tests` section:
    * With conflicts Argo Workflow controller instances (DSP and External controllers coexisting and looking for the same type of events)
    * With DSP and external workflow controller on different RBAC
    * DSP with incompatible workflow schema
* Test Cases for `Positive functional tests` section:
    * With artifacts
    * Without artifacts
    * For Loop
    * Parallel for
    * Custom root kfp
    * Custom python package indexes
    * Custom base images
    * With input
    * Without input
    * With output
    * Without output
    * With iteration count
    * With retry
    * With cert handling
    * etc.
    * Override Pod Spec patch - create separate test cases for the following:
      * Node taint
      * PVC
      * Custom labels
* Test Cases for `Security Tests` section:
  * with different RBAC access with DSP at cluster level and Argo Workflow controller at Namespace level access
* Test Cases for `Miscellaneous Tests` section:
  * Validate a successful run of a simple hello world pipeline With DSP Argo Workflow Controller to coexist with External Argo Workflow controller
* Test Cases for `Final Regression/Full E2E Tests` section (Run this on a fully deployed RHOAI cluster with latest of all products for that specific release):
  * Run Iris Pipeline on a standard RHOAI Cluster with DB as storage
  * Run Iris Pipeline on a FIPS enabled RHOAI Cluster
  * Run Iris Pipeline on a disconnected RHOAI Cluster
  * Run Iris Pipeline on a standard RHOAI Cluster with K8s Native API Storage
* Test case should be in a Markdown table format and include following:
  - test case summary
  - test steps
    + Test steps should be a HTML format ordered list
  - Expected results
    + If there are multiple expectations, then it should be in a HTML format ordered list
* Iterate over 5 times before generating a final output
* Use this test plan documentation as an Example test plan document https://github.com/kubeflow/pipelines/blob/c1876c509aca1ffb68b467ac0213fa88088df7e1/proposals/11551-kubernetes-native-api/TestPlan.md
* Create a Markdown file as the output test plan

### Example Test Plan
https://github.com/kubeflow/pipelines/blob/c1876c509aca1ffb68b467ac0213fa88088df7e1/proposals/11551-kubernetes-native-api/TestPlan.md
