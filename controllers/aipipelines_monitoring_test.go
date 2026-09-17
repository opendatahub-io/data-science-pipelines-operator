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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
}
