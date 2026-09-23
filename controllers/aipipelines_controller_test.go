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
	"errors"
	"testing"
	"time"

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	argoassets "github.com/opendatahub-io/data-science-pipelines-operator/config/argo"
	monitoringassets "github.com/opendatahub-io/data-science-pipelines-operator/config/prometheus"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestBuildAIPipelinesStatusReady(t *testing.T) {
	viper.Set("DSPO.PlatformVersion", "v3.6.0")
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	module.Generation = 4

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation(), readyMonitoringObservation())

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

	status := buildAIPipelinesStatus(module, dspo, readyArgoObservation(), readyMonitoringObservation())

	require.Equal(t, common.PhaseNotReady, status.Phase)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).Status)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, conditionTypeDSPOReady).Status)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeArgoReady).Status)
}

func TestBuildAIPipelinesStatusRemovedArgoDoesNotBlockReadiness(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Removed)

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), argoLifecycleObservation{Status: metav1.ConditionTrue, Reason: "Removed", Message: "removed"}, readyMonitoringObservation())

	require.Equal(t, common.PhaseReady, status.Phase)
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, status.Conditions, conditionTypeArgoReady).Status)
}

func TestBuildAIPipelinesStatusRejectsInvalidManagementState(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.ManagementState("Invalid"))
	status := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation(), readyMonitoringObservation())

	require.Equal(t, common.PhaseNotReady, status.Phase)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, conditionTypeConfigurationValid).Status)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeProvisioningSucceeded)).Status)
}

func TestBuildAIPipelinesStatusReportsManifestApplicationFailure(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	argo := argoLifecycleObservation{
		Status:  metav1.ConditionFalse,
		Reason:  "ArgoResourcesProgressing",
		Message: "Waiting for shared Argo resources: ConfigMap/workflow-controller-configmap",
	}

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), argo, readyMonitoringObservation())
	provisioning := requireModuleCondition(t, status.Conditions, string(common.ConditionTypeProvisioningSucceeded))
	require.Equal(t, metav1.ConditionFalse, provisioning.Status)
	require.Equal(t, argo.Reason, provisioning.Reason)
	require.Equal(t, argo.Message, provisioning.Message)
}

func TestBuildAIPipelinesStatusReportsMonitoringFailure(t *testing.T) {
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	monitoring := monitoringObservation{
		Status:  metav1.ConditionFalse,
		Reason:  "MonitoringReconcileFailed",
		Message: "forbidden",
	}

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation(), monitoring)

	require.Equal(t, common.PhaseNotReady, status.Phase)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).Status)
	require.Equal(t, metav1.ConditionFalse, requireModuleCondition(t, status.Conditions, conditionTypeMonitoringReady).Status)
	provisioning := requireModuleCondition(t, status.Conditions, string(common.ConditionTypeProvisioningSucceeded))
	require.Equal(t, metav1.ConditionFalse, provisioning.Status)
	require.Equal(t, monitoring.Reason, provisioning.Reason)
	require.Equal(t, monitoring.Message, provisioning.Message)
}

func TestBuildAIPipelinesStatusPreservesPlatformReleaseUntilManifestsApply(t *testing.T) {
	viper.Set("DSPO.PlatformVersion", "3.6.0")
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	module.Status.SetPlatformRelease("3.5.0")
	argo := argoLifecycleObservation{
		Status:  metav1.ConditionFalse,
		Reason:  "ArgoResourcesProgressing",
		Message: "Waiting for shared Argo resources: ConfigMap/workflow-controller-configmap",
	}

	failedStatus := buildAIPipelinesStatus(module, readyDSPOObservation(), argo, readyMonitoringObservation())
	require.Equal(t, "3.5.0", failedStatus.GetPlatformRelease())

	module.Status = failedStatus
	successfulStatus := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation(), readyMonitoringObservation())
	require.Equal(t, "3.6.0", successfulStatus.GetPlatformRelease())
}

func TestBuildAIPipelinesStatusAcknowledgesLivePlatformVersion(t *testing.T) {
	viper.Set("DSPO.PlatformVersion", "3.5.0")
	t.Cleanup(viper.Reset)

	module := newTestAIPipelines(common.Managed)
	module.Status.SetPlatformRelease("3.5.0")
	platformConfig := platformConfigObservation{
		Status:  metav1.ConditionTrue,
		Reason:  "PlatformConfigurationAvailable",
		Message: "Platform configuration is available",
		Version: "3.6.0",
	}

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation(), readyMonitoringObservation(), platformConfig)
	require.Equal(t, "3.6.0", status.GetPlatformRelease())
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

	status := buildAIPipelinesStatus(module, readyDSPOObservation(), readyArgoObservation(), readyMonitoringObservation())
	require.Equal(t, transitionTime, requireModuleCondition(t, status.Conditions, string(common.ConditionTypeReady)).LastTransitionTime)
}

func TestAIPipelinesReconcileUpdatesStatus(t *testing.T) {
	viper.Set(config.EnableAIPipelinesModuleControllerConfigName, true)
	viper.Set("DSPO.PlatformVersion", "3.6.0")
	t.Cleanup(viper.Reset)
	t.Setenv(applicationsNamespaceEnv, "opendatahub")

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	scheme.AddKnownTypeWithName(legacyDataSciencePipelinesGVK, &unstructured.Unstructured{})
	module := newTestAIPipelines(common.Managed)
	legacy := newLegacyDataSciencePipelines()
	legacy.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "datasciencecluster.opendatahub.io/v2",
		Kind:       "DataScienceCluster",
		Name:       "default-dsc",
	}})
	dspoDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: dspoDeploymentName, Namespace: "opendatahub", Generation: 1},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Conditions:         []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}},
		},
	}
	platformConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: platformConfigMapName, Namespace: "opendatahub"},
		Data:       map[string]string{platformVersionKey: "3.6.0"},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aipipelinesv1alpha1.AIPipelines{}).
		WithObjects(module, dspoDeployment, platformConfig, legacy).
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
		if asset.GetKind() != "CustomResourceDefinition" {
			require.NoError(t, controllerutil.SetControllerReference(module, asset, scheme))
		}
		require.NoError(t, k8sClient.Create(context.Background(), asset))
	}
	setArgoCRDsEstablished(t, context.Background(), k8sClient, "opendatahub", metav1.ConditionTrue)
	reconciler := &AIPipelinesReconciler{
		Client: k8sClient, APIReader: k8sClient, Scheme: scheme,
		Namespace: "opendatahub", MonitoringNamespace: "odh-monitoring",
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
	}})
	require.NoError(t, err)
	require.Equal(t, config.DefaultRequeueTime, result.RequeueAfter)

	updated := &aipipelinesv1alpha1.AIPipelines{}
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Name: module.Name}, updated))
	require.Equal(t, common.PhaseReady, updated.Status.Phase)
	require.Equal(t, "3.6.0", updated.Status.GetPlatformRelease())
	require.Equal(t, metav1.ConditionTrue, requireModuleCondition(t, updated.Status.Conditions, "PlatformConfigurationValid").Status)
	require.True(t, apierrors.IsNotFound(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(legacy), newLegacyDataSciencePipelines())))

	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(platformConfig), platformConfig))
	platformConfig.Data[platformVersionKey] = "3.7.0"
	require.NoError(t, k8sClient.Update(context.Background(), platformConfig))

	result, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
	}})
	require.NoError(t, err)
	require.Equal(t, config.DefaultRequeueTime, result.RequeueAfter)
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Name: module.Name}, updated))
	require.Equal(t, "3.7.0", updated.Status.GetPlatformRelease())

	rule, err := monitoringassets.Rule("opendatahub")
	require.NoError(t, err)
	ruleKey := types.NamespacedName{Name: rule.GetName(), Namespace: rule.GetNamespace()}
	require.NoError(t, k8sClient.Get(context.Background(), ruleKey, rule))
	require.NoError(t, k8sClient.Delete(context.Background(), rule))

	result, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
	}})
	require.NoError(t, err)
	require.Equal(t, config.DefaultRequeueTime, result.RequeueAfter)
	require.NoError(t, k8sClient.Get(context.Background(), ruleKey, rule))
}

func TestAIPipelinesReconcileRequeuesWhileNotReady(t *testing.T) {
	viper.Set(config.EnableAIPipelinesModuleControllerConfigName, true)
	t.Cleanup(viper.Reset)
	t.Setenv(applicationsNamespaceEnv, "opendatahub")

	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	scheme.AddKnownTypeWithName(legacyDataSciencePipelinesGVK, &unstructured.Unstructured{})
	module := newTestAIPipelines(common.Managed)
	legacy := newLegacyDataSciencePipelines()
	legacy.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "datasciencecluster.opendatahub.io/v2",
		Kind:       "DataScienceCluster",
		Name:       "default-dsc",
	}})
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aipipelinesv1alpha1.AIPipelines{}).
		WithObjects(module, legacy).
		Build()
	reconciler := &AIPipelinesReconciler{
		Client: k8sClient, APIReader: k8sClient, Scheme: scheme, Namespace: "opendatahub",
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
	}})
	require.NoError(t, err)
	require.Equal(t, config.DefaultRequeueTime, result.RequeueAfter)
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(legacy), newLegacyDataSciencePipelines()))
}

func TestAIPipelinesReconcileUpdatesStatusWhenPrometheusRuleCRDWasRemoved(t *testing.T) {
	viper.Set(config.EnableAIPipelinesModuleControllerConfigName, true)
	t.Cleanup(viper.Reset)
	t.Setenv(applicationsNamespaceEnv, "opendatahub")

	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := newTestAIPipelines(common.Managed)
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aipipelinesv1alpha1.AIPipelines{}).
		WithObjects(module).
		Build()
	k8sClient := &prometheusRuleCreateNotFoundClient{Client: baseClient}
	reconciler := &AIPipelinesReconciler{
		Client: k8sClient, APIReader: baseClient, Scheme: scheme,
		Namespace: "opendatahub", MonitoringNamespace: "odh-monitoring",
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
	}})
	require.NoError(t, err)
	require.Equal(t, config.DefaultRequeueTime, result.RequeueAfter)

	updated := &aipipelinesv1alpha1.AIPipelines{}
	require.NoError(t, baseClient.Get(context.Background(), client.ObjectKeyFromObject(module), updated))
	require.Equal(t, common.PhaseNotReady, updated.Status.Phase)
	require.NotEmpty(t, updated.Status.Conditions)
}

func TestAIPipelinesReconcileReportsMonitoringFailure(t *testing.T) {
	t.Cleanup(viper.Reset)
	t.Setenv(applicationsNamespaceEnv, "opendatahub")

	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := newTestAIPipelines(common.Managed)
	module.UID = "module-uid"
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aipipelinesv1alpha1.AIPipelines{}).
		WithObjects(module).
		Build()
	k8sClient := &clusterRoleBindingCreateForbiddenClient{Client: baseClient}
	reconciler := &AIPipelinesReconciler{
		Client: k8sClient, APIReader: baseClient, Scheme: scheme,
		Namespace: "opendatahub", MonitoringNamespace: "odh-monitoring",
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
	}})
	require.ErrorContains(t, err, "reconcile RHOAI metrics reader binding")

	updated := &aipipelinesv1alpha1.AIPipelines{}
	require.NoError(t, baseClient.Get(context.Background(), client.ObjectKeyFromObject(module), updated))
	monitoring := requireModuleCondition(t, updated.Status.Conditions, conditionTypeMonitoringReady)
	require.Equal(t, metav1.ConditionFalse, monitoring.Status)
	require.Equal(t, "MonitoringReconcileFailed", monitoring.Reason)
	require.Contains(t, monitoring.Message, "forbidden")
}

func TestAIPipelinesReconcileRequeuesStatusConflictWithoutError(t *testing.T) {
	t.Cleanup(viper.Reset)

	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := newTestAIPipelines(common.Managed)
	module.UID = "module-uid"
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aipipelinesv1alpha1.AIPipelines{}).
		WithObjects(module).
		Build()
	k8sClient := &statusConflictClient{Client: baseClient}
	reconciler := &AIPipelinesReconciler{
		Client: k8sClient, APIReader: baseClient, Scheme: scheme, Namespace: "opendatahub",
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
	}})
	require.NoError(t, err)
	require.True(t, result.Requeue)
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

func readyMonitoringObservation() monitoringObservation {
	return monitoringObservation{Status: metav1.ConditionTrue, Reason: "MonitoringResourcesReady", Message: "ready"}
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

type prometheusRuleCreateNotFoundClient struct {
	client.Client
}

type clusterRoleBindingCreateForbiddenClient struct {
	client.Client
}

func (c *clusterRoleBindingCreateForbiddenClient) Create(
	ctx context.Context,
	object client.Object,
	opts ...client.CreateOption,
) error {
	if object.GetObjectKind().GroupVersionKind().Kind == "ClusterRoleBinding" {
		return apierrors.NewForbidden(schema.GroupResource{
			Group: "rbac.authorization.k8s.io", Resource: "clusterrolebindings",
		}, object.GetName(), errors.New("attempting to grant RBAC permissions not currently held"))
	}
	return c.Client.Create(ctx, object, opts...)
}

type statusConflictClient struct {
	client.Client
}

func (c *statusConflictClient) Status() client.SubResourceWriter {
	return &statusConflictWriter{delegate: c.Client.Status()}
}

type statusConflictWriter struct {
	delegate client.SubResourceWriter
}

func (w *statusConflictWriter) Create(
	ctx context.Context,
	object client.Object,
	subResource client.Object,
	opts ...client.SubResourceCreateOption,
) error {
	return w.delegate.Create(ctx, object, subResource, opts...)
}

func (w *statusConflictWriter) Update(
	_ context.Context,
	object client.Object,
	_ ...client.SubResourceUpdateOption,
) error {
	return apierrors.NewConflict(schema.GroupResource{
		Group: "components.platform.opendatahub.io", Resource: "aipipelines",
	}, object.GetName(), errors.New("object has been modified"))
}

func (w *statusConflictWriter) Patch(
	ctx context.Context,
	object client.Object,
	patch client.Patch,
	opts ...client.SubResourcePatchOption,
) error {
	return w.delegate.Patch(ctx, object, patch, opts...)
}

func (w *statusConflictWriter) Apply(
	ctx context.Context,
	object runtime.ApplyConfiguration,
	opts ...client.SubResourceApplyOption,
) error {
	return w.delegate.Apply(ctx, object, opts...)
}

func (c *prometheusRuleCreateNotFoundClient) Create(
	ctx context.Context,
	object client.Object,
	opts ...client.CreateOption,
) error {
	if object.GetObjectKind().GroupVersionKind().Group == "monitoring.rhobs" &&
		object.GetObjectKind().GroupVersionKind().Kind == "PrometheusRule" {
		return apierrors.NewNotFound(schema.GroupResource{
			Group: "monitoring.rhobs", Resource: "prometheusrules",
		}, object.GetName())
	}
	return c.Client.Create(ctx, object, opts...)
}
