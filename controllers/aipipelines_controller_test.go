//go:build test_all || test_unit

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

package controllers

import (
	"context"
	"testing"
	"time"

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildAIPipelinesStatusReadyWithoutDSPAs(t *testing.T) {
	viper.Set("DSPO.PlatformVersion", "v3.6.0")
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	module.Generation = 4

	status := buildAIPipelinesStatus(module, nil)

	require.Equal(t, common.PhaseReady, status.Phase)
	require.EqualValues(t, 4, status.ObservedGeneration)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).Status)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeDSPOReady).Status)
	argo := requireModuleCondition(t, status.Conditions, conditionTypeArgoReady)
	require.Equal(t, metav1.ConditionTrue, argo.Status)
	require.Equal(t, "No DSPA instances are present", argo.Message)
	require.Equal(t, "v3.6.0", status.GetPlatformRelease())
}

func TestBuildAIPipelinesStatusCountsOnlyApplicableArgoInstances(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	ready := readyTestDSPA("team-a", "ready")
	ready.Status.Conditions = append(ready.Status.Conditions, metav1.Condition{
		Type:   config.WorkflowControllerReady,
		Status: metav1.ConditionTrue,
	})
	notApplicable := readyTestDSPA("team-b", "external-argo")
	notApplicable.Status.Conditions = append(notApplicable.Status.Conditions, metav1.Condition{
		Type:   config.WorkflowControllerReady,
		Status: metav1.ConditionFalse,
		Reason: "NotApplicable",
	})

	status := buildAIPipelinesStatus(module, []dspav1.DataSciencePipelinesApplication{ready, notApplicable})

	argo := requireModuleCondition(t, status.Conditions, conditionTypeArgoReady)
	require.Equal(t, metav1.ConditionTrue, argo.Status)
	require.Equal(t, "All 1 applicable DSPA instance(s) report ready", argo.Message)
}

func TestBuildAIPipelinesStatusAggregatesDSPAAndArgoReadiness(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	dspa := readyTestDSPA("team-a", "pipelines")
	dspa.Status.Conditions = append(dspa.Status.Conditions, metav1.Condition{
		Type:   config.WorkflowControllerReady,
		Status: metav1.ConditionFalse,
		Reason: "DeploymentUnavailable",
	})

	status := buildAIPipelinesStatus(module, []dspav1.DataSciencePipelinesApplication{dspa})

	require.Equal(t, common.PhaseNotReady, status.Phase)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).Status)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeDSPOReady).Status)
	argo := requireModuleCondition(t, status.Conditions, conditionTypeArgoReady)
	require.Equal(t, metav1.ConditionFalse, argo.Status)
	require.Contains(t, argo.Message, "team-a/pipelines")
}

func TestBuildAIPipelinesStatusRemovedArgoDoesNotBlockReadiness(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Removed)
	dspa := readyTestDSPA("team-a", "pipelines")
	dspa.Status.Conditions = append(dspa.Status.Conditions, metav1.Condition{
		Type:   config.WorkflowControllerReady,
		Status: metav1.ConditionFalse,
		Reason: "DeploymentUnavailable",
	})

	status := buildAIPipelinesStatus(module, []dspav1.DataSciencePipelinesApplication{dspa})

	require.Equal(t, common.PhaseReady, status.Phase)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeArgoReady).Status)
}

func TestBuildAIPipelinesStatusRejectsInvalidManagementState(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.ManagementState("Invalid"))
	status := buildAIPipelinesStatus(module, nil)

	require.Equal(t, common.PhaseNotReady, status.Phase)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, conditionTypeConfigurationValid).Status)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeProvisioningSucceeded)).Status)
}

func TestBuildAIPipelinesStatusPreservesTransitionTime(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	transitionTime := metav1.NewTime(time.Unix(123, 0))
	module.Status.Conditions = []common.Condition{{
		Type:               string(common.ConditionTypeReady),
		Status:             metav1.ConditionTrue,
		LastTransitionTime: transitionTime,
	}}

	status := buildAIPipelinesStatus(module, nil)
	require.Equal(t, transitionTime, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).LastTransitionTime)
}

func TestAIPipelinesReconcileUpdatesStatus(t *testing.T) {
	viper.Set("DSPO.PlatformVersion", "v3.6.0")
	t.Cleanup(viper.Reset)

	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	require.NoError(t, dspav1.AddToScheme(scheme))
	module := newTestAIPipelines(common.Managed)
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aipipelinesv1alpha1.AIPipelines{}).
		WithObjects(module).
		Build()
	reconciler := &AIPipelinesReconciler{Client: client, Scheme: scheme}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
	}})
	require.NoError(t, err)

	updated := &aipipelinesv1alpha1.AIPipelines{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: module.Name}, updated))
	require.Equal(t, common.PhaseReady, updated.Status.Phase)
	require.Equal(t, "v3.6.0", updated.Status.GetPlatformRelease())
}

func TestAIPipelinesReconcileIgnoresNonSingleton(t *testing.T) {
	t.Cleanup(viper.Reset)

	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	require.NoError(t, dspav1.AddToScheme(scheme))
	module := newTestAIPipelines(common.Managed)
	module.Name = "legacy-aipipelines"
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aipipelinesv1alpha1.AIPipelines{}).
		WithObjects(module).
		Build()
	reconciler := &AIPipelinesReconciler{Client: client, Scheme: scheme}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: module.Name,
	}})
	require.NoError(t, err)

	updated := &aipipelinesv1alpha1.AIPipelines{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: module.Name}, updated))
	require.Empty(t, updated.Status)
}

func newTestAIPipelines(state common.ManagementState) *aipipelinesv1alpha1.AIPipelines {
	return &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName},
		Spec: aipipelinesv1alpha1.AIPipelinesSpec{
			ArgoWorkflowsControllers: &aipipelinesv1alpha1.ArgoWorkflowsControllersSpec{ManagementState: state},
		},
	}
}

func readyTestDSPA(namespace, name string) dspav1.DataSciencePipelinesApplication {
	return dspav1.DataSciencePipelinesApplication{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Status: dspav1.DSPAStatus{Conditions: []metav1.Condition{{
			Type:   config.CrReady,
			Status: metav1.ConditionTrue,
		}}},
	}
}

func requireModuleCondition(t *testing.T, conditions []common.Condition, conditionType string) common.Condition {
	t.Helper()
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return conditions[i]
		}
	}
	t.Fatalf("condition %q not found", conditionType)
	return common.Condition{}
}
