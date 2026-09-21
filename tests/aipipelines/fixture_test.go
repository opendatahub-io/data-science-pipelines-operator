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
	"context"
	"os"
	"testing"
	"time"

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	argoassets "github.com/opendatahub-io/data-science-pipelines-operator/config/argo"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fixtureLabel = "testing.opendatahub.io/aipipelines"
	configName   = "odh-aipipelines-config"
	pollInterval = 2 * time.Second
	deadline     = 8 * time.Minute
)

type fixture struct {
	t            *testing.T
	ctx          context.Context
	client       client.Client
	restConfig   *rest.Config
	applications string
	namespace    string
	dspa         *dspav1.DataSciencePipelinesApplication
}

// The singleton and its handshake must be explicitly marked as test fixtures.
// Run after the modular integration suite on the same cluster.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	require.NotEmpty(t, kubeconfig, "set KUBECONFIG to a dedicated test cluster's config file")
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	restConfig.Timeout = 30 * time.Second
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(scheme))
	require.NoError(t, dspav1.AddToScheme(scheme))
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	require.NoError(t, err)
	applications := os.Getenv("APPLICATIONS_NAMESPACE")
	if applications == "" {
		applications = "opendatahub"
	}
	f := &fixture{t: t, ctx: context.Background(), client: k8sClient, restConfig: restConfig, applications: applications}
	module := f.module()
	require.Equal(t, "true", module.Labels[fixtureLabel], "the module must be a dedicated test fixture")
	handshake := &corev1.ConfigMap{}
	f.get(client.ObjectKey{Name: configName, Namespace: applications}, handshake)
	require.Equal(t, "true", handshake.Labels[fixtureLabel])
	require.Empty(t, module.OwnerReferences, "run directly against DSPO, without a platform controller overwriting the fixture")
	f.waitModule(common.Managed, handshake.Data["platformVersion"])

	t.Cleanup(func() {
		assert.NoError(t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			current := &aipipelinesv1alpha1.AIPipelines{}
			err := f.client.Get(f.ctx, client.ObjectKeyFromObject(module), current)
			if apierrors.IsNotFound(err) {
				return f.client.Create(f.ctx, &aipipelinesv1alpha1.AIPipelines{
					ObjectMeta: metav1.ObjectMeta{Name: module.Name, Labels: module.Labels}, Spec: module.Spec,
				})
			}
			if err != nil {
				return err
			}
			current.Spec = module.Spec
			return f.client.Update(f.ctx, current)
		}))
	})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		current := &aipipelinesv1alpha1.AIPipelines{}
		if err := f.client.Get(f.ctx, client.ObjectKeyFromObject(module), current); err == nil {
			t.Logf("module status: %+v", current.Status)
		}
	})
	return f
}

func (f *fixture) attachIntegrationDSPA() {
	f.t.Helper()
	namespace := os.Getenv("DSPANAMESPACE")
	if namespace == "" {
		namespace = "test-dspa"
	}
	name := os.Getenv("AIPIPELINES_DSPA_NAME")
	if name == "" {
		name = "test-dspa"
	}
	f.namespace = namespace
	f.dspa = &dspav1.DataSciencePipelinesApplication{}
	f.get(client.ObjectKey{Namespace: namespace, Name: name}, f.dspa)
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		current := &dspav1.DataSciencePipelinesApplication{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKeyFromObject(f.dspa), current)) {
			return
		}
		found := false
		for _, condition := range current.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
				found = true
			}
		}
		assert.True(c, found, "integration DSPA %s/%s must be Ready before module lifecycle tests", namespace, name)
	}, deadline, pollInterval)
}

func (f *fixture) assertIntegrationDSPAReady() {
	f.t.Helper()
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		current := &dspav1.DataSciencePipelinesApplication{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKeyFromObject(f.dspa), current)) {
			return
		}
		found := false
		for _, condition := range current.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
				found = true
			}
		}
		assert.True(c, found, "integration DSPA %s/%s must remain Ready after module cleanup", f.namespace, f.dspa.Name)
	}, deadline, pollInterval)
}

func (f *fixture) get(key client.ObjectKey, object client.Object) {
	f.t.Helper()
	require.NoError(f.t, f.client.Get(f.ctx, key, object))
}

func (f *fixture) module() *aipipelinesv1alpha1.AIPipelines {
	f.t.Helper()
	module := &aipipelinesv1alpha1.AIPipelines{}
	f.get(client.ObjectKey{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}, module)
	return module
}

func (f *fixture) waitModule(state common.ManagementState, version string) {
	f.t.Helper()
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		module := &aipipelinesv1alpha1.AIPipelines{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKey{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}, module)) {
			return
		}
		var contract common.PlatformObject = module
		assert.Equal(c, state, module.Spec.ArgoWorkflowsControllersManagementState())
		assert.Equal(c, module.Generation, contract.GetStatus().ObservedGeneration)
		assert.Equal(c, common.PhaseReady, contract.GetStatus().Phase)
		platformRelease := contract.GetReleaseStatus().GetRelease(common.ReleasePlatform)
		if assert.NotNil(c, platformRelease, "missing status.releases[name=%s]", common.ReleasePlatform) {
			assert.Equal(c, version, platformRelease.Version)
		}
		for _, kind := range []string{"Ready", "ProvisioningSucceeded", "DSPOReady", "ArgoWorkflowsControllersReady"} {
			found := false
			for _, condition := range contract.GetConditions() {
				if condition.Type == kind {
					found = true
					assert.Equal(c, metav1.ConditionTrue, condition.Status, "%s: %s", kind, condition.Message)
					assert.Equal(c, module.Generation, condition.ObservedGeneration)
				}
			}
			assert.True(c, found, "missing %s", kind)
		}
	}, deadline, pollInterval)
}

func (f *fixture) setState(state common.ManagementState) {
	f.t.Helper()
	require.NoError(f.t, retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		module := &aipipelinesv1alpha1.AIPipelines{}
		if err := f.client.Get(f.ctx, client.ObjectKey{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}, module); err != nil {
			return err
		}
		module.Spec.ArgoWorkflowsControllers = &aipipelinesv1alpha1.ArgoWorkflowsControllersSpec{ManagementState: state}
		return f.client.Update(f.ctx, module)
	}))
}

func (f *fixture) waitModuleCondition(kind string, status metav1.ConditionStatus, release string) {
	f.t.Helper()
	require.EventuallyWithT(f.t, func(c *assert.CollectT) {
		module := &aipipelinesv1alpha1.AIPipelines{}
		if !assert.NoError(c, f.client.Get(f.ctx, client.ObjectKey{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}, module)) {
			return
		}
		assert.Equal(c, module.Generation, module.Status.ObservedGeneration)
		assert.Equal(c, release, module.Status.GetPlatformRelease())
		found := false
		for _, condition := range module.Status.Conditions {
			if condition.Type == kind {
				found = true
				assert.Equal(c, status, condition.Status, condition.Message)
				assert.Equal(c, module.Generation, condition.ObservedGeneration)
			}
		}
		assert.True(c, found, "missing %s", kind)
	}, deadline, pollInterval)
}

func (f *fixture) assertSharedAssets(removed bool) {
	f.t.Helper()
	assets, err := argoassets.Objects(f.applications)
	require.NoError(f.t, err)
	module := &aipipelinesv1alpha1.AIPipelines{}
	if !removed {
		module = f.module()
	}
	for _, asset := range assets {
		current := asset.DeepCopy()
		err := f.client.Get(f.ctx, client.ObjectKeyFromObject(asset), current)
		if removed && asset.GetKind() != "CustomResourceDefinition" {
			require.True(f.t, apierrors.IsNotFound(err), "%s/%s: %v", asset.GetKind(), asset.GetName(), err)
			continue
		}
		require.NoError(f.t, err)
		if asset.GetKind() == "CustomResourceDefinition" {
			require.Empty(f.t, current.GetOwnerReferences())
		} else {
			require.True(f.t, metav1.IsControlledBy(current, module))
		}
	}
}
