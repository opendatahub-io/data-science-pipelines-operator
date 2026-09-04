/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AIPipelinesComponentName is the platform module name.
	AIPipelinesComponentName = "aipipelines"
	// AIPipelinesInstanceName is the only valid AIPipelines resource name.
	AIPipelinesInstanceName = "default-" + AIPipelinesComponentName
	// AIPipelinesKind is the module API kind.
	AIPipelinesKind = "AIPipelines"
)

// Verify that AIPipelines implements the platform module status contract.
var _ common.PlatformObject = (*AIPipelines)(nil)

// ArgoWorkflowsControllersSpec configures the lifecycle of the bundled Argo Workflows controllers.
type ArgoWorkflowsControllersSpec struct {
	// ManagementState controls whether the bundled Argo Workflows controllers are managed or removed.
	// +kubebuilder:validation:Enum=Managed;Removed
	// +kubebuilder:default=Managed
	ManagementState common.ManagementState `json:"managementState,omitempty"`
}

// AIPipelinesSpec defines the desired state of the AIPipelines module.
type AIPipelinesSpec struct {
	// ArgoWorkflowsControllers configures the lifecycle of the bundled Argo Workflows controllers.
	// When omitted, the controller uses Managed.
	// +optional
	ArgoWorkflowsControllers *ArgoWorkflowsControllersSpec `json:"argoWorkflowsControllers,omitempty"`
}

// ArgoWorkflowsControllersManagementState returns the effective Argo Workflows controller management state.
func (s *AIPipelinesSpec) ArgoWorkflowsControllersManagementState() common.ManagementState {
	if s.ArgoWorkflowsControllers == nil || s.ArgoWorkflowsControllers.ManagementState == "" {
		return common.Managed
	}

	return s.ArgoWorkflowsControllers.ManagementState
}

// AIPipelinesStatus defines the observed state of the AIPipelines module.
type AIPipelinesStatus struct {
	common.Status                 `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-aipipelines'",message="AIPipelines name must be default-aipipelines"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"

// AIPipelines is the cluster-scoped module API managed by the Data Science Pipelines Operator.
type AIPipelines struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   AIPipelinesSpec   `json:"spec"`
	Status AIPipelinesStatus `json:"status,omitempty"`
}

// GetStatus returns the common module status.
func (a *AIPipelines) GetStatus() *common.Status {
	return &a.Status.Status
}

// GetConditions returns the module conditions.
func (a *AIPipelines) GetConditions() []common.Condition {
	return a.Status.Conditions
}

// SetConditions replaces the module conditions.
func (a *AIPipelines) SetConditions(conditions []common.Condition) {
	a.Status.SetConditions(conditions)
}

// GetReleaseStatus returns the module release status.
func (a *AIPipelines) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &a.Status.ComponentReleaseStatus
}

// SetReleaseStatus replaces the module release status.
func (a *AIPipelines) SetReleaseStatus(status common.ComponentReleaseStatus) {
	a.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// AIPipelinesList contains a list of AIPipelines resources.
type AIPipelinesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIPipelines `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIPipelines{}, &AIPipelinesList{})
}
