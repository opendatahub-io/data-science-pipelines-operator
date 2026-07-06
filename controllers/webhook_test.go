//go:build test_all || test_unit

/*

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
	"os"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	testDSPONamespace = "testDSPONamespace"
	testWebhookName   = "ds-pipelines-webhook"
)

func TestWebhookLifecycle(t *testing.T) {
	t.Setenv("DSPO_NAMESPACE", testDSPONamespace)
	t.Run("DeployWebhook", func(t *testing.T) {
		dspa := testutil.CreateTestDSPA()

		// Create Context, Fake Controller and Params
		ctx, params, reconciler := CreateNewTestObjects()
		err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
		require.NoError(t, err)

		dspoDeployment := testutil.CreateTestDSPODeployment(params.DSPONamespace)
		err = reconciler.Client.Create(ctx, dspoDeployment)
		assert.NoError(t, err)

		// Assert Webhook Deployment doesn't yet exist
		deployment := &appsv1.Deployment{}
		created, err := reconciler.IsResourceCreated(ctx, deployment, testWebhookName, testDSPONamespace)
		assert.False(t, created)
		require.NoError(t, err)

		// Run test reconciliation
		err = reconciler.ReconcileWebhook(ctx, params)
		require.NoError(t, err)

		// Assert APIServer Deployment now exists
		deployment = &appsv1.Deployment{}
		created, err = reconciler.IsResourceCreated(ctx, deployment, testWebhookName, testDSPONamespace)
		assert.True(t, created)
		require.NoError(t, err)
	})

	t.Run("WebhookNotDeployedWithoutKubernetesPipelineStore", func(t *testing.T) {
		dspa := testutil.CreateEmptyDSPA()
		dspa.Name = "dspa-non-k8s"
		dspa.Namespace = "testnamespace"
		dspa.Spec.APIServer = &dspav1.APIServer{
			Deploy: true,
		}

		ctx, params, reconciler := CreateNewTestObjects()
		err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
		require.NoError(t, err)

		dspoDeployment := testutil.CreateTestDSPODeployment(params.DSPONamespace)
		err = reconciler.Client.Create(ctx, dspoDeployment)
		assert.NoError(t, err)

		err = reconciler.Client.Create(ctx, dspa)
		assert.NoError(t, err)

		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      dspa.Name,
				Namespace: dspa.Namespace,
			},
		})
		assert.NoError(t, err)

		// Webhook Deployment should NOT be created
		deployment := &appsv1.Deployment{}
		created, err := reconciler.IsResourceCreated(ctx, deployment, testWebhookName, testDSPONamespace)
		assert.False(t, created)
		require.NoError(t, err)
	})

	t.Run("WebhookRemovedWhenLastKubernetesDSPADeleted", func(t *testing.T) {
		dspa := testutil.CreateTestDSPA()

		ctx, params, reconciler := CreateNewTestObjects()
		err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
		require.NoError(t, err)

		dspoDeployment := testutil.CreateTestDSPODeployment(params.DSPONamespace)
		err = reconciler.Client.Create(ctx, dspoDeployment)
		assert.NoError(t, err)

		controllerutil.AddFinalizer(dspa, finalizerName)

		// Create the DSPA resource in the fake client
		err = reconciler.Client.Create(ctx, dspa)
		assert.NoError(t, err)

		// First reconcile creates the webhook
		err = reconciler.ReconcileWebhook(ctx, params)
		require.NoError(t, err)

		deployment := &appsv1.Deployment{}
		created, err := reconciler.IsResourceCreated(ctx, deployment, testWebhookName, testDSPONamespace)
		assert.True(t, created)
		require.NoError(t, err)

		// Simulate deletion of last DSPA with kubernetes storage
		err = reconciler.Client.Delete(ctx, dspa)
		assert.NoError(t, err)

		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      dspa.Name,
				Namespace: dspa.Namespace,
			},
		})
		assert.NoError(t, err)

		// Webhook should now be deleted
		deployment = &appsv1.Deployment{}
		created, err = reconciler.IsResourceCreated(ctx, deployment, testWebhookName, testDSPONamespace)
		assert.False(t, created)
		require.NoError(t, err)
	})

	t.Run("WebhookPersistsWhenNonKubernetesDSPADeleted", func(t *testing.T) {
		// First: create a DSPA with kubernetes PipelineStore to ensure webhook exists
		k8sDSPA := testutil.CreateTestDSPA()

		os.Setenv("DSPO_NAMESPACE", "testDSPONamespace")
		defer os.Unsetenv("DSPO_NAMESPACE")

		ctx, params, reconciler := CreateNewTestObjects()
		err := params.ExtractParams(ctx, k8sDSPA, reconciler.Client, reconciler.Log)
		require.NoError(t, err)

		dspoDeployment := testutil.CreateTestDSPODeployment(params.DSPONamespace)
		err = reconciler.Client.Create(ctx, dspoDeployment)
		assert.NoError(t, err)

		controllerutil.AddFinalizer(k8sDSPA, finalizerName)

		// Create the DSPA resource in the fake client
		err = reconciler.Client.Create(ctx, k8sDSPA)
		assert.NoError(t, err)

		err = reconciler.ReconcileWebhook(ctx, params)
		require.NoError(t, err)

		deployment := &appsv1.Deployment{}
		created, err := reconciler.IsResourceCreated(ctx, deployment, testWebhookName, testDSPONamespace)
		assert.True(t, created)
		require.NoError(t, err)

		//Create a DSPA with non-kubernetes PipelineStore
		nonK8sDSPA := testutil.CreateEmptyDSPA()
		nonK8sDSPA.Name = "dspa-non-k8s"
		nonK8sDSPA.Namespace = "testnamespace"
		nonK8sDSPA.Spec.APIServer = &dspav1.APIServer{
			Deploy: true,
		}

		err = reconciler.Client.Create(ctx, nonK8sDSPA)
		assert.NoError(t, err)

		// Simulate deletion of DSPA with non-kubernetes storage
		err = reconciler.Client.Delete(ctx, nonK8sDSPA)
		assert.NoError(t, err)

		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      nonK8sDSPA.Name,
				Namespace: nonK8sDSPA.Namespace,
			},
		})
		assert.NoError(t, err)

		// Webhook should still exist
		deployment = &appsv1.Deployment{}
		created, err = reconciler.IsResourceCreated(ctx, deployment, testWebhookName, testDSPONamespace)
		assert.True(t, created)
		require.NoError(t, err)
	})

	t.Run("WebhookAnnotationsAreSet", func(t *testing.T) {
		dspa := testutil.CreateTestDSPA()

		ctx, params, reconciler := CreateNewTestObjects()
		err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
		require.NoError(t, err)

		dspoDeployment := testutil.CreateTestDSPODeployment(params.DSPONamespace)
		err = reconciler.Client.Create(ctx, dspoDeployment)
		assert.NoError(t, err)

		params.WebhookAnnotations = map[string]string{
			"test.annotation/key1": "value1",
			"test.annotation/key2": "value2",
		}

		// Run test reconciliation
		err = reconciler.ReconcileWebhook(ctx, params)
		require.NoError(t, err)

		// Verify ValidatingWebhookConfiguration has annotations
		validatingWebhook := &admv1.ValidatingWebhookConfiguration{}
		err = reconciler.Client.Get(ctx, types.NamespacedName{Name: "pipelineversions.pipelines.kubeflow.org"}, validatingWebhook)
		require.NoError(t, err)
		assert.Equal(t, "value1", validatingWebhook.Annotations["test.annotation/key1"])
		assert.Equal(t, "value2", validatingWebhook.Annotations["test.annotation/key2"])

		// Verify MutatingWebhookConfiguration has annotations
		mutatingWebhook := &admv1.MutatingWebhookConfiguration{}
		err = reconciler.Client.Get(ctx, types.NamespacedName{Name: "pipelineversions.pipelines.kubeflow.org"}, mutatingWebhook)
		require.NoError(t, err)
		assert.Equal(t, "value1", mutatingWebhook.Annotations["test.annotation/key1"])
		assert.Equal(t, "value2", mutatingWebhook.Annotations["test.annotation/key2"])
	})
}
