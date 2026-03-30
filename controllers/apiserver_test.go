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
	"context"
	"encoding/json"
	"testing"

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	initManagedPipelinesContainerName = "init-managed-pipelines"
	dsPipelineAPIServerContainerName  = "ds-pipeline-api-server"
)

// getInitManagedPipelinesContainer returns the init-managed-pipelines container from the deployment, or nil if not found.
func getInitManagedPipelinesContainer(deployment *appsv1.Deployment) *corev1.Container {
	for i := range deployment.Spec.Template.Spec.InitContainers {
		if deployment.Spec.Template.Spec.InitContainers[i].Name == initManagedPipelinesContainerName {
			return &deployment.Spec.Template.Spec.InitContainers[i]
		}
	}
	return nil
}

// getDSPipelineAPIServerContainer returns the ds-pipeline-api-server container from the deployment, or nil if not found.
func getDSPipelineAPIServerContainer(deployment *appsv1.Deployment) *corev1.Container {
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == dsPipelineAPIServerContainerName {
			return &deployment.Spec.Template.Spec.Containers[i]
		}
	}
	return nil
}

// getEnvValue returns the value of the named env var in the container, or "" if not found.
func getEnvValue(c *corev1.Container, name string) string {
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func TestDeployAPIServer(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with deployed APIServer
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer: &dspav1.APIServer{
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
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert APIServer Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert APIServer Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Ensure readiness is handled
	apiServerReady, err := reconciler.evaluateCondition(ctx, dspa, params.APIServerDefaultResourceName, config.APIServerReady)
	assert.Equal(t, "Deploying", apiServerReady.Reason)
	assert.Nil(t, err)
}

func TestDontDeployAPIServer(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with non-deployed APIServer
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			APIServer: &dspav1.APIServer{
				Deploy: false,
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()

	// Ensure APIServer Deployment doesn't yet exist
	created, err := reconciler.IsResourceCreated(ctx, dspa, testDSPAName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	assert.Nil(t, err)

	// Ensure APIServer Deployment still doesn't exist
	created, err = reconciler.IsResourceCreated(ctx, dspa, expectedAPIServerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}

func TestDeployAPIServerWithManagedPipelines(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	dspa := testutil.CreateDSPAWithManagedPipelines(
		"quay.io/opendatahub/odh-pipelines-components:latest",
		nil,
		nil,
	)
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	assert.Nil(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	require.True(t, created)
	require.NoError(t, err)

	// Init container image matches spec
	initC := getInitManagedPipelinesContainer(deployment)
	require.NotNil(t, initC, "init-managed-pipelines container should exist")
	assert.Equal(t, "quay.io/opendatahub/odh-pipelines-components:latest", initC.Image)

	// ALL_PIPELINES env when pipeline list omitted
	assert.Equal(t, "true", getEnvValue(initC, "ALL_PIPELINES"))
	assert.Empty(t, getEnvValue(initC, "PIPELINE_NAMES"))
	wantTags := config.BuildManagedPipelinesUploadTags(config.ResolvedPlatformVersion())
	assert.Equal(t, wantTags, getEnvValue(initC, "MANAGED_PIPELINES_UPLOAD_TAGS"))

	apiC := getDSPipelineAPIServerContainer(deployment)
	require.NotNil(t, apiC, "ds-pipeline-api-server container should exist")
	assert.Equal(t, wantTags, getEnvValue(apiC, "MANAGED_PIPELINES_UPLOAD_TAGS"))

	// Init container gets default requests/limits when CR omits resources
	require.NotNil(t, initC.Resources.Requests)
	assert.True(t, initC.Resources.Requests.Cpu().Equal(resource.MustParse("250m")))
	assert.True(t, initC.Resources.Requests.Memory().Equal(resource.MustParse("500Mi")))
	require.NotNil(t, initC.Resources.Limits)
	assert.True(t, initC.Resources.Limits.Cpu().Equal(resource.MustParse("500m")))
	assert.True(t, initC.Resources.Limits.Memory().Equal(resource.MustParse("1Gi")))

	// Verify managed pipelines fields are preserved on DSPA
	assert.NotNil(t, dspa.Spec.APIServer.ManagedPipelines)
	assert.Equal(t, "quay.io/opendatahub/odh-pipelines-components:latest", dspa.Spec.APIServer.ManagedPipelines.Image)
	assert.Nil(t, dspa.Spec.APIServer.ManagedPipelines.Pipelines)
	assert.Nil(t, dspa.Spec.APIServer.ManagedPipelines.Resources)
}

func TestDeployAPIServerWithManagedPipelinesAndPipelineList(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	pipelines := []dspav1.ManagedPipeline{
		{Name: "trainer-ostf"},
		{Name: "lm-eval"},
	}
	resources := &dspav1.ResourceRequirements{
		Requests: &dspav1.Resources{
			CPU:    resource.MustParse("250m"),
			Memory: resource.MustParse("512Mi"),
		},
	}

	dspa := testutil.CreateDSPAWithManagedPipelines(
		"quay.io/opendatahub/odh-pipelines-components:latest",
		pipelines,
		resources,
	)
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	assert.Nil(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	require.True(t, created)
	require.NoError(t, err)

	// Init container image matches spec
	initC := getInitManagedPipelinesContainer(deployment)
	require.NotNil(t, initC, "init-managed-pipelines container should exist")
	assert.Equal(t, "quay.io/opendatahub/odh-pipelines-components:latest", initC.Image)

	// PIPELINE_NAMES env for explicit pipeline list
	assert.Equal(t, "trainer-ostf,lm-eval", getEnvValue(initC, "PIPELINE_NAMES"))
	assert.Empty(t, getEnvValue(initC, "ALL_PIPELINES"))
	wantTags := config.BuildManagedPipelinesUploadTags(config.ResolvedPlatformVersion())
	assert.Equal(t, wantTags, getEnvValue(initC, "MANAGED_PIPELINES_UPLOAD_TAGS"))

	apiC := getDSPipelineAPIServerContainer(deployment)
	require.NotNil(t, apiC, "ds-pipeline-api-server container should exist")
	assert.Equal(t, wantTags, getEnvValue(apiC, "MANAGED_PIPELINES_UPLOAD_TAGS"))

	// Init container resources match CR requests; missing limits filled from defaults
	require.NotNil(t, initC.Resources.Requests)
	assert.True(t, initC.Resources.Requests.Cpu().Equal(resource.MustParse("250m")))
	assert.True(t, initC.Resources.Requests.Memory().Equal(resource.MustParse("512Mi")))
	require.NotNil(t, initC.Resources.Limits)
	assert.True(t, initC.Resources.Limits.Cpu().Equal(resource.MustParse("500m")))
	assert.True(t, initC.Resources.Limits.Memory().Equal(resource.MustParse("1Gi")))

	// Verify managed pipelines fields on DSPA
	mp := dspa.Spec.APIServer.ManagedPipelines
	assert.NotNil(t, mp)
	assert.Equal(t, "quay.io/opendatahub/odh-pipelines-components:latest", mp.Image)
	assert.Len(t, mp.Pipelines, 2)
	assert.Equal(t, "trainer-ostf", mp.Pipelines[0].Name)
	assert.Equal(t, "lm-eval", mp.Pipelines[1].Name)
	assert.NotNil(t, mp.Resources)
	assert.Equal(t, resource.MustParse("250m"), mp.Resources.Requests.CPU)
	assert.Equal(t, resource.MustParse("512Mi"), mp.Resources.Requests.Memory)
}

func TestDeployAPIServerWithoutManagedPipelines(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer: &dspav1.APIServer{
				Deploy: true,
			},
			MLMD: &dspav1.MLMD{Deploy: true},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB:            &dspav1.MariaDB{Deploy: true},
			},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio:              &dspav1.Minio{Deploy: false, Image: "someimage"},
			},
		},
	}
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	assert.Nil(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	require.True(t, created)
	require.NoError(t, err)

	// No init container when managedPipelines is nil
	initC := getInitManagedPipelinesContainer(deployment)
	assert.Nil(t, initC, "init-managed-pipelines container must not exist when ManagedPipelines is nil")

	apiC := getDSPipelineAPIServerContainer(deployment)
	require.NotNil(t, apiC)
	assert.Empty(t, getEnvValue(apiC, "MANAGED_PIPELINES_UPLOAD_TAGS"))

	// Verify managed pipelines is nil (backward compatibility)
	assert.Nil(t, dspa.Spec.APIServer.ManagedPipelines)
}

func TestApiServerEndpoints(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with deployed APIServer
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer: &dspav1.APIServer{
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
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert APIServer Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	assert.Nil(t, err)

	dspa_created := &dspav1.DataSciencePipelinesApplication{}
	created, err = reconciler.IsResourceCreated(ctx, dspa, testDSPAName, testNamespace)
	assert.NotNil(t, dspa_created.Status.Components.APIServer.Url)
	assert.NotNil(t, dspa_created.Status.Components.APIServer.ExternalUrl)
}

// Sample config tests

func TestGetSampleConfig_ExplicitList_AddsPipelinesToJSON(t *testing.T) {
	_, _, reconciler := CreateNewTestObjects()
	dspa := testutil.CreateDSPAWithManagedPipelines("img", []dspav1.ManagedPipeline{{Name: "p1"}}, nil)
	dspa.Spec.APIServer.EnableSamplePipeline = false

	jsonStr, err := reconciler.generateSampleConfigJSON(dspa, config.ResolvedPlatformVersion())
	require.NoError(t, err)

	var out struct {
		Pipelines []map[string]string `json:"pipelines"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &out))
	require.Len(t, out.Pipelines, 1)
	assert.Equal(t, "p1", out.Pipelines[0]["name"])
	assert.Equal(t, "/config/managed-pipelines/p1.yaml", out.Pipelines[0]["file"])
}

func TestGetSampleConfig_OmittedList_NoManagedEntriesInJSON(t *testing.T) {
	_, _, reconciler := CreateNewTestObjects()
	// Set minimal config for Iris so EnableSamplePipeline works
	viper.Set("ManagedPipelinesMetadata.iris.Name", "[Demo] iris")
	viper.Set("ManagedPipelinesMetadata.iris.Filepath", "/samples/iris.yaml")
	viper.Set("DSPO.PlatformVersion", "v1")
	t.Cleanup(func() { viper.Reset() })

	dspa := testutil.CreateDSPAWithManagedPipelines("img", nil, nil) // omitted list = "all"
	dspa.Spec.APIServer.EnableSamplePipeline = true

	jsonStr, err := reconciler.generateSampleConfigJSON(dspa, config.ResolvedPlatformVersion())
	require.NoError(t, err)

	var out struct {
		Pipelines []map[string]string `json:"pipelines"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &out))
	// Only Iris, no managed pipeline entries
	require.Len(t, out.Pipelines, 1)
	assert.Equal(t, "[Demo] iris", out.Pipelines[0]["name"])
}

func TestGetSampleConfig_NoManagedPipelines_IrisOnly(t *testing.T) {
	viper.Set("ManagedPipelinesMetadata.iris.Name", "[Demo] iris")
	viper.Set("ManagedPipelinesMetadata.iris.Filepath", "/samples/iris.yaml")
	viper.Set("DSPO.PlatformVersion", "v1")
	t.Cleanup(func() { viper.Reset() })

	dspa := testutil.CreateEmptyDSPA()
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true, EnableSamplePipeline: true}
	dspa.Spec.APIServer.ManagedPipelines = nil

	_, _, reconciler := CreateNewTestObjects()
	jsonStr, err := reconciler.generateSampleConfigJSON(dspa, config.ResolvedPlatformVersion())
	require.NoError(t, err)

	var out struct {
		Pipelines []map[string]string `json:"pipelines"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &out))
	require.Len(t, out.Pipelines, 1)
	assert.Equal(t, "[Demo] iris", out.Pipelines[0]["name"])
}

func TestGetSampleConfig_ConfigVsMinimalMetadata(t *testing.T) {
	// When config has ManagedPipelinesMetadata.foo, entry uses config
	viper.Set("ManagedPipelinesMetadata.foo.Name", "Display Foo")
	viper.Set("ManagedPipelinesMetadata.foo.Filepath", "/custom/foo.yaml")
	viper.Set("ManagedPipelinesMetadata.foo.Description", "From config")
	viper.Set("DSPO.PlatformVersion", "v1")
	t.Cleanup(func() { viper.Reset() })

	_, _, reconciler := CreateNewTestObjects()
	dspa := testutil.CreateDSPAWithManagedPipelines("img", []dspav1.ManagedPipeline{{Name: "foo"}}, nil)
	dspa.Spec.APIServer.EnableSamplePipeline = false

	jsonStr, err := reconciler.generateSampleConfigJSON(dspa, config.ResolvedPlatformVersion())
	require.NoError(t, err)
	var out struct {
		Pipelines []map[string]string `json:"pipelines"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &out))
	require.Len(t, out.Pipelines, 1)
	assert.Equal(t, "Display Foo", out.Pipelines[0]["name"])
	assert.Equal(t, "/custom/foo.yaml", out.Pipelines[0]["file"])
	assert.Equal(t, "From config", out.Pipelines[0]["description"])

	// When config does not have "bar", minimal entry is used
	viper.Reset()
	viper.Set("DSPO.PlatformVersion", "v1")

	dspa2 := testutil.CreateDSPAWithManagedPipelines("img", []dspav1.ManagedPipeline{{Name: "bar"}}, nil)
	jsonStr2, err := reconciler.generateSampleConfigJSON(dspa2, config.ResolvedPlatformVersion())
	require.NoError(t, err)
	var out2 struct {
		Pipelines []map[string]string `json:"pipelines"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonStr2), &out2))
	require.Len(t, out2.Pipelines, 1)
	assert.Equal(t, "bar", out2.Pipelines[0]["name"])
	assert.Equal(t, "/config/managed-pipelines/bar.yaml", out2.Pipelines[0]["file"])
}

// Config hash unit tests

func TestReconcileAPIServer_ConfigHashIncludesManagedPipelinesSpec(t *testing.T) {
	_, _, reconciler := CreateNewTestObjects()
	ctx := context.Background()

	dspa1 := testutil.CreateDSPAWithManagedPipelines("img1", nil, nil)
	dspa1.Name = "dspa"
	dspa1.Namespace = "ns"
	params1 := &DSPAParams{}
	require.NoError(t, params1.ExtractParams(ctx, dspa1, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa1, params1))

	dspa2 := testutil.CreateDSPAWithManagedPipelines("img2", nil, nil)
	dspa2.Name = "dspa"
	dspa2.Namespace = "ns"
	params2 := &DSPAParams{}
	require.NoError(t, params2.ExtractParams(ctx, dspa2, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa2, params2))

	assert.NotEqual(t, params1.APIServerConfigHash, params2.APIServerConfigHash, "hash should differ when managedPipelines spec differs")
}

func TestReconcileAPIServer_ConfigHashWhenManagedPipelinesNil(t *testing.T) {
	viper.Set("ManagedPipelinesMetadata.iris.Name", "Iris")
	viper.Set("ManagedPipelinesMetadata.iris.Filepath", "/samples/iris.yaml")
	viper.Set("DSPO.PlatformVersion", "v1")
	t.Cleanup(func() { viper.Reset() })

	dspa := testutil.CreateEmptyDSPA()
	dspa.Name = "dspa"
	dspa.Namespace = "ns"
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true, EnableSamplePipeline: true}
	dspa.Spec.APIServer.ManagedPipelines = nil

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

	assert.NotEmpty(t, params.APIServerConfigHash)
}

func TestReconcileAPIServer_ConfigHashChangesWhenPlatformVersionChanges(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	ctx := context.Background()

	dspa := testutil.CreateEmptyDSPA()
	dspa.Name = "dspa"
	dspa.Namespace = "ns"
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true, EnableSamplePipeline: false}
	dspa.Spec.APIServer.ManagedPipelines = nil

	_, _, reconciler := CreateNewTestObjects()

	viper.Set("DSPO.PlatformVersion", "v1.0")
	params1 := &DSPAParams{}
	require.NoError(t, params1.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params1))

	viper.Set("DSPO.PlatformVersion", "v2.0")
	params2 := &DSPAParams{}
	require.NoError(t, params2.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params2))

	assert.NotEqual(t, params1.APIServerConfigHash, params2.APIServerConfigHash,
		"hash should differ when platform version changes")
}

func TestExtractParams_ManagedPipelinesImageFromSpec(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })

	dspa := testutil.CreateDSPAWithManagedPipelines("my-img:tag", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))

	require.NotNil(t, params.APIServer.ManagedPipelines)
	assert.Equal(t, "my-img:tag", params.APIServer.ManagedPipelines.Image)
	require.NotNil(t, params.APIServer.ManagedPipelines.Resources)
	assert.Equal(t, resource.MustParse("250m"), params.APIServer.ManagedPipelines.Resources.Requests.CPU)
	assert.Equal(t, resource.MustParse("500m"), params.APIServer.ManagedPipelines.Resources.Limits.CPU)
}

func TestExtractParams_ManagedPipelinesVolumeSizeLimit(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	ctx := context.Background()

	tests := []struct {
		name       string
		volumeSpec string
		want       string
		wantErr    bool
	}{
		{name: "default when omitted", volumeSpec: "", want: config.DefaultManagedPipelinesVolumeSizeLimit},
		{name: "explicit value", volumeSpec: "512Mi", want: "512Mi"},
		{name: "invalid quantity", volumeSpec: "not-a-quantity", wantErr: true},
		{name: "zero quantity", volumeSpec: "0", wantErr: true},
		{name: "negative quantity", volumeSpec: "-1Mi", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", nil, nil)
			dspa.Name = "dspa"
			dspa.Namespace = "ns"
			dspa.Spec.APIServer.ManagedPipelines.VolumeSizeLimit = tt.volumeSpec

			_, params, reconciler := CreateNewTestObjects()
			err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "volumeSizeLimit")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, params.APIServer.ManagedPipelines.VolumeSizeLimit)
		})
	}
}

func TestExtractParams_ManagedPipelinesResourcesMergePartialRequests(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })

	resources := &dspav1.ResourceRequirements{
		Requests: &dspav1.Resources{
			CPU:    resource.MustParse("100m"),
			Memory: resource.MustParse("128Mi"),
		},
	}
	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", nil, resources)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))

	mp := params.APIServer.ManagedPipelines
	require.NotNil(t, mp.Resources)
	assert.Equal(t, resource.MustParse("100m"), mp.Resources.Requests.CPU)
	assert.Equal(t, resource.MustParse("128Mi"), mp.Resources.Requests.Memory)
	require.NotNil(t, mp.Resources.Limits)
	assert.Equal(t, resource.MustParse("500m"), mp.Resources.Limits.CPU)
	assert.Equal(t, resource.MustParse("1Gi"), mp.Resources.Limits.Memory)
}
