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
	"encoding/json"
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

func componentContainerReferencesAllPodClaims(podSpec corev1.PodSpec, containerName string) bool {
	for i := range podSpec.Containers {
		container := &podSpec.Containers[i]
		if container.Name != containerName {
			continue
		}

		referencedClaims := make(map[string]struct{}, len(container.Resources.Claims))
		for _, claim := range container.Resources.Claims {
			referencedClaims[claim.Name] = struct{}{}
		}
		for _, claim := range podSpec.ResourceClaims {
			if _, found := referencedClaims[claim.Name]; !found {
				return false
			}
		}
		return true
	}

	return false
}

func requireComponentContainerClaims(t *testing.T, deployment *appsv1.Deployment, containerName string) *corev1.Container {
	t.Helper()

	podSpec := deployment.Spec.Template.Spec
	require.True(t, componentContainerReferencesAllPodClaims(podSpec, containerName))

	var componentContainer *corev1.Container
	for i := range podSpec.Containers {
		container := &podSpec.Containers[i]
		if container.Name == containerName {
			componentContainer = container
			continue
		}
		assert.Empty(t, container.Resources.Claims, "claims must not be attached to container %q", container.Name)
	}
	require.NotNil(t, componentContainer)

	expected := make([]corev1.ResourceClaim, 0, len(podSpec.ResourceClaims))
	for _, claim := range podSpec.ResourceClaims {
		expected = append(expected, corev1.ResourceClaim{Name: claim.Name})
	}
	assert.ElementsMatch(t, expected, componentContainer.Resources.Claims)

	return componentContainer
}

func TestComponentContainerReferencesAllPodClaims(t *testing.T) {
	tests := []struct {
		name          string
		podSpec       corev1.PodSpec
		containerName string
		want          bool
	}{
		{
			name: "all claims referenced by intended container",
			podSpec: corev1.PodSpec{
				ResourceClaims: []corev1.PodResourceClaim{{Name: "gpu"}, {Name: "fpga"}},
				Containers: []corev1.Container{{
					Name: "component",
					Resources: corev1.ResourceRequirements{Claims: []corev1.ResourceClaim{
						{Name: "gpu"}, {Name: "fpga"},
					}},
				}},
			},
			containerName: "component",
			want:          true,
		},
		{
			name: "no pod claims",
			podSpec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "component"}},
			},
			containerName: "component",
			want:          true,
		},
		{
			name: "claim attached only to wrong container",
			podSpec: corev1.PodSpec{
				ResourceClaims: []corev1.PodResourceClaim{{Name: "gpu"}},
				Containers: []corev1.Container{
					{Name: "component"},
					{Name: "sidecar", Resources: corev1.ResourceRequirements{Claims: []corev1.ResourceClaim{{Name: "gpu"}}}},
				},
			},
			containerName: "component",
			want:          false,
		},
		{
			name: "one pod claim reference missing",
			podSpec: corev1.PodSpec{
				ResourceClaims: []corev1.PodResourceClaim{{Name: "gpu"}, {Name: "fpga"}},
				Containers: []corev1.Container{{
					Name:      "component",
					Resources: corev1.ResourceRequirements{Claims: []corev1.ResourceClaim{{Name: "gpu"}}},
				}},
			},
			containerName: "component",
			want:          false,
		},
		{
			name: "intended container missing",
			podSpec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "sidecar"}},
			},
			containerName: "component",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, componentContainerReferencesAllPodClaims(tt.podSpec, tt.containerName))
		})
	}
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
	requireComponentContainerClaims(t, deployment, "ds-pipeline-api-server")
}

func TestAPIServerResourceClaims_WithTemplateName(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.APIServer.ResourceClaims = []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "ds-pipeline-api-server")
}

func TestAPIServerResourceClaims_WithClaimName(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.APIServer.ResourceClaims = []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "ds-pipeline-api-server")
}

func TestAPIServerResourceClaims_MultipleClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.APIServer.ResourceClaims = []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "ds-pipeline-api-server")
}

func TestAPIServerResourceClaims_QuoteAndNewlineAreSafelyEncoded(t *testing.T) {
	dspa := baseDSPA()
	claimName := "gpu\"\nhostNetwork: true"
	resourceClaimName := "existing\"\nhostPID: true"
	dspa.Spec.APIServer.ResourceClaims = []dspav1.PodResourceClaim{
		{
			Name:              claimName,
			ResourceClaimName: ptr.To(resourceClaimName),
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

	require.Len(t, deployment.Spec.Template.Spec.ResourceClaims, 1)
	assert.Equal(t, claimName, deployment.Spec.Template.Spec.ResourceClaims[0].Name)
	assert.Equal(t, ptr.To(resourceClaimName), deployment.Spec.Template.Spec.ResourceClaims[0].ResourceClaimName)
	assert.False(t, deployment.Spec.Template.Spec.HostNetwork)
	assert.False(t, deployment.Spec.Template.Spec.HostPID)
	requireComponentContainerClaims(t, deployment, "ds-pipeline-api-server")
}

// Other components — smoke tests

func TestPersistenceAgentResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.PersistenceAgent.ResourceClaims = []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "ds-pipeline-persistenceagent")
}

func TestScheduledWorkflowResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.ScheduledWorkflow.ResourceClaims = []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "ds-pipeline-scheduledworkflow")
}

func TestWorkflowControllerResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.WorkflowController.ResourceClaims = []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "ds-pipeline-workflow-controller")
}

func TestMariaDBResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.Database.MariaDB.ResourceClaims = []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "mariadb")
}

func TestMinioResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.ObjectStorage.Minio.ResourceClaims = []dspav1.PodResourceClaim{
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
	container := requireComponentContainerClaims(t, deployment, "minio")
	assert.Equal(t, resource.MustParse("250m"), container.Resources.Requests.Cpu().DeepCopy())
	assert.Equal(t, resource.MustParse("1Gi"), container.Resources.Limits.Memory().DeepCopy())
}

func TestMLMDEnvoyResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.MLMD.Envoy = &dspav1.Envoy{
		ResourceClaims: []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "container")
}

func TestMLMDGRPCResourceClaims(t *testing.T) {
	dspa := baseDSPA()
	dspa.Spec.MLMD.GRPC = &dspav1.GRPC{
		ResourceClaims: []dspav1.PodResourceClaim{
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
	requireComponentContainerClaims(t, deployment, "container")
}

// Params extraction test

func TestExtractParams_ResourceClaimsPreserved(t *testing.T) {
	dspa := baseDSPA()

	expectedClaims := []dspav1.PodResourceClaim{
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

func TestDSPAResourceClaimsJSONRoundTrip(t *testing.T) {
	dspa := baseDSPA()
	expectedClaims := []dspav1.PodResourceClaim{
		{
			Name:              "existing-claim",
			ResourceClaimName: ptr.To("shared-gpu"),
		},
		{
			Name:                      "generated-claim",
			ResourceClaimTemplateName: ptr.To("gpu-template"),
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

	serialized, err := json.Marshal(dspa)
	require.NoError(t, err)

	decoded := &dspav1.DataSciencePipelinesApplication{}
	require.NoError(t, json.Unmarshal(serialized, decoded))

	assert.Equal(t, expectedClaims, decoded.Spec.APIServer.ResourceClaims)
	assert.Equal(t, expectedClaims, decoded.Spec.PersistenceAgent.ResourceClaims)
	assert.Equal(t, expectedClaims, decoded.Spec.ScheduledWorkflow.ResourceClaims)
	assert.Equal(t, expectedClaims, decoded.Spec.WorkflowController.ResourceClaims)
	assert.Equal(t, expectedClaims, decoded.Spec.Database.MariaDB.ResourceClaims)
	assert.Equal(t, expectedClaims, decoded.Spec.ObjectStorage.Minio.ResourceClaims)
	assert.Equal(t, expectedClaims, decoded.Spec.MLMD.Envoy.ResourceClaims)
	assert.Equal(t, expectedClaims, decoded.Spec.MLMD.GRPC.ResourceClaims)
}
