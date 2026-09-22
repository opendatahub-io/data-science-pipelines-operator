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
	monitoringassets "github.com/opendatahub-io/data-science-pipelines-operator/config/prometheus"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestReconcilePrometheusRuleCreatesModuleOwnedRule(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName, UID: "module-uid"},
		Spec: aipipelinesv1alpha1.AIPipelinesSpec{
			ArgoWorkflowsControllers: &aipipelinesv1alpha1.ArgoWorkflowsControllersSpec{ManagementState: common.Managed},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module).Build()
	reconciler := &AIPipelinesReconciler{Client: k8sClient, Scheme: scheme, Namespace: "opendatahub"}

	require.NoError(t, reconciler.reconcilePrometheusRule(ctx, module))

	rule := &unstructured.Unstructured{}
	rule.SetAPIVersion("monitoring.rhobs/v1")
	rule.SetKind("PrometheusRule")
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{
		Name: monitoringassets.RuleName, Namespace: "opendatahub",
	}, rule))
	require.Len(t, rule.GetOwnerReferences(), 1)
	require.Equal(t, module.UID, rule.GetOwnerReferences()[0].UID)
}

func TestReconcilePrometheusRuleAdoptsAndRepairsExistingRule(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName, UID: "module-uid"},
	}
	existing, err := monitoringassets.Rule("opendatahub")
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedSlice(existing.Object, []interface{}{}, "spec", "groups"))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module, existing).Build()
	reconciler := &AIPipelinesReconciler{Client: k8sClient, Scheme: scheme, Namespace: "opendatahub"}

	require.NoError(t, reconciler.reconcilePrometheusRule(ctx, module))

	updated := &unstructured.Unstructured{}
	updated.SetGroupVersionKind(existing.GroupVersionKind())
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), updated))
	groups, found, err := unstructured.NestedSlice(updated.Object, "spec", "groups")
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, groups)
	require.Len(t, updated.GetOwnerReferences(), 1)
	require.Equal(t, module.UID, updated.GetOwnerReferences()[0].UID)
}

func TestReconcilePrometheusRuleRepairsRuleAlreadyOwnedByModule(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName, UID: "module-uid"},
	}
	existing, err := monitoringassets.Rule("opendatahub")
	require.NoError(t, err)
	require.NoError(t, controllerutil.SetControllerReference(module, existing, scheme))
	require.NoError(t, unstructured.SetNestedSlice(existing.Object, []interface{}{}, "spec", "groups"))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module, existing).Build()
	reconciler := &AIPipelinesReconciler{Client: k8sClient, Scheme: scheme, Namespace: "opendatahub"}

	require.NoError(t, reconciler.reconcilePrometheusRule(ctx, module))

	updated := &unstructured.Unstructured{}
	updated.SetGroupVersionKind(existing.GroupVersionKind())
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), updated))
	groups, found, err := unstructured.NestedSlice(updated.Object, "spec", "groups")
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, groups)
	require.True(t, metav1.IsControlledBy(updated, module))
}

func TestReconcilePrometheusRuleRefusesForeignControllerOwner(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName, UID: "module-uid"},
	}
	existing, err := monitoringassets.Rule("opendatahub")
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedSlice(existing.Object, []interface{}{}, "spec", "groups"))
	isController := true
	existing.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "example.io/v1",
		Kind:       "ForeignController",
		Name:       "foreign-owner",
		UID:        "foreign-uid",
		Controller: &isController,
	}})
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module, existing).Build()
	reconciler := &AIPipelinesReconciler{Client: k8sClient, Scheme: scheme, Namespace: "opendatahub"}

	err = reconciler.reconcilePrometheusRule(ctx, module)
	require.ErrorContains(t, err, "foreign-owner")

	unchanged := &unstructured.Unstructured{}
	unchanged.SetGroupVersionKind(existing.GroupVersionKind())
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), unchanged))
	require.EqualValues(t, "foreign-uid", metav1.GetControllerOf(unchanged).UID)
	groups, found, err := unstructured.NestedSlice(unchanged.Object, "spec", "groups")
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, groups)
}

func TestReconcileMonitoringResourcesCreatesBothServiceMonitors(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	module := &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName, UID: "module-uid"},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module).Build()
	reconciler := &AIPipelinesReconciler{
		Client: k8sClient, APIReader: k8sClient, Scheme: scheme,
		Namespace: monitoringassets.RHOAIApplicationsNamespace,
	}

	require.NoError(t, reconciler.reconcileMonitoringResources(ctx, module))

	core, err := monitoringassets.CoreServiceMonitor(monitoringassets.RHOAIApplicationsNamespace)
	require.NoError(t, err)
	rhoai, err := monitoringassets.RHOAIServiceMonitor()
	require.NoError(t, err)
	rule, err := monitoringassets.Rule(monitoringassets.RHOAIApplicationsNamespace)
	require.NoError(t, err)
	binding, err := monitoringassets.RHOAIMetricsReaderRoleBinding()
	require.NoError(t, err)
	for _, object := range []*unstructured.Unstructured{core, rhoai, rule, binding} {
		actual := &unstructured.Unstructured{}
		actual.SetGroupVersionKind(object.GroupVersionKind())
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(object), actual))
		require.True(t, metav1.IsControlledBy(actual, module), "%s/%s", actual.GetKind(), actual.GetName())
	}
}

func TestReconcileMonitoringObjectHandlesAPIChangesDuringUpdate(t *testing.T) {
	for _, test := range []struct {
		name              string
		updateError       error
		expectedSupported bool
	}{
		{
			name: "API removed",
			updateError: &meta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{
				Group: "monitoring.rhobs", Version: "v1", Resource: "prometheusrules",
			}},
			expectedSupported: false,
		},
		{
			name: "object deleted",
			updateError: apierrors.NewNotFound(schema.GroupResource{
				Group: "monitoring.rhobs", Resource: "prometheusrules",
			}, monitoringassets.RuleName),
			expectedSupported: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
			module := &aipipelinesv1alpha1.AIPipelines{
				ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName, UID: "module-uid"},
			}
			existing, err := monitoringassets.Rule("opendatahub")
			require.NoError(t, err)
			require.NoError(t, unstructured.SetNestedSlice(existing.Object, []interface{}{}, "spec", "groups"))
			baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(module, existing).Build()
			reconciler := &AIPipelinesReconciler{
				Client:    &updateErrorClient{Client: baseClient, err: test.updateError},
				APIReader: baseClient,
				Scheme:    scheme,
				Namespace: "opendatahub",
			}
			desired, err := monitoringassets.Rule("opendatahub")
			require.NoError(t, err)

			supported, err := reconciler.reconcileMonitoringObject(ctx, module, desired, "spec")
			require.NoError(t, err)
			require.Equal(t, test.expectedSupported, supported)
		})
	}
}

type updateErrorClient struct {
	client.Client
	err error
}

func (c *updateErrorClient) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return c.err
}
