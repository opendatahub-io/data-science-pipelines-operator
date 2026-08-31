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
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

func baseDSPA() *dspav1.DataSciencePipelinesApplication {
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer: &dspav1.APIServer{
				Deploy: true,
			},
			PersistenceAgent: &dspav1.PersistenceAgent{
				Deploy: true,
			},
			ScheduledWorkflow: &dspav1.ScheduledWorkflow{
				Deploy: true,
			},
			WorkflowController: &dspav1.WorkflowController{
				Deploy: true,
			},
			MLMD: &dspav1.MLMD{
				Deploy: true,
			},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: true,
					Image:  "someimage",
					Resources: &dspav1.ResourceRequirements{
						Requests: &dspav1.Resources{
							CPU:    resource.MustParse("250m"),
							Memory: resource.MustParse("500Mi"),
						},
						Limits: &dspav1.Resources{
							CPU:    resource.MustParse("500m"),
							Memory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"
	return dspa
}

// APIServer — thorough tests covering all template branches

func TestAPIServerResourceClaims_NoClaims(t *testing.T) {
	dspa := baseDSPA()

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	assert.Empty(t, deployment.Spec.Template.Spec.ResourceClaims)
}

func TestAPIServerResourceClaims_WithTemplateName(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.APIServer.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name:                      "gpu-claim",
			ResourceClaimTemplateName: ptr.To("gpu-claim-template"),
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-claim-template"), claims[0].ResourceClaimTemplateName)
	assert.Nil(t, claims[0].ResourceClaimName)
}

func TestAPIServerResourceClaims_WithClaimName(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.APIServer.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name:              "existing-claim",
			ResourceClaimName: ptr.To("my-pre-existing-claim"),
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "existing-claim", claims[0].Name)
	assert.Equal(t, ptr.To("my-pre-existing-claim"), claims[0].ResourceClaimName)
	assert.Nil(t, claims[0].ResourceClaimTemplateName)
}

func TestAPIServerResourceClaims_MultipleClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.APIServer.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name:                      "gpu-claim",
			ResourceClaimTemplateName: ptr.To("gpu-template"),
		},
		{
			Name:              "fpga-claim",
			ResourceClaimName: ptr.To("shared-fpga"),
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 2)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-template"), claims[0].ResourceClaimTemplateName)
	assert.Equal(t, "fpga-claim", claims[1].Name)
	assert.Equal(t, ptr.To("shared-fpga"), claims[1].ResourceClaimName)
}

// Other components — smoke tests

func TestPersistenceAgentResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.PersistenceAgent.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name:                      "gpu-claim",
			ResourceClaimTemplateName: ptr.To("gpu-template"),
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcilePersistenceAgent(dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, persistenceAgentDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-template"), claims[0].ResourceClaimTemplateName)
}

func TestScheduledWorkflowResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.ScheduledWorkflow.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name:                      "gpu-claim",
			ResourceClaimTemplateName: ptr.To("gpu-template"),
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileScheduledWorkflow(dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, scheduledWorkflowDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-template"), claims[0].ResourceClaimTemplateName)
}

func TestWorkflowControllerResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.WorkflowController.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name:                      "gpu-claim",
			ResourceClaimTemplateName: ptr.To("gpu-template"),
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	_, err = reconciler.ReconcileWorkflowController(dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, workflowControllerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-template"), claims[0].ResourceClaimTemplateName)
}

func TestMariaDBResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.Database.MariaDB.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name:                      "gpu-claim",
			ResourceClaimTemplateName: ptr.To("gpu-template"),
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileDatabase(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, "mariadb-"+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-template"), claims[0].ResourceClaimTemplateName)
}

func TestMinioResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.ObjectStorage.Minio.ResourceClaims = []corev1.PodResourceClaim{
		{
			Name:                      "gpu-claim",
			ResourceClaimTemplateName: ptr.To("gpu-template"),
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileStorage(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, "minio-"+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-template"), claims[0].ResourceClaimTemplateName)
}

func TestMLMDEnvoyResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.MLMD.Envoy = &dspav1.Envoy{
		ResourceClaims: []corev1.PodResourceClaim{
			{
				Name:                      "gpu-claim",
				ResourceClaimTemplateName: ptr.To("gpu-template"),
			},
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileMLMD(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, "ds-pipeline-metadata-envoy-"+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-template"), claims[0].ResourceClaimTemplateName)
}

func TestMLMDGRPCResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.MLMD.GRPC = &dspav1.GRPC{
		ResourceClaims: []corev1.PodResourceClaim{
			{
				Name:                      "gpu-claim",
				ResourceClaimTemplateName: ptr.To("gpu-template"),
			},
		},
	}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileMLMD(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, "ds-pipeline-metadata-grpc-"+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	claims := deployment.Spec.Template.Spec.ResourceClaims
	require.Len(t, claims, 1)
	assert.Equal(t, "gpu-claim", claims[0].Name)
	assert.Equal(t, ptr.To("gpu-template"), claims[0].ResourceClaimTemplateName)
}

// Params extraction test

func TestExtractParams_ResourceClaimsPreserved(t *testing.T) {
	dspa := baseDSPA()

	expectedClaims := []corev1.PodResourceClaim{
		{
			Name:                      "gpu-claim",
			ResourceClaimTemplateName: ptr.To("gpu-template"),
		},
		{
			Name:              "fpga-claim",
			ResourceClaimName: ptr.To("shared-fpga"),
		},
	}

	dspa.Spec.APIServer.ResourceClaims = expectedClaims
	dspa.Spec.PersistenceAgent.ResourceClaims = expectedClaims
	dspa.Spec.ScheduledWorkflow.ResourceClaims = expectedClaims
	dspa.Spec.WorkflowController.ResourceClaims = expectedClaims
	dspa.Spec.Database.MariaDB.ResourceClaims = expectedClaims
	dspa.Spec.ObjectStorage.Minio.ResourceClaims = expectedClaims
	dspa.Spec.MLMD.Envoy = &dspav1.Envoy{ResourceClaims: expectedClaims}
	dspa.Spec.MLMD.GRPC = &dspav1.GRPC{ResourceClaims: expectedClaims}

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	assert.Equal(t, expectedClaims, params.APIServer.ResourceClaims)
	assert.Equal(t, expectedClaims, params.PersistenceAgent.ResourceClaims)
	assert.Equal(t, expectedClaims, params.ScheduledWorkflow.ResourceClaims)
	assert.Equal(t, expectedClaims, params.WorkflowController.ResourceClaims)
	assert.Equal(t, expectedClaims, params.MariaDB.ResourceClaims)
	assert.Equal(t, expectedClaims, params.Minio.ResourceClaims)
	assert.Equal(t, expectedClaims, params.MLMD.Envoy.ResourceClaims)
	assert.Equal(t, expectedClaims, params.MLMD.GRPC.ResourceClaims)
}
