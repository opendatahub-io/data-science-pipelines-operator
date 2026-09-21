//go:build test_integration

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

package aipipelines_test

import (
	"testing"

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const argoCleanupFinalizer = "aipipelines.components.platform.opendatahub.io/argo-cleanup"

var bundledArgoOperandKinds = []schema.GroupVersionKind{
	{Group: "apps", Version: "v1", Kind: "Deployment"},
	{Version: "v1", Kind: "Service"},
	{Version: "v1", Kind: "ConfigMap"},
	{Version: "v1", Kind: "ServiceAccount"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
}

func TestAIPipelinesLifecycle(t *testing.T) {
	t.Run("reports_live_module_status", testLiveModuleStatus)
	t.Run("projects_management_state", testManagementState)
	t.Run("finalizes_module_resources", testFinalizationAndCleanup)
}

func testLiveModuleStatus(t *testing.T) {
	// newFixture already requires the test-labeled handshake and waits for Ready.
	newFixture(t)
}

func testManagementState(t *testing.T) {
	f := newFixture(t)
	f.attachIntegrationDSPA()
	version := f.module().Status.GetPlatformRelease()

	t.Log("project Managed -> Removed -> Managed on shared and DSPA Argo operands")
	blockerKey := client.ObjectKey{Name: "workflow-controller-configmap", Namespace: f.applications}
	const holdFinalizer = "testing.opendatahub.io/hold-argo-cleanup"
	require.NoError(t, f.setConfigMapFinalizer(blockerKey, holdFinalizer, true))
	t.Cleanup(func() { assert.NoError(t, f.setConfigMapFinalizer(blockerKey, holdFinalizer, false)) })

	f.assertBundledArgoOperandsExist()
	f.setState(common.Removed)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		cm := &corev1.ConfigMap{}
		if assert.NoError(c, f.client.Get(f.ctx, blockerKey, cm)) {
			assert.False(c, cm.DeletionTimestamp.IsZero())
		}
	}, deadline, pollInterval)
	f.waitModuleCondition("ArgoWorkflowsControllersReady", metav1.ConditionFalse, version)
	f.waitModuleCondition("ProvisioningSucceeded", metav1.ConditionFalse, version)
	require.NoError(t, f.setConfigMapFinalizer(blockerKey, holdFinalizer, false))
	f.waitModule(common.Removed, version)
	f.assertSharedAssets(true)
	f.assertBundledArgoOperandsRemoved()
	// Bundled Argo removal must not remove the DSPA API server Deployment.
	f.get(client.ObjectKey{Name: "ds-pipeline-" + f.dspa.Name, Namespace: f.namespace}, &appsv1.Deployment{})

	f.setState(common.Managed)
	f.waitModule(common.Managed, version)
	f.assertBundledArgoOperandsExist()
}

func testFinalizationAndCleanup(t *testing.T) {
	f := newFixture(t)
	f.attachIntegrationDSPA()
	version := f.module().Status.GetPlatformRelease()
	clusterRoleBindingKey := client.ObjectKey{Name: "ds-pipeline-ui-auth-delegator-" + f.namespace + "-" + f.dspa.Name}
	f.get(clusterRoleBindingKey, &rbacv1.ClusterRoleBinding{})

	t.Log("hold module deletion until shared Argo cleanup completes")
	blockerKey := client.ObjectKey{Name: "workflow-controller-configmap", Namespace: f.applications}
	const holdFinalizer = "testing.opendatahub.io/hold-module-finalization"
	require.NoError(t, f.setConfigMapFinalizer(blockerKey, holdFinalizer, true))
	t.Cleanup(func() { assert.NoError(t, f.setConfigMapFinalizer(blockerKey, holdFinalizer, false)) })
	module := f.module()
	require.Contains(t, module.Finalizers, argoCleanupFinalizer)
	require.NoError(t, f.client.Delete(f.ctx, module))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		current := &aipipelinesv1alpha1.AIPipelines{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKeyFromObject(module), current)) {
			return
		}
		assert.False(c, current.DeletionTimestamp.IsZero())
		assert.Contains(c, current.Finalizers, argoCleanupFinalizer)
		blocker := &corev1.ConfigMap{}
		if assert.NoError(c, f.client.Get(f.ctx, blockerKey, blocker)) {
			assert.False(c, blocker.DeletionTimestamp.IsZero())
		}
	}, deadline, pollInterval)
	require.NoError(t, f.setConfigMapFinalizer(blockerKey, holdFinalizer, false))
	require.Eventually(t, func() bool {
		return apierrors.IsNotFound(f.client.Get(f.ctx, client.ObjectKeyFromObject(module), &aipipelinesv1alpha1.AIPipelines{}))
	}, deadline, pollInterval)
	f.assertSharedAssets(true)

	require.NoError(t, f.client.Create(f.ctx, &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: module.Name, Labels: module.Labels}, Spec: module.Spec,
	}))
	f.waitModule(common.Managed, version)

	t.Log("DSPA deletion finalizes cluster-scoped bindings and owned operands")
	owned := f.ownedDSPAResources()
	require.NotEmpty(t, owned)
	require.NoError(t, f.client.Delete(f.ctx, f.dspa))
	require.Eventually(t, func() bool {
		return apierrors.IsNotFound(f.client.Get(f.ctx, client.ObjectKeyFromObject(f.dspa), &dspav1.DataSciencePipelinesApplication{}))
	}, deadline, pollInterval)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		for _, object := range owned {
			err := f.client.Get(f.ctx, client.ObjectKeyFromObject(object), object.DeepCopy())
			assert.True(c, apierrors.IsNotFound(err), "%s/%s remains: %v", object.GetKind(), object.GetName(), err)
		}
	}, deadline, pollInterval)
	require.Eventually(t, func() bool {
		return apierrors.IsNotFound(f.client.Get(f.ctx, clusterRoleBindingKey, &rbacv1.ClusterRoleBinding{}))
	}, deadline, pollInterval, "DSPA finalizer did not remove %s", clusterRoleBindingKey.Name)
	f.waitModule(common.Managed, version)
}

func (f *fixture) bundledArgoOperandName(gvk schema.GroupVersionKind) string {
	switch gvk.Kind {
	case "Service":
		return "ds-pipeline-workflow-controller-metrics-" + f.dspa.Name
	case "Role":
		return "ds-pipeline-workflow-controller-role-" + f.dspa.Name
	case "RoleBinding":
		return "ds-pipeline-workflow-controller-rolebinding-" + f.dspa.Name
	default:
		return "ds-pipeline-workflow-controller-" + f.dspa.Name
	}
}

func (f *fixture) assertBundledArgoOperandsExist() {
	f.t.Helper()
	for _, gvk := range bundledArgoOperandKinds {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)
		name := f.bundledArgoOperandName(gvk)
		err := f.client.Get(f.ctx, client.ObjectKey{Name: name, Namespace: f.namespace}, resource)
		require.NoError(f.t, err, "expected Argo %s %s before removal", gvk.Kind, name)
	}
}

func (f *fixture) assertBundledArgoOperandsRemoved() {
	f.t.Helper()
	for _, gvk := range bundledArgoOperandKinds {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)
		name := f.bundledArgoOperandName(gvk)
		require.Eventually(f.t, func() bool {
			return apierrors.IsNotFound(f.client.Get(f.ctx, client.ObjectKey{Name: name, Namespace: f.namespace}, resource))
		}, deadline, pollInterval, "remaining Argo %s %s", gvk.Kind, name)
	}
}

func (f *fixture) setConfigMapFinalizer(key client.ObjectKey, finalizer string, hold bool) error {
	f.t.Helper()
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		cm := &corev1.ConfigMap{}
		if err := f.client.Get(f.ctx, key, cm); err != nil {
			return client.IgnoreNotFound(err)
		}
		finalizers := make([]string, 0, len(cm.Finalizers)+1)
		for _, value := range cm.Finalizers {
			if value != finalizer {
				finalizers = append(finalizers, value)
			}
		}
		if hold {
			finalizers = append(finalizers, finalizer)
		}
		cm.Finalizers = finalizers
		return f.client.Update(f.ctx, cm)
	})
}

func (f *fixture) ownedDSPAResources() []*unstructured.Unstructured {
	f.t.Helper()
	var objects []*unstructured.Unstructured
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "DeploymentList"},
		{Group: "apps", Version: "v1", Kind: "StatefulSetList"},
		{Group: "apps", Version: "v1", Kind: "DaemonSetList"},
		{Version: "v1", Kind: "ServiceList"}, {Version: "v1", Kind: "ConfigMapList"},
		{Version: "v1", Kind: "SecretList"}, {Version: "v1", Kind: "PersistentVolumeClaimList"}, {Version: "v1", Kind: "ServiceAccountList"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleList"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBindingList"},
		{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicyList"},
		{Group: "networking.k8s.io", Version: "v1", Kind: "IngressList"},
		{Group: routev1.GroupVersion.Group, Version: routev1.GroupVersion.Version, Kind: "RouteList"},
	} {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		require.NoError(f.t, f.client.List(f.ctx, list, client.InNamespace(f.namespace)))
		for _, item := range list.Items {
			if item.GetLabels()["dsp-version"] != "" || metav1.IsControlledBy(&item, f.dspa) {
				require.True(f.t, metav1.IsControlledBy(&item, f.dspa), "%s/%s is missing its DSPA owner", item.GetKind(), item.GetName())
				objects = append(objects, item.DeepCopy())
			}
		}
	}
	return objects
}
