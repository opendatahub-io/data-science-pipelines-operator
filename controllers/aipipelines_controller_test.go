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
	argoassets "github.com/opendatahub-io/data-science-pipelines-operator/config/argo"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildAIPipelinesStatusReady(t *testing.T) {
	viper.Set("DSPO.PlatformVersion", "v3.6.0")
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	module.Generation = 4

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation())

	require.Equal(t, common.PhaseReady, status.Phase)
	require.EqualValues(t, 4, status.ObservedGeneration)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).Status)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeDSPOReady).Status)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeArgoReady).Status)
	require.Equal(t, "v3.6.0", status.GetPlatformRelease())
}

func TestBuildAIPipelinesStatusReportsDSPODeploymentReadiness(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	dspo := dspoDeploymentObservation{Status: metav1.ConditionFalse, Reason: "DSPOUnavailable", Message: "waiting"}

	status := buildAIPipelinesStatus(module, dspo, readyArgoObservation())

	require.Equal(t, common.PhaseNotReady, status.Phase)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).Status)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, conditionTypeDSPOReady).Status)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeArgoReady).Status)
}

func TestBuildAIPipelinesStatusRemovedArgoDoesNotBlockReadiness(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Removed)

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), argoLifecycleObservation{Status: metav1.ConditionTrue, Reason: "Removed", Message: "removed"})

	require.Equal(t, common.PhaseReady, status.Phase)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeArgoReady).Status)
}

func TestBuildAIPipelinesStatusRejectsInvalidManagementState(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.ManagementState("Invalid"))
	status := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation())

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

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation())
	require.Equal(t, transitionTime, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).LastTransitionTime)
}

func TestAIPipelinesReconcileUpdatesStatus(t *testing.T) {
	viper.Set("DSPO.PlatformVersion", "v3.6.0")
	t.Cleanup(viper.Reset)

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := newTestAIPipelines(common.Managed)
	dspoDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: dspoDeploymentName, Namespace: "opendatahub", Generation: 1},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Conditions:         []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}},
		},
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aipipelinesv1alpha1.AIPipelines{}).
		WithObjects(module, dspoDeployment).
		Build()
	assets, err := argoassets.Objects("opendatahub")
	require.NoError(t, err)
	for _, asset := range assets {
		annotations := asset.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[argoManagedAnnotation] = "true"
		asset.SetAnnotations(annotations)
		labels := asset.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[legacyPipelinesComponentLabel] = "true"
		labels[config.DSPVersionk8sLabel] = config.DSPV2VersionString
		asset.SetLabels(labels)
		require.NoError(t, client.Create(context.Background(), asset))
	}
	reconciler := &AIPipelinesReconciler{Client: client, APIReader: client, Scheme: scheme, Namespace: "opendatahub"}

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
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

func readyArgoObservation() argoLifecycleObservation {
	return argoLifecycleObservation{Status: metav1.ConditionTrue, Reason: "ArgoResourcesReady", Message: "ready"}
}

func readyDSPOObservation() dspoDeploymentObservation {
	return dspoDeploymentObservation{Status: metav1.ConditionTrue, Reason: "DSPOAvailable", Message: "ready"}
}

func newTestAIPipelines(state common.ManagementState) *aipipelinesv1alpha1.AIPipelines {
	return &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName},
		Spec: aipipelinesv1alpha1.AIPipelinesSpec{
			ArgoWorkflowsControllers: &aipipelinesv1alpha1.ArgoWorkflowsControllersSpec{ManagementState: state},
		},
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
