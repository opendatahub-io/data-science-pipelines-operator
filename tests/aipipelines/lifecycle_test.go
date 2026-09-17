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
	"encoding/json"
	"testing"

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAIPipelinesLifecycle(t *testing.T) {
	f := newFixture(t)
	version := f.module().Status.GetPlatformRelease()
	t.Log("validate the singleton through the live API and reconcile real DSPA operands")
	invalid := &aipipelinesv1alpha1.AIPipelines{ObjectMeta: metav1.ObjectMeta{Name: "another-module"}}
	require.True(t, apierrors.IsInvalid(f.client.Create(f.ctx, invalid)))
	f.assertSharedAssets(false)
	f.deployDSPA()
	f.waitSampleVersion(version)
	f.apiRequest("GET", "/healthz", nil)
	run := f.runWorkflow("before-reconfiguration")
	runUID := run.GetUID()

	t.Log("propagate live platform configuration to status and DSPA without a spec edit")
	originalDSPA := &dspav1.DataSciencePipelinesApplication{}
	f.get(client.ObjectKeyFromObject(f.dspa), originalDSPA)
	f.setVersion(version + "-e2e")
	f.waitModule(common.Managed, version+"-e2e")
	f.waitSampleVersion(version + "-e2e")
	f.waitOperands()
	currentDSPA := &dspav1.DataSciencePipelinesApplication{}
	f.get(client.ObjectKeyFromObject(f.dspa), currentDSPA)
	require.Equal(t, originalDSPA.Generation, currentDSPA.Generation)
	f.setVersion("")
	f.waitModuleCondition("PlatformConfigurationValid", metav1.ConditionFalse, version+"-e2e")
	f.waitModuleCondition("ProvisioningSucceeded", metav1.ConditionFalse, version+"-e2e")
	f.waitModuleCondition("Ready", metav1.ConditionFalse, version+"-e2e")
	f.setVersion(version)
	f.waitModule(common.Managed, version)
	f.waitSampleVersion(version)
	f.waitOperands()

	t.Log("reflect a DSPA spec change in the operand and recover from pod loss")
	require.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &dspav1.DataSciencePipelinesApplication{}
		if err := f.client.Get(f.ctx, client.ObjectKeyFromObject(f.dspa), current); err != nil {
			return err
		}
		current.Spec.APIServer.EnableSamplePipeline = false
		return f.client.Update(f.ctx, current)
	}))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		cm := &corev1.ConfigMap{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKey{Name: "sample-config-" + f.dspa.Name, Namespace: f.namespace}, cm)) {
			return
		}
		var config struct {
			Pipelines []interface{} `json:"pipelines"`
		}
		if assert.NoError(c, json.Unmarshal([]byte(cm.Data["sample_config.json"]), &config)) {
			assert.Empty(c, config.Pipelines)
		}
	}, deadline, pollInterval)
	f.waitOperands()
	apiDeployment := &appsv1.Deployment{}
	f.get(client.ObjectKey{Name: "ds-pipeline-" + f.dspa.Name, Namespace: f.namespace}, apiDeployment)
	pods := &corev1.PodList{}
	require.NoError(t, f.client.List(f.ctx, pods, client.InNamespace(f.namespace), client.MatchingLabels(apiDeployment.Spec.Selector.MatchLabels)))
	require.NotEmpty(t, pods.Items)
	deletedPod := pods.Items[0]
	require.NoError(t, f.client.Delete(f.ctx, &deletedPod))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		current := &corev1.PodList{}
		if !assert.NoError(c, f.client.List(f.ctx, current, client.InNamespace(f.namespace), client.MatchingLabels(apiDeployment.Spec.Selector.MatchLabels))) {
			return
		}
		replacementReady := false
		for _, pod := range current.Items {
			if pod.UID == deletedPod.UID || pod.DeletionTimestamp != nil {
				continue
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					replacementReady = true
				}
			}
		}
		assert.True(c, replacementReady)
	}, deadline, pollInterval)
	f.waitOperands()
	f.apiRequest("GET", "/healthz", nil)

	t.Log("project Managed -> Removed -> Managed to shared and DSPA Argo operands")
	blockerKey := client.ObjectKey{Name: "workflow-controller-configmap", Namespace: f.applications}
	const holdFinalizer = "testing.opendatahub.io/hold-argo-cleanup"
	setHold := func(hold bool) error {
		return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			cm := &corev1.ConfigMap{}
			if err := f.client.Get(f.ctx, blockerKey, cm); err != nil {
				return client.IgnoreNotFound(err)
			}
			finalizers := []string{}
			for _, value := range cm.Finalizers {
				if value != holdFinalizer {
					finalizers = append(finalizers, value)
				}
			}
			if hold {
				finalizers = append(finalizers, holdFinalizer)
			}
			cm.Finalizers = finalizers
			return f.client.Update(f.ctx, cm)
		})
	}
	require.NoError(t, setHold(true))
	t.Cleanup(func() { assert.NoError(t, setHold(false)) })
	f.setState(common.Removed)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		cm := &corev1.ConfigMap{}
		if assert.NoError(c, f.client.Get(f.ctx, blockerKey, cm)) {
			assert.False(c, cm.DeletionTimestamp.IsZero())
		}
	}, deadline, pollInterval)
	f.waitModuleCondition("ArgoWorkflowsControllersReady", metav1.ConditionFalse, version)
	f.waitModuleCondition("ProvisioningSucceeded", metav1.ConditionFalse, version)
	require.NoError(t, setHold(false))
	f.waitModule(common.Removed, version)
	f.assertSharedAssets(true)
	argoResources := []schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Version: "v1", Kind: "Service"}, {Version: "v1", Kind: "ConfigMap"}, {Version: "v1", Kind: "ServiceAccount"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	}
	for _, gvk := range argoResources {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)
		name := "ds-pipeline-workflow-controller-" + f.dspa.Name
		if gvk.Kind == "Role" {
			name = "ds-pipeline-workflow-controller-role-" + f.dspa.Name
		}
		if gvk.Kind == "RoleBinding" {
			name = "ds-pipeline-workflow-controller-rolebinding-" + f.dspa.Name
		}
		require.Eventually(t, func() bool {
			return apierrors.IsNotFound(f.client.Get(f.ctx, client.ObjectKey{Name: name, Namespace: f.namespace}, resource))
		}, deadline, pollInterval, "remaining Argo %s", gvk.Kind)
	}
	f.get(client.ObjectKeyFromObject(run), run)
	require.Equal(t, runUID, run.GetUID())
	// Removing bundled Argo must preserve unrelated DSPA workloads and data.
	f.waitDeployment(f.namespace, "ds-pipeline-"+f.dspa.Name)
	f.apiRequest("GET", "/healthz", nil)
	f.setState(common.Managed)
	f.waitModule(common.Managed, version)
	f.waitOperands()
	f.runWorkflow("after-reconfiguration")

	t.Log("finalize the module, preserve workflow data, and recreate it")
	module := f.module()
	require.NoError(t, f.client.Delete(f.ctx, module))
	require.Eventually(t, func() bool {
		return apierrors.IsNotFound(f.client.Get(f.ctx, client.ObjectKeyFromObject(module), &aipipelinesv1alpha1.AIPipelines{}))
	}, deadline, pollInterval)
	f.assertSharedAssets(true)
	f.get(client.ObjectKeyFromObject(run), run)
	require.Equal(t, runUID, run.GetUID())
	require.NoError(t, f.client.Create(f.ctx, &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: module.Name, Labels: module.Labels}, Spec: module.Spec,
	}))
	f.waitModule(common.Managed, version)
	f.waitOperands()

	t.Log("DSPA deletion removes all owned resources through finalization and garbage collection")
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
		pods := &corev1.PodList{}
		if assert.NoError(c, f.client.List(f.ctx, pods, client.InNamespace(f.namespace), client.MatchingLabels{"component": "data-science-pipelines"})) {
			assert.Empty(c, pods.Items, "DSPA workload pods remain after cleanup")
		}
	}, deadline, pollInterval)
	f.waitModule(common.Managed, version)
}

func (f *fixture) ownedDSPAResources() []*unstructured.Unstructured {
	f.t.Helper()
	var objects []*unstructured.Unstructured
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "DeploymentList"},
		{Version: "v1", Kind: "ServiceList"}, {Version: "v1", Kind: "ConfigMapList"},
		{Version: "v1", Kind: "SecretList"}, {Version: "v1", Kind: "PersistentVolumeClaimList"}, {Version: "v1", Kind: "ServiceAccountList"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleList"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBindingList"},
		{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicyList"},
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
