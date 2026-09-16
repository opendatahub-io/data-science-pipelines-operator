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

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	argoassets "github.com/opendatahub-io/data-science-pipelines-operator/config/argo"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAIPipelinesArgoReconcileManagedCreatesAssets(t *testing.T) {
	ctx := context.Background()
	reconciler, k8sClient, module := newArgoTestReconciler(t, common.Managed)

	result, err := reconciler.Reconcile(ctx, moduleRequest())
	require.NoError(t, err)
	require.True(t, result.Requeue)
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(module), module))
	require.Contains(t, module.Finalizers, argoLifecycleFinalizer)

	_, err = reconciler.Reconcile(ctx, moduleRequest())
	require.NoError(t, err)

	configMap := argoTestObject("v1", "ConfigMap", "opendatahub", "workflow-controller-configmap")
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(configMap), configMap))
	require.Equal(t, "true", configMap.GetAnnotations()[argoManagedAnnotation])
	require.Len(t, configMap.GetOwnerReferences(), 1)
	binding := argoTestObject("rbac.authorization.k8s.io/v1", "ClusterRoleBinding", "", "ds-pipeline-argo-binding")
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	subjects, found, err := unstructured.NestedSlice(binding.Object, "subjects")
	require.NoError(t, err)
	require.True(t, found)
	subjectNamespace := subjects[0].(map[string]interface{})["namespace"].(string)
	require.Equal(t, "opendatahub", subjectNamespace)

	setArgoCRDsEstablished(t, ctx, k8sClient, "opendatahub", metav1.ConditionFalse)
	observation := observeArgoLifecycle(ctx, k8sClient, "opendatahub", common.Managed, module)
	require.Equal(t, metav1.ConditionFalse, observation.Status, observation.Message)
	require.Equal(t, "ArgoResourcesProgressing", observation.Reason)

	setArgoCRDsEstablished(t, ctx, k8sClient, "opendatahub", metav1.ConditionTrue)
	observation = observeArgoLifecycle(ctx, k8sClient, "opendatahub", common.Managed, module)
	require.Equal(t, metav1.ConditionTrue, observation.Status)

	configMap.SetOwnerReferences(nil)
	require.NoError(t, k8sClient.Update(ctx, configMap))
	observation = observeArgoLifecycle(ctx, k8sClient, "opendatahub", common.Managed, module)
	require.Equal(t, metav1.ConditionFalse, observation.Status)
	require.Equal(t, "ArgoResourcesProgressing", observation.Reason)

	_, err = reconciler.Reconcile(ctx, moduleRequest())
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(configMap), configMap))
	observation = observeArgoLifecycle(ctx, k8sClient, "opendatahub", common.Managed, module)
	require.Equal(t, metav1.ConditionTrue, observation.Status)

	require.NoError(t, unstructured.SetNestedField(configMap.Object, map[string]interface{}{"unexpected": "drift"}, "data"))
	require.NoError(t, k8sClient.Update(ctx, configMap))
	observation = observeArgoLifecycle(ctx, k8sClient, "opendatahub", common.Managed, module)
	require.Equal(t, metav1.ConditionFalse, observation.Status)
	require.Equal(t, "ArgoResourcesProgressing", observation.Reason)

	_, err = reconciler.Reconcile(ctx, moduleRequest())
	require.NoError(t, err)
	observation = observeArgoLifecycle(ctx, k8sClient, "opendatahub", common.Managed, module)
	require.Equal(t, metav1.ConditionTrue, observation.Status)
}

func TestAIPipelinesArgoReconcileRemovedPreservesCRDs(t *testing.T) {
	ctx := context.Background()
	reconciler, k8sClient, module := newArgoTestReconciler(t, common.Removed)
	module.Finalizers = []string{argoLifecycleFinalizer}
	require.NoError(t, k8sClient.Update(ctx, module))

	assets, err := argoassets.Objects("opendatahub")
	require.NoError(t, err)
	for _, asset := range assets {
		annotations := asset.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[argoManagedAnnotation] = "true"
		asset.SetAnnotations(annotations)
		require.NoError(t, k8sClient.Create(ctx, asset))
	}

	_, err = reconciler.Reconcile(ctx, moduleRequest())
	require.NoError(t, err)

	configMap := argoTestObject("v1", "ConfigMap", "opendatahub", "workflow-controller-configmap")
	require.True(t, apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(configMap), configMap)))
	workflowCRD := argoTestObject("apiextensions.k8s.io/v1", "CustomResourceDefinition", "", argoWorkflowCRDName)
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(workflowCRD), workflowCRD))

	observation := observeArgoLifecycle(ctx, k8sClient, "opendatahub", common.Removed, module)
	require.Equal(t, metav1.ConditionTrue, observation.Status)
}

func TestAIPipelinesArgoReconcileDoesNotAdoptForeignAssets(t *testing.T) {
	testCases := []struct {
		name      string
		assetName string
	}{
		{name: "sibling CRD", assetName: "applications.app.k8s.io"},
		{name: "RBAC object", assetName: "argo-cluster-role"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			reconciler, k8sClient, module := newArgoTestReconciler(t, common.Managed)
			module.Finalizers = []string{argoLifecycleFinalizer}
			require.NoError(t, k8sClient.Update(ctx, module))

			assets, err := argoassets.Objects("opendatahub")
			require.NoError(t, err)
			for _, asset := range assets {
				if asset.GetName() == testCase.assetName {
					require.NoError(t, k8sClient.Create(ctx, asset))
					break
				}
			}

			_, err = reconciler.Reconcile(ctx, moduleRequest())
			require.ErrorContains(t, err, "not owned by AI Pipelines")
			firstAsset := argoTestObject("rbac.authorization.k8s.io/v1", "ClusterRole", "", "argo-aggregate-to-admin")
			require.True(t, apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(firstAsset), firstAsset)))
		})
	}
}

func TestAIPipelinesArgoReconcileAdoptsLegacyOwnedAssets(t *testing.T) {
	ctx := context.Background()
	reconciler, k8sClient, module := newArgoTestReconciler(t, common.Managed)
	module.Finalizers = []string{argoLifecycleFinalizer}
	require.NoError(t, k8sClient.Update(ctx, module))

	assets, err := argoassets.Objects("opendatahub")
	require.NoError(t, err)
	for _, asset := range assets {
		asset.SetLabels(map[string]string{legacyPipelinesComponentLabel: "true"})
		require.NoError(t, k8sClient.Create(ctx, asset))
	}

	_, err = reconciler.Reconcile(ctx, moduleRequest())
	require.NoError(t, err)

	configMap := argoTestObject("v1", "ConfigMap", "opendatahub", "workflow-controller-configmap")
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(configMap), configMap))
	require.Equal(t, "true", configMap.GetAnnotations()[argoManagedAnnotation])
	require.Len(t, configMap.GetOwnerReferences(), 1)
}

func TestAIPipelinesArgoReconcileIgnoresNonSingleton(t *testing.T) {
	ctx := context.Background()
	reconciler, k8sClient, module := newArgoTestReconciler(t, common.Managed)
	module.Name = "legacy-aipipelines"
	module.ResourceVersion = ""
	require.NoError(t, k8sClient.Create(ctx, module))

	result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: module.Name}})
	require.NoError(t, err)
	require.False(t, result.Requeue)

	updated := &aipipelinesv1alpha1.AIPipelines{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(module), updated))
	require.Empty(t, updated.Finalizers)
}

func TestAIPipelinesArgoReconcileFinalizesNonSingleton(t *testing.T) {
	ctx := context.Background()
	reconciler, k8sClient, module := newArgoTestReconciler(t, common.Managed)
	managedConfigMap := argoTestObject("v1", "ConfigMap", "opendatahub", "workflow-controller-configmap")
	managedConfigMap.SetAnnotations(map[string]string{argoManagedAnnotation: "true"})
	require.NoError(t, k8sClient.Create(ctx, managedConfigMap))

	module.Name = "legacy-aipipelines"
	module.ResourceVersion = ""
	module.Finalizers = []string{argoLifecycleFinalizer}
	require.NoError(t, k8sClient.Create(ctx, module))
	require.NoError(t, k8sClient.Delete(ctx, module))

	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: module.Name}})
	require.NoError(t, err)

	updated := &aipipelinesv1alpha1.AIPipelines{}
	require.True(t, apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(module), updated)))
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(managedConfigMap), managedConfigMap))
}

func newArgoTestReconciler(t *testing.T, state common.ManagementState) (*AIPipelinesArgoReconciler, client.Client, *aipipelinesv1alpha1.AIPipelines) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := newTestAIPipelines(state)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module).Build()
	return &AIPipelinesArgoReconciler{
		Client:    k8sClient,
		APIReader: k8sClient,
		Scheme:    scheme,
		Namespace: "opendatahub",
	}, k8sClient, module.DeepCopy()
}

func moduleRequest() ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}}
}

func argoTestObject(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{}
	object.SetAPIVersion(apiVersion)
	object.SetKind(kind)
	object.SetNamespace(namespace)
	object.SetName(name)
	return object
}

func setArgoCRDsEstablished(t *testing.T, ctx context.Context, k8sClient client.Client, namespace string, status metav1.ConditionStatus) {
	t.Helper()
	assets, err := argoassets.Objects(namespace)
	require.NoError(t, err)
	for _, asset := range assets {
		if asset.GetKind() != "CustomResourceDefinition" {
			continue
		}
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(asset.GroupVersionKind())
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(asset), current))
		require.NoError(t, unstructured.SetNestedSlice(current.Object, []interface{}{
			map[string]interface{}{
				"type":   string(apiextensionsv1.Established),
				"status": string(status),
			},
		}, "status", "conditions"))
		require.NoError(t, k8sClient.Status().Update(ctx, current))
	}
}
