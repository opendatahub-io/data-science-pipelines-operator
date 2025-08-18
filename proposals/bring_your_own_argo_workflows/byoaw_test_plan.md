# Test Plan: Bring Your Own Argo WorkFlows (BYOAWF)

## Table of Contents
1. [Test Scope](#test-scope)
2. [Test Environment Requirements](#test-environment-requirements)
3. [Test Categories](#test-categories)
4. [Test Execution Phases](#test-implementationexecution-phases)

## Test Scope

### Out of Scope
- Partial ArgoWF installs combined with DSP-shipped Workflow Controller
- Isolation between DSP ArgoWF WC and vanilla cluster-scoped ArgoWF installation

## Test Environment Requirements

### Prerequisites
- OpenShift/Kubernetes clusters with ODH/DSP installed
- Multiple test environments with different Argo Workflows versions
- Access to modify DataScienceCluster and DSPA configurations
- Sample pipelines covering various complexity levels
- Test data for migration scenarios

### Test Environments
| Environment | Argo Version   | DSP Version | Purpose                       |
|-------------|----------------|-------------|-------------------------------|
| Env-1       | Current(3.7.x) | Current     | N version compatibility       |
| Env-2       | 3.6.x          | Current     | N-1 version compatibility     |
| Env-3       | 3.4.x - 3.5.y  | Previous    | Upgrade scenarios             |

## Test Categories

## 1. Cluster Configuration Tests
This section covers tests for different cluster configurations to ensure BYOAWF functionality across various deployment scenarios.

### 1.1 Global Configuration Toggle

| Test Case ID          | TC-CC-001                                                                                                                                                                                                                                                                                                                                                                                                                                           |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify global toggle to disable WorkflowControllers works correctly                                                                                                                                                                                                                                                                                                                                                                                 |
| **Test Steps**        | <ol><li>Install ODH with default configuration (WorkflowControllers enabled)</li><li> Create DSPA and verify WorkflowController deployment</li><li> Update DataScienceCluster to disable WorkflowControllers:<br/>`spec.components.datasciencepipelines.argoWorkflowsControllers.managementState: Removed`</li><li> Verify existing WorkflowControllers are removed</li><li> Create new DSPA and verify no WorkflowController is deployed</li></ol> |
| **Expected Results**  | <ul><li> Global toggle successfully disables WorkflowController deployment</li><li> Existing WorkflowControllers are cleanly removed</li><li> New DSPAs respect global configuration</li><li> No data loss during WorkflowController removal </li></ul>                                                                                                                                                                                             |

| Test Case ID          | TC-CC-002                                                                                                                                                                                                                                                                                        |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify re-enabling WorkflowControllers after global disable                                                                                                                                                                                                                                      |
| **Test Steps**        | <ol><li>Start with globally disabled WorkflowControllers</li><li>Create DSPA without WorkflowController</li><li>Re-enable WorkflowControllers globally</li><li>Verify WorkflowController is deployed to existing DSPA</li><li>Create new DSPA and verify WorkflowController deployment</li></ol> |
| **Expected Results**  | <ul><li> Global re-enable successfully restores WorkflowController deployment</li><li> Existing DSPAs receive WorkflowControllers</li><li> New DSPAs deploy with WorkflowControllers</li><li> Pipeline history and data preserved </li></ul>                                                     |

### 1.2 Kubernetes Native Mode

| Test Case ID          | TC-CC-003                                                                                                                                                                                                                            |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify BYOAWF compatibility with Kubernetes Native Mode - Create Pipeline Via CR                                                                                                                                                      |
| **Test Steps**        | <ol><li>Configure cluster for Kubernetes Native Mode</li><li>Install external Argo Workflows</li><li>Disable DSP WorkflowControllers globally</li><li>Create DSPA</li><li>Create Pipeline via CR and create a pipeline run</li></ol> |
| **Expected Results**  | <ul><li> Kubernetes Native Mode works with external Argo</li><li> Pipeline execution uses Kubernetes-native constructs</li><li> No conflicts between modes </li></ul>                                                                |

| Test Case ID          | TC-CC-006                                                                                                                                                                                                                                |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify BYOAWF compatibility with Kubernetes Native Mode - Create Pipeline via API                                                                                                                                                         |
| **Test Steps**        | <ol><li>Configure cluster for Kubernetes Native Mode</li><li>Install external Argo Workflows</li><li>Disable DSP WorkflowControllers globally</li><li>Create DSPA</li><li>Create Pipeline via API/UI and create a pipeline run</li></ol> |
| **Expected Results**  | <ul><li> Kubernetes Native Mode works with external Argo</li><li> Pipeline executes successfully</li></ul>                                                                                                                               |

### 1.3 FIPS Mode Compatibility

| Test Case ID          | TC-CC-004                                                                                                                                                                                                         |
|-----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify BYOAWF works in FIPS-enabled clusters                                                                                                                                                                       |
| **Test Steps**        | <ol><li>Configure FIPS-enabled cluster</li><li>Install FIPS-compatible external Argo</li><li>Configure DSPA with external Argo</li><li>Execute pipeline suite</li><li>Verify FIPS compliance maintained</li></ol> |
| **Expected Results**  | <ul><li> External Argo respects FIPS requirements</li><li> Pipeline execution maintains FIPS compliance</li><li> No cryptographic violations </li></ul>                                                           |

### 1.4 Disconnected Cluster Support

| Test Case ID          | TC-CC-005                                                                                                                                                                                                                                |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify BYOAWF functionality in disconnected environments                                                                                                                                                                                  |
| **Test Steps**        | <ol><li>Configure disconnected cluster environment</li><li>Install external Argo from local registry</li><li>Configure DSPA for external Argo</li><li>Execute pipelines using local artifacts</li><li>Verify offline operation</li></ol> |
| **Expected Results**  | <ul><li> External Argo operates in disconnected mode</li><li> Pipeline execution works without external connectivity</li><li> Local registries and artifacts accessible </li></ul>                                                       |

## 2. Positive Functional Tests
This section covers all positive functional tests to make sure that feature works as expected and there is no regression as well

### 2.1 Basic Pipeline Execution

| Test Case ID          | TC-PF-001                                                                                                                                                                                                      |
|-----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify basic pipeline execution with external Argo                                                                                                                                                             |
| **Test Steps**        | <ol><li>Configure DSPA with external Argo</li><li>Submit simple addition pipeline</li><li> Monitor execution through DSP UI</li><li> Verify completion and results</li><li> Check logs and artifacts</li></ol> |
| **Expected Results**  | <ul><li> Pipeline submits successfully</li><li> Execution progresses normally</li><li> Results accessible through DSP interface</li><li> Logs and monitoring functional </li></ul>                             |

### 2.3 Pod Spec Override Testing
Tests to validate that if you override Pod Spec, then correct kubernetes properties gets applied when the pods are created

| Test Case ID          | TC-PF-018                                                                                                                                                                                    |
|-----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify pipeline execution with Pod spec overrides containing "Node taints and tolerations"                                                                                                   |
| **Test Steps**        | <ol><li> Configure pipelines with Pod spec patch: Node taints and tolerations</li><li>Execute pipelines with external Argo  </li></ol>                                                       |
| **Expected Results**  | <ul><li> Pod spec overrides applied successfully</li><li> Pipelines schedule on correct nodes</li><li> PVCs mounted and accessible</li><li> Custom labels and annotations present </li></ul> |

### 2.4 Multi-DSPA Environment

| Test Case ID          | TC-PF-022                                                                                                                                                                                                                 |
|-----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify multiple DSPAs sharing external Argo                                                                                                                                                                               |
| **Test Steps**        | <ol><li> Create DSPAs in different namespaces</li><li>Configure all for external Argo</li><li>Execute pipelines simultaneously</li><li>Verify namespace isolation</li><li>Check resource sharing and conflicts </li></ol> |
| **Expected Results**  | <ul><li> Multiple DSPAs operate independently</li><li> Proper namespace isolation maintained</li><li> No pipeline interference or data leakage</li><li> Resource sharing works correctly </li></ul>                       |

## 3. RBAC and Security Tests
Make sure that RBACs are handled properly and users cannot misuse clusters due to a security hole

### 3.1 Namespace-Level RBAC

| Test Case ID          | TC-RBAC-001                                                                                                                                                                                                                                                     |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify RBAC with DSP cluster-level and Argo namespace-level access                                                                                                                                                                                              |
| **Test Steps**        | <ol><li> Configure DSP with cluster-level permissions</li><li>Configure Argo with namespace-level restrictions</li><li>Create users with different permission levels</li><li>Test pipeline access and execution</li><li>Verify permission boundaries </li></ol> |
| **Expected Results**  | <ul><li> RBAC properly enforced at both levels</li><li> Users limited to appropriate namespaces</li><li> No unauthorized access to pipelines</li><li> Permission escalation prevented </li></ul>                                                                |

### 3.2 Service Account Integration

| Test Case ID          | TC-RBAC-002                                                                                                                                                                                                                              |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify service account integration with external Argo                                                                                                                                                                                    |
| **Test Steps**        | <ol><li> Configure custom service accounts</li><li>Set specific RBAC permissions</li><li>Execute pipelines with different service accounts</li><li>Verify permission enforcement</li><li>Test cross-namespace access controls </li></ol> |
| **Expected Results**  | <ul><li> Service accounts properly integrated</li><li> Permissions correctly enforced</li><li> No unauthorized resource access</li><li> Proper audit trail maintained </li></ul>                                                         |

### 3.3 Workflow Visibility and Project Access Control

| Test Case ID          | TC-RBAC-003                                                                                                                                                                                                                                                                                                                                                                                                                                                |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify workflows using external Argo are only visible to users with Project access                                                                                                                                                                                                                                                                                                                                                                         |
| **Test Steps**        | <ol><li> Create multiple Data Science Projects with different users</li><li>Configure external Argo for all projects</li><li>Execute pipelines from different projects</li><li>Test workflow visibility across projects with different users</li><li>Verify users can only see workflows from their accessible projects</li><li>Test API access controls and UI filtering</li><li>Verify external Argo workflows respect DSP project boundaries </li></ol> |
| **Expected Results**  | <ul><li> Workflows only visible to users with project access</li><li> Proper isolation between Data Science Projects</li><li> API and UI enforce access controls correctly</li><li> External Argo workflows respect DSP boundaries</li><li> No cross-project workflow visibility </li></ul>                                                                                                                                                                |

## 4. Compatibility Matrix Tests
Based on the compatability matrix as defined in #Test Environments

### 4.1 Current Version (N) Compatibility

| Test Case ID          | TC-CM-001                                                                                                                                                                                                                                       |
|-----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Validate compatibility with current supported Argo version                                                                                                                                                                                      |
| **Test Steps**        | <ol><li> Install current supported Argo version (e.g., 3.4.16)</li><li>Configure DSPA for external Argo</li><li>Execute comprehensive pipeline test suite</li><li>Verify all features work correctly</li><li>Document any limitations</li></ol> |
| **Expected Results**  | <ul><li> Full compatibility with current version</li><li> All pipeline features operational</li><li> No breaking changes or issues</li><li> Performance within acceptable range </li></ul>                                                      |

### 4.2 Previous Version (N-1) Compatibility

| Test Case ID          | TC-CM-002                                                                                                                                                                                                                                                    |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Validate compatibility with previous supported Argo version                                                                                                                                                                                                  |
| **Test Steps**        | <ol><li> Install previous supported Argo version (e.g., 3.4.15)</li><li>Configure DSPA for external Argo</li><li>Execute comprehensive pipeline test suite</li><li>Document compatibility differences</li><li>Verify core functionality maintained</li></ol> |
| **Expected Results**  | <ul><li> Core functionality works with N-1 version</li><li> Any limitations clearly documented</li><li> No critical failures or data loss</li><li> Upgrade path available </li></ul>                                                                         |

### 4.3 DSP and External Argo Co-existence Validation

| Test Case ID          | TC-CM-004                                                                                                                                                                                                                                                                                                                                                                                                           |
|-----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Validate successful hello world pipeline with DSP and External Argo co-existing                                                                                                                                                                                                                                                                                                                                     |
| **Test Steps**        | <ol><li> Deploy DSPA with internal WorkflowController</li><li>Install external Argo WorkflowController on same cluster</li><li>Submit simple hello world pipeline through DSP</li><li>Verify pipeline executes successfully using DSP controller</li><li>Verify external Argo remains unaffected</li><li>Test pipeline monitoring and status reporting</li><li>Validate artifact handling and logs access</li></ol> |
| **Expected Results**  | <ul><li> Hello world pipeline executes successfully</li><li> DSP WorkflowController processes the pipeline</li><li> External Argo WorkflowController unaffected</li><li> No resource conflicts or interference</li><li> Pipeline status and logs accessible</li><li> Artifacts properly stored and retrievable </li></ul>                                                                                           |

## 5. Uninstall and Data Preservation Tests
Verify that if you uninstall DSPA or Argo Workflow Controller, then the data is still preserved, so that the next time deployment happens, things continue - this includes use case for different deployment strategies

### 5.1 DSPA Uninstall with External Argo

| Test Case ID          | TC-UP-001                                                                                                                                                                                                                                                                                                                         |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify DSPA uninstall behavior with external Argo                                                                                                                                                                                                                                                                                 |
| **Test Steps**        | <ol><li> Configure DSPA with external Argo (no internal WC)</li><li>Execute multiple pipelines and generate data</li><li>Delete DSPA</li><li>Verify external Argo WorkflowController remains intact</li><li>Verify DSPA-specific resources are cleaned up</li><li>Check that pipeline history is appropriately handled </li></ol> |
| **Expected Results**  | <ul><li> DSPA removes cleanly</li><li> External Argo WorkflowController unaffected</li><li> No impact on other DSPAs using same external Argo</li><li> Pipeline data handling follows standard procedures </li></ul>                                                                                                              |

### 5.2 DSPA Uninstall with Internal WorkflowController

| Test Case ID          | TC-UP-002                                                                                                                                                                                                                                                                             |
|-----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify standard DSPA uninstall with internal WorkflowController                                                                                                                                                                                                                       |
| **Test Steps**        | <ol><li> Configure DSPA with internal WorkflowController</li><li>Execute pipelines and generate data</li><li>Delete DSPA</li><li>Verify WorkflowController is removed with DSPA</li><li>Verify proper cleanup of all DSPA components</li><li>Ensure no external Argo impact</li></ol> |
| **Expected Results**  | <ul><li> DSPA and WorkflowController removed completely</li><li> Standard cleanup procedures followed</li><li> No resource leaks or orphaned components</li><li> External Argo installations unaffected </li></ul>                                                                    |

### 5.3 Data Preservation During WorkflowController Transitions

| Test Case ID          | TC-UP-003                                                                                                                                                                                                                                                                                                                                                          |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify data preservation during WorkflowController management transitions                                                                                                                                                                                                                                                                                          |
| **Test Steps**        | <ol><li> Create DSPA with internal WC and execute pipelines</li><li>Disable WC globally (transition to external Argo)</li><li>Verify run history, artifacts, and metadata preserved</li><li>Re-enable WC globally (transition back to internal)</li><li>Verify all historical data remains accessible</li><li>Test new pipeline execution in both states</li></ol> |
| **Expected Results**  | <ul><li> Pipeline run history preserved across transitions</li><li> Artifacts remain accessible</li><li> Metadata integrity maintained</li><li> New pipelines work in both configurations </li></ul>                                                                                                                                                               |

### 5.4 WorkflowTemplates and CronWorkflows Preservation

| Test Case ID          | TC-UP-004                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify preservation of WorkflowTemplates and CronWorkflows during DSP install/uninstall                                                                                                                                                                                                                                                                                                                                                                   |
| **Test Steps**        | <ol><li> Install external Argo and create WorkflowTemplates and CronWorkflows</li><li>Install DSP with BYOAWF configuration</li><li>Verify existing WorkflowTemplates and CronWorkflows remain intact</li><li>Create additional WorkflowTemplates through DSP interface</li><li>Uninstall DSP components</li><li>Verify all WorkflowTemplates and CronWorkflows still exist</li><li>Test functionality of preserved resources with external Argo</li></ol> |
| **Expected Results**  | <ul><li> Pre-existing WorkflowTemplates and CronWorkflows preserved</li><li> DSP-created templates also preserved during uninstall</li><li> All preserved resources remain functional</li><li> No data corruption or resource deletion</li><li> External Argo can use all preserved templates </li></ul>                                                                                                                                                  |

## 6. Migration and Upgrade Tests
Covers migration from internal to external WC and vice versa. Also covers upgrade of ODH and Argo versions

### 6.1 DSP-Managed to External Migration

| Test Case ID          | TC-MU-001                                                                                                                                                                                                                            |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify migration from DSP-managed to external Argo                                                                                                                                                                                   |
| **Test Steps**        | <ol><li> Create DSPA with internal WorkflowController</li><li>Execute pipelines and accumulate data</li><li>Install external Argo</li><li>Disable internal WCs globally</li><li>Verify data preservation and new execution</li></ol> |
| **Expected Results**  | <ul><li> Migration completes without data loss</li><li> Historical data remains accessible</li><li> New pipelines use external Argo</li><li> Artifacts and metadata preserved </li></ul>                                             |

### 6.2 External to DSP-Managed Migration

| Test Case ID          | TC-MU-002                                                                                                                                                                                                            |
|-----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify migration from external to DSP-managed Argo                                                                                                                                                                   |
| **Test Steps**        | <ol><li> Configure DSPA with external Argo</li><li>Execute pipelines and verify data</li><li>Re-enable internal WCs globally</li><li>Remove external Argo configuration</li><li>Verify continued operation</li></ol> |
| **Expected Results**  | <ul><li> Migration to internal WC successful</li><li> Pipeline history preserved</li><li> New pipelines use internal WC</li><li> No service interruption  </li></ul>                                                 |

### 6.3 ODH Upgrade Scenarios

| Test Case ID          | TC-MU-003                                                                                                                                                                                                            |
|-----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify ODH upgrade preserves external Argo setup                                                                                                                                                                     |
| **Test Steps**        | <ol><li> Configure ODH with external Argo</li><li>Execute baseline pipeline tests</li><li>Upgrade ODH to newer version</li><li>Verify external Argo configuration intact</li><li>Re-execute pipeline tests</li></ol> |
| **Expected Results**  | <ul><li> Upgrade preserves BYOAWF configuration</li><li> External Argo continues working</li><li> No functionality regression</li><li> Configuration settings maintained </li></ul>                                   |

### 6.4 Argo Version Upgrade with External Installation

| Test Case ID          | TC-MU-004                                                                                                                                                                                                                                                                                |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify external Argo version upgrade scenarios                                                                                                                                                                                                                                           |
| **Test Steps**        | <ol><li> Configure DSPA with external Argo version N-1</li><li>Execute baseline pipeline tests</li><li>Upgrade external Argo to version N</li><li>Verify compatibility matrix adherence</li><li>Test pipeline execution post-upgrade</li><li>Document any required ODH updates</li></ol> |
| **Expected Results**  | <ul><li> External Argo upgrade completes successfully</li><li> Compatibility maintained within support matrix</li><li> Clear guidance for required ODH updates</li><li> Pipeline functionality preserved  </li></ul>                                                                     |

### 6.5 Independent Lifecycle Management

| Test Case ID          | TC-MU-005                                                                                                                                                                                                                                                                                                                                                                                                          |
|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify independent lifecycle management of ODH and external Argo                                                                                                                                                                                                                                                                                                                                                   |
| **Test Steps**        | <ol><li> Install and configure ODH with external Argo</li><li>Perform independent upgrade of external Argo installation</li><li>Verify ODH continues operating without issues</li><li>Perform independent upgrade of ODH</li><li>Verify external Argo continues operating without issues</li><li>Test independent scaling of each component</li><li>Verify independent maintenance and restart scenarios</li></ol> |
| **Expected Results**  | <ul><li> Independent upgrades work without mutual interference</li><li> Each component maintains functionality during the other's maintenance</li><li> Scaling operations work independently</li><li> No forced coupling of upgrade/maintenance schedules</li><li> Clear documentation of independence boundaries  </li></ul>                                                                                      |

## 7 Miscellaneous Tests
Anything that we did cover in the above sections and do not fall under a certain category as well

### 7.1 Platform-Level CRD and RBAC Management

| Test Case ID          | TC-MT-001                                                                                                                                                                                                                                                                                                                                                 |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify platform-level Argo CRDs and RBAC remain intact with external Argo                                                                                                                                                                                                                                                                                 |
| **Test Steps**        | <ol><li>Install DSPO which creates platform-level Argo CRDs and RBAC</li><li>Install external Argo with different CRD versions</li><li>Toggle global WorkflowController disable</li><li>Verify platform CRDs are not removed</li><li>Test that user modifications to CRDs are preserved</li><li>Verify RBAC conflicts are handled appropriately</li></ol> |
| **Expected Results**  | <ul><li> Platform-level CRDs remain intact</li><li> User CRD modifications preserved</li><li> RBAC conflicts resolved without breaking functionality</li><li> Platform operator doesn't overwrite user changes </li></ul>                                                                                                                                 |

### 7.2 Sub-Component Removal Testing

| Test Case ID          | TC-MT-002                                                                                                                                                                                                                                                                                                                                                           |
|-----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify sub-component removal functionality for WorkflowControllers                                                                                                                                                                                                                                                                                                  |
| **Test Steps**        | <ol><li>Deploy DSPA with WorkflowController enabled</li><li>Execute pipelines and accumulate run data</li><li>Disable WorkflowController globally</li><li>Verify WorkflowController is removed but data preserved</li><li>Verify backing data (run details, metrics) remains intact</li><li>Test re-enabling WorkflowController preserves historical data</li></ol> |
| **Expected Results**  | <ul><li> WorkflowController removed cleanly</li><li> Run details and metrics preserved</li><li> Historical pipeline data remains accessible</li><li> Re-enabling restores full functionality </li></ul>                                                                                                                                                             |

### 7.3 Pre-existing Argo Detection and Prevention

| Test Case ID          | TC-MT-003                                                                                                                                                                                                                                                                                                                                                                                             |
|-----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify detection and prevention of DSPA creation when pre-existing Argo exists                                                                                                                                                                                                                                                                                                                        |
| **Test Steps**        | <ol><li>Install external Argo Workflows on cluster</li><li>Install ODH DSP operator</li><li>Attempt to create DSPA with default configuration (WC enabled)</li><li>Verify detection mechanism identifies pre-existing Argo</li><li>Test prevention of DSPA creation or automatic WC disable</li><li>Verify appropriate warning/guidance messages</li><li>Test manual override if supported </li></ol> |
| **Expected Results**  | <ul><li> Pre-existing Argo installation detected</li><li> DSPA creation prevented or WC automatically disabled</li><li> Clear guidance provided to user</li><li> Manual override works when applicable</li><li> No conflicts or resource competition  </li></ul>                                                                                                                                      |

### 7.4 CRD Update-in-Place Testing

| Test Case ID          | TC-MT-004                                                                                                                                                                                                                                                                                                                                                                                                                   |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Test Case Summary** | Verify CRD update-in-place when differences exist between pre-existing and shipped CRDs                                                                                                                                                                                                                                                                                                                                     |
| **Test Steps**        | <ol><li>Install external Argo with specific CRD version</li><li>Create Workflows, WorkflowTemplates, and CronWorkflows</li><li>Install DSP with different compatible CRD version</li><li>Verify CRDs are updated in-place</li><li>Verify existing CRs (Workflows, WorkflowTemplates, CronWorkflows) remain intact</li><li>Test new CR creation with updated CRD schema</li><li>Verify no data loss or corruption </li></ol> |
| **Expected Results**  | <ul><li> CRDs updated in-place successfully</li><li> Existing Workflows, WorkflowTemplates, CronWorkflows preserved</li><li> New CRs work with updated schema</li><li> No data loss or corruption</li><li> Compatibility maintained </li></ul>                                                                                                                                                                              |

## 8 Regression Tests
This is to verify that the integration of this feature with other product components does not introduce any regression. So this should be the very last tests that we need to run after verifying that there is no regression if used with last RHOAI release of other product components

| Test Case ID          | TC-IL-001                                                      |
|-----------------------|----------------------------------------------------------------|
| **Test Case Summary** | Verify that Iris Pipeline Runs on a **standard** RHOAI cluster |
| **Test Steps**        | <ol><li> Run an IRIS pipeline</li></ol>                        |
| **Expected Results**  | Verify that the pipeline run succeeds                          |

| Test Case ID          | TC-IL-002                                                          |
|-----------------------|--------------------------------------------------------------------|
| **Test Case Summary** | Verify that Iris Pipeline Runs on a **FIPS Enabled** RHOAI cluster |
| **Test Steps**        | <ol><li> Run an IRIS pipeline</li></ol>                            |
| **Expected Results**  | Verify that the pipeline run succeeds                              |

| Test Case ID          | TC-IL-003                                                          |
|-----------------------|--------------------------------------------------------------------|
| **Test Case Summary** | Verify that Iris Pipeline Runs on a **Disconnected** RHOAI cluster |
| **Test Steps**        | <ol><li> Run an IRIS pipeline</li></ol>                            |
| **Expected Results**  | Verify that the pipeline run succeeds                              |

## Test Implementation/Execution Phases
### Phase 1
List Test Cases to be executed/implemented as part of this phase

### Phase 2
List Test Cases to be executed/implemented as part of this phase

### Phase 3
Full End to End tests for that specific RHOAI release (with the `latest` of all products) as covered in #initiative_level_tests section
