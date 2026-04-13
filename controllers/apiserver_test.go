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
	"errors"
	"os"
	"strings"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	initManagedPipelinesContainerName = "init-managed-pipelines"
	dsPipelineAPIServerContainerName  = "ds-pipeline-api-server"
)

func requireAPIServerReadyCondition(t *testing.T, dspa *dspav1.DataSciencePipelinesApplication) *metav1.Condition {
	t.Helper()
	for i := range dspa.Status.Conditions {
		if dspa.Status.Conditions[i].Type == config.APIServerReady {
			return &dspa.Status.Conditions[i]
		}
	}
	t.Fatalf("expected APIServerReady condition in status")
	return nil
}

// getInitManagedPipelinesContainer returns the init-managed-pipelines container from the deployment, or nil if not found.
func getInitManagedPipelinesContainer(t testing.TB, deployment *appsv1.Deployment) *corev1.Container {
	t.Helper()
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

// getEnvValue returns the value of the named env var and whether it is present.
func getEnvValue(t testing.TB, c *corev1.Container, name string) (string, bool) {
	t.Helper()
	if c == nil {
		return "", false
	}
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

func clearRelatedImageEnv(t *testing.T) {
	t.Helper()
	for _, raw := range os.Environ() {
		key, value, found := strings.Cut(raw, "=")
		if !found {
			continue
		}
		if strings.HasPrefix(key, relatedImageEnvPrefix) {
			os.Unsetenv(key)
			t.Cleanup(func() { os.Setenv(key, value) })
		}
	}
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
	initC := getInitManagedPipelinesContainer(t, deployment)
	require.NotNil(t, initC, "init-managed-pipelines container should exist")
	assert.Equal(t, "quay.io/opendatahub/odh-pipelines-components:latest", initC.Image)

	// ALL_PIPELINES env when pipeline list omitted
	value, found := getEnvValue(t, initC, "ALL_PIPELINES")
	require.True(t, found)
	assert.Equal(t, "true", value)
	_, found = getEnvValue(t, initC, "PIPELINE_NAMES")
	assert.False(t, found)
	wantTags := config.BuildManagedPipelinesUploadTags(config.ResolvedPlatformVersion())
	value, found = getEnvValue(t, initC, "MANAGED_PIPELINES_UPLOAD_TAGS")
	require.True(t, found)
	assert.Equal(t, wantTags, value)

	apiC := getDSPipelineAPIServerContainer(deployment)
	require.NotNil(t, apiC, "ds-pipeline-api-server container should exist")
	value, found = getEnvValue(t, apiC, "MANAGED_PIPELINES_UPLOAD_TAGS")
	require.True(t, found)
	assert.Equal(t, wantTags, value)
	assert.Contains(t, apiC.Args, "--managedPipelinesDir=/config/managed-pipelines")

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
	initC := getInitManagedPipelinesContainer(t, deployment)
	require.NotNil(t, initC, "init-managed-pipelines container should exist")
	assert.Equal(t, "quay.io/opendatahub/odh-pipelines-components:latest", initC.Image)

	// PIPELINE_NAMES env for explicit pipeline list
	value, found := getEnvValue(t, initC, "PIPELINE_NAMES")
	require.True(t, found)
	assert.Equal(t, "trainer-ostf,lm-eval", value)
	_, found = getEnvValue(t, initC, "ALL_PIPELINES")
	assert.False(t, found)
	wantTags := config.BuildManagedPipelinesUploadTags(config.ResolvedPlatformVersion())
	value, found = getEnvValue(t, initC, "MANAGED_PIPELINES_UPLOAD_TAGS")
	require.True(t, found)
	assert.Equal(t, wantTags, value)

	apiC := getDSPipelineAPIServerContainer(deployment)
	require.NotNil(t, apiC, "ds-pipeline-api-server container should exist")
	value, found = getEnvValue(t, apiC, "MANAGED_PIPELINES_UPLOAD_TAGS")
	require.True(t, found)
	assert.Equal(t, wantTags, value)

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
	initC := getInitManagedPipelinesContainer(t, deployment)
	assert.Nil(t, initC, "init-managed-pipelines container must not exist when ManagedPipelines is nil")

	apiC := getDSPipelineAPIServerContainer(deployment)
	require.NotNil(t, apiC)
	_, found := getEnvValue(t, apiC, "MANAGED_PIPELINES_UPLOAD_TAGS")
	assert.False(t, found)
	assert.NotContains(t, apiC.Args, "--managedPipelinesDir=/config/managed-pipelines")

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
	viper.Set("Images.PipelinesComponents", "quay.io/opendatahub/odh-pipelines-components:from-config-only")

	dspa := testutil.CreateDSPAWithManagedPipelines("my-img:tag", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))

	require.NotNil(t, params.APIServer.ManagedPipelines)
	assert.Equal(t, "my-img:tag", params.APIServer.ManagedPipelines.Image, "explicit CR image must override operator config")
	require.NotNil(t, params.APIServer.ManagedPipelines.Resources)
	assert.Equal(t, resource.MustParse("250m"), params.APIServer.ManagedPipelines.Resources.Requests.CPU)
	assert.Equal(t, resource.MustParse("500m"), params.APIServer.ManagedPipelines.Resources.Limits.CPU)
}

func TestExtractParams_ManagedPipelinesImageDefaultsFromOperatorConfig(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	want := "quay.io/opendatahub/odh-pipelines-components:odh-stable"
	viper.Set("Images.PipelinesComponents", want)

	dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))

	require.NotNil(t, params.APIServer.ManagedPipelines)
	assert.Equal(t, want, params.APIServer.ManagedPipelines.Image)
}

func TestExtractParams_ManagedPipelinesFailsWhenEnvVarEmpty(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	// Simulates IMAGES_PIPELINES_COMPONENTS="" with AllowEmptyEnv: key is set but value is empty, so we must error.
	viper.Set("Images.PipelinesComponents", "")

	dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrManagedPipelinesImageUnset))
	require.Contains(t, err.Error(), "IMAGES_PIPELINES_COMPONENTS")
}

func TestExtractParams_ManagedPipelinesFailsWhenImageUnsetAndNotInOperatorConfig(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })

	dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrManagedPipelinesImageUnset))
	require.Contains(t, err.Error(), "managedPipelines")
	require.Contains(t, err.Error(), "Images.PipelinesComponents")
	require.Contains(t, err.Error(), "IMAGES_PIPELINES_COMPONENTS")
}

func TestExtractParams_ManagedPipelinesWhitespaceImageTreatedAsOmitted(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	want := "quay.io/opendatahub/odh-pipelines-components:odh-stable"
	viper.Set("Images.PipelinesComponents", want)

	dspa := testutil.CreateDSPAWithManagedPipelines("  \t  ", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.APIServer.ManagedPipelines)
	assert.Equal(t, want, params.APIServer.ManagedPipelines.Image)
}

func TestExtractParams_ManagedPipelinesWhitespaceOnlyFailsWithoutOperatorConfig(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })

	dspa := testutil.CreateDSPAWithManagedPipelines("   ", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	_, params, reconciler := CreateNewTestObjects()
	ctx := context.Background()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrManagedPipelinesImageUnset))
	require.Contains(t, err.Error(), "managedPipelines")
	require.Contains(t, err.Error(), "IMAGES_PIPELINES_COMPONENTS")
}

func TestReconcile_SetsAPIServerNotReadyOnExtractParamsErrors(t *testing.T) {
	tests := []struct {
		name            string
		prepareDSPA     func(t *testing.T) *dspav1.DataSciencePipelinesApplication
		wantMsgContains []string
	}{
		{
			name: "managed_pipelines_image_unset",
			prepareDSPA: func(_ *testing.T) *dspav1.DataSciencePipelinesApplication {
				d := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
				d.Name = "dspa-mp-status"
				d.Namespace = "testnamespace"
				return d
			},
			wantMsgContains: []string{"managedPipelines", "Images.PipelinesComponents", "IMAGES_PIPELINES_COMPONENTS"},
		},
		{
			name: "invalid_related_image_env_var_name",
			prepareDSPA: func(t *testing.T) *dspav1.DataSciencePipelinesApplication {
				clearRelatedImageEnv(t)
				t.Setenv("RELATED_IMAGE_toolbox", "registry.example/x:latest")
				d := testutil.CreateDSPAWithManagedPipelines("img:latest", nil, nil)
				d.Name = "dspa-extract-bad-related"
				d.Namespace = "testnamespace"
				return d
			},
			wantMsgContains: []string{"invalid RELATED_IMAGE_* env var name"},
		},
		{
			name: "invalid_managed_pipelines_volume_size_limit",
			prepareDSPA: func(_ *testing.T) *dspav1.DataSciencePipelinesApplication {
				d := testutil.CreateDSPAWithManagedPipelines("img:latest", nil, nil)
				d.Spec.APIServer.ManagedPipelines.VolumeSizeLimit = "not-a-quantity"
				d.Name = "dspa-extract-volume"
				d.Namespace = "testnamespace"
				return d
			},
			wantMsgContains: []string{"managedPipelines.volumeSizeLimit must be a valid Kubernetes quantity"},
		},
		{
			name: "custom_kfp_launcher_configmap_not_found",
			prepareDSPA: func(_ *testing.T) *dspav1.DataSciencePipelinesApplication {
				d := testutil.CreateDSPAWithCustomKfpLauncherConfigMap("missing-launcher-cm")
				d.Name = "dspa-extract-launcher-cm"
				d.Namespace = "testnamespace"
				return d
			},
			wantMsgContains: []string{"not found"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(viper.Reset)
			viper.Reset()

			dspa := tt.prepareDSPA(t)
			ctx, _, reconciler := CreateNewTestObjects()
			require.NoError(t, reconciler.Client.Create(ctx, dspa))

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: dspa.Name, Namespace: dspa.Namespace},
			})
			require.NoError(t, err)
			assert.True(t, result.Requeue, "should requeue when ExtractParams fails")
			assert.NotZero(t, result.RequeueAfter, "RequeueAfter should be set")

			updated := &dspav1.DataSciencePipelinesApplication{}
			require.NoError(t, reconciler.Get(ctx, types.NamespacedName{Name: dspa.Name, Namespace: dspa.Namespace}, updated))

			apiCond := requireAPIServerReadyCondition(t, updated)
			assert.Equal(t, metav1.ConditionFalse, apiCond.Status)
			assert.Equal(t, config.FailingToDeploy, apiCond.Reason)
			for _, sub := range tt.wantMsgContains {
				assert.Contains(t, apiCond.Message, sub)
			}
		})
	}
}

func TestDeployAPIServerWithManagedPipelines_OmittedImageUsesOperatorConfigDefault(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	want := "quay.io/opendatahub/odh-pipelines-components:odh-stable"
	viper.Set("Images.PipelinesComponents", want)

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	require.True(t, created)
	require.NoError(t, err)

	initC := getInitManagedPipelinesContainer(t, deployment)
	require.NotNil(t, initC)
	assert.Equal(t, want, initC.Image)
}

func TestReconcileAPIServer_ConfigHashChangesWhenDefaultPipelinesComponentsImageChanges(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	ctx := context.Background()

	dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"
	dspa.Spec.APIServer.Deploy = true
	dspa.Spec.APIServer.EnableSamplePipeline = false

	_, _, reconciler := CreateNewTestObjects()

	viper.Set("Images.PipelinesComponents", "quay.io/opendatahub/odh-pipelines-components:tag-a")
	params1 := &DSPAParams{}
	require.NoError(t, params1.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params1))

	viper.Set("Images.PipelinesComponents", "quay.io/opendatahub/odh-pipelines-components:tag-b")
	params2 := &DSPAParams{}
	require.NoError(t, params2.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params2))

	assert.NotEqual(t, params1.APIServerConfigHash, params2.APIServerConfigHash,
		"hash should differ when operator default pipelines-components image changes (CR omits image)")
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

// TestAPIServerDeploymentTemplate_ManagedPipelineImageEnvValuesAreYAMLStrings ensures forwarded
// operator env values are emitted as YAML string scalars. Unquoted values like true/null/123 are
// parsed as non-strings and can break manifest apply / OpenAPI validation.
func TestAPIServerDeploymentTemplate_ManagedPipelineImageEnvValuesAreYAMLStrings(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)
	t.Setenv("RELATED_IMAGE_BOOLISH", "true")
	t.Setenv("RELATED_IMAGE_NULLISH", "null")
	t.Setenv("RELATED_IMAGE_NUMERIC", "123")

	ctx, params, reconciler := CreateNewTestObjects()
	dspa := testutil.CreateDSPAWithManagedPipelines(
		"quay.io/opendatahub/pipelines-components:latest",
		nil,
		nil,
	)
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))

	src, err := config.PathTemplateSource(reconciler.TemplatesPath+"apiserver/default/deployment.yaml.tmpl", params)
	require.NoError(t, err)
	objs, err := src.Parse()
	require.NoError(t, err)
	require.Len(t, objs, 1)

	initContainers, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "template", "spec", "initContainers")
	require.NoError(t, err)
	require.True(t, found)

	var initC map[string]interface{}
	for _, ic := range initContainers {
		m, ok := ic.(map[string]interface{})
		require.True(t, ok)
		if m["name"] == initManagedPipelinesContainerName {
			initC = m
			break
		}
	}
	require.NotNil(t, initC, "init-managed-pipelines container should exist")

	envSlice, found, err := unstructured.NestedSlice(initC, "env")
	require.NoError(t, err)
	require.True(t, found)

	cases := []struct {
		name string
		want string
	}{
		{"RELATED_IMAGE_BOOLISH", "true"},
		{"RELATED_IMAGE_NULLISH", "null"},
		{"RELATED_IMAGE_NUMERIC", "123"},
	}
	for _, tc := range cases {
		var envMap map[string]interface{}
		for _, e := range envSlice {
			em, ok := e.(map[string]interface{})
			require.True(t, ok)
			if em["name"] == tc.name {
				envMap = em
				break
			}
		}
		require.NotNil(t, envMap, "env %s should exist", tc.name)
		rawVal, ok := envMap["value"]
		require.True(t, ok, "env %s should have value field", tc.name)
		_, isStr := rawVal.(string)
		require.True(t, isStr, "env %s value must be YAML string scalar, got %T (%v)", tc.name, rawVal, rawVal)
		assert.Equal(t, tc.want, rawVal)
	}
}

func TestDeployAPIServerWithManagedPipelines_ForwardsConfiguredManagedPipelineImageEnv(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)
	t.Setenv("RELATED_IMAGE_TOOLBOX", "registry.example/toolbox@sha256:aaa")
	t.Setenv("RELATED_IMAGE_RHEL_AI", "registry.example/rhelai@sha256:bbb")

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	dspa := testutil.CreateDSPAWithManagedPipelines(
		"quay.io/opendatahub/pipelines-components:latest",
		nil,
		nil,
	)
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.NoError(t, err)

	err = reconciler.ReconcileAPIServer(ctx, dspa, params)
	require.NoError(t, err)

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	require.True(t, created)
	require.NoError(t, err)

	initC := getInitManagedPipelinesContainer(t, deployment)
	require.NotNil(t, initC)
	value, found := getEnvValue(t, initC, "RELATED_IMAGE_RHEL_AI")
	require.True(t, found)
	assert.Equal(t, "registry.example/rhelai@sha256:bbb", value)
	value, found = getEnvValue(t, initC, "RELATED_IMAGE_TOOLBOX")
	require.True(t, found)
	assert.Equal(t, "registry.example/toolbox@sha256:aaa", value)
	// Built-in env for pipeline selection still present
	value, found = getEnvValue(t, initC, "ALL_PIPELINES")
	require.True(t, found)
	assert.Equal(t, "true", value)
}

func TestDeployAPIServerWithManagedPipelines_ForwardsConfiguredManagedPipelineImageEnv_QuotesSpecialChars(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)
	t.Setenv("RELATED_IMAGE_TOOLBOX", `registry.example/img:latest"withquotes`)

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	dspa := testutil.CreateDSPAWithManagedPipelines(
		"quay.io/opendatahub/pipelines-components:latest",
		nil,
		nil,
	)
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	require.True(t, created)
	require.NoError(t, err)

	initC := getInitManagedPipelinesContainer(t, deployment)
	require.NotNil(t, initC)
	value, found := getEnvValue(t, initC, "RELATED_IMAGE_TOOLBOX")
	require.True(t, found)
	assert.Equal(t, `registry.example/img:latest"withquotes`, value)
}

func TestExtractParams_ManagedPipelineImageEnvVarsNilWithoutManagedPipelines(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)
	t.Setenv("RELATED_IMAGE_SHOULD_NOT_APPEAR", "x")

	dspa := testutil.CreateEmptyDSPA()
	dspa.Name = "dspa"
	dspa.Namespace = "ns"
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true}
	dspa.Spec.APIServer.ManagedPipelines = nil

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))

	assert.Nil(t, params.ManagedPipelineImageEnvVars)
}

func TestExtractParams_ManagedPipelineImageEnvVarsDefaultsToEmpty(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)
	// Do not set any RELATED_IMAGE_* env vars.

	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))

	require.NotNil(t, params.ManagedPipelineImageEnvVars)
	assert.Empty(t, params.ManagedPipelineImageEnvVars)
}

func TestDeployAPIServerWithManagedPipelines_EmptyMappingNoRelatedImageEnv(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	dspa := testutil.CreateDSPAWithManagedPipelines(
		"quay.io/opendatahub/pipelines-components:latest",
		nil,
		nil,
	)
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	require.True(t, created)
	require.NoError(t, err)

	initC := getInitManagedPipelinesContainer(t, deployment)
	require.NotNil(t, initC)

	for _, e := range initC.Env {
		assert.False(t, strings.HasPrefix(e.Name, relatedImageEnvPrefix),
			"no RELATED_IMAGE_* env should be present when mapping is empty, found %s", e.Name)
	}
}

func TestReconcileAPIServer_ConfigHashIncludesManagedPipelineImageEnvVars(t *testing.T) {
	ctx, _, reconciler := CreateNewTestObjects()
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)

	dspa := testutil.CreateDSPAWithManagedPipelines("same-img", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	t.Setenv("RELATED_IMAGE_X", "digest-one")
	params1 := &DSPAParams{}
	require.NoError(t, params1.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params1))

	t.Setenv("RELATED_IMAGE_X", "digest-two")
	params2 := &DSPAParams{}
	require.NoError(t, params2.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params2))

	assert.NotEqual(t, params1.APIServerConfigHash, params2.APIServerConfigHash,
		"hash should change when RELATED_IMAGE_* env vars change")
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

func TestDeployAPIServerWithManagedPipelines_OnlyDetectedManagedImageEnvVarsOnInitContainer(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)
	t.Setenv("RELATED_IMAGE_SENTINEL", "registry.example/sentinel@sha256:ccc")

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedAPIServerName := apiServerDefaultResourceNamePrefix + testDSPAName

	dspa := testutil.CreateDSPAWithManagedPipelines(
		"quay.io/opendatahub/pipelines-components:latest",
		nil,
		nil,
	)
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotEmpty(t, params.ManagedPipelineImageEnvVars)
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedAPIServerName, testNamespace)
	require.True(t, created)
	require.NoError(t, err)

	initC := getInitManagedPipelinesContainer(t, deployment)
	require.NotNil(t, initC)

	var managedImageEnvCount int
	for _, e := range initC.Env {
		if strings.HasPrefix(e.Name, relatedImageEnvPrefix) {
			managedImageEnvCount++
		}
	}
	value, found := getEnvValue(t, initC, "RELATED_IMAGE_SENTINEL")
	require.True(t, found)
	assert.Equal(t, "registry.example/sentinel@sha256:ccc", value)
	assert.Equal(t, len(params.ManagedPipelineImageEnvVars), managedImageEnvCount,
		"init container should carry exactly the RELATED_IMAGE_* env vars from operator environment")
}

func TestReconcileAPIServer_ConfigHashIdempotent(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })
	clearRelatedImageEnv(t)
	t.Setenv("RELATED_IMAGE_X", "digest-same")

	ctx, _, reconciler := CreateNewTestObjects()

	dspa := testutil.CreateDSPAWithManagedPipelines("same-img", nil, nil)
	dspa.Name = "dspa"
	dspa.Namespace = "ns"

	params1 := &DSPAParams{}
	require.NoError(t, params1.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params1))

	params2 := &DSPAParams{}
	require.NoError(t, params2.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params2))

	assert.Equal(t, params1.APIServerConfigHash, params2.APIServerConfigHash,
		"hash should be identical across reconciles with the same input")
}
