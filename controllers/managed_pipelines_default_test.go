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
	"fmt"
	"os"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/dspastatus"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

func TestManagedPipelinesOmittedDefaultsWithoutMutatingDSPA(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	t.Cleanup(viper.Reset)
	viper.Set(config.PipelinesComponentsImagePath, "pipelines-components:test")
	dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
	dspa.Spec.APIServer.ManagedPipelines = nil
	require.True(t, dspa.ManagedPipelinesEnabled())
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.APIServer.ManagedPipelines)
	require.Equal(t, "pipelines-components:test", params.APIServer.ManagedPipelines.Image)
	require.Nil(t, dspa.Spec.APIServer.ManagedPipelines)
	status := dspastatus.NewDSPAStatus(dspa)
	proceed, requeue, err := reconciler.validateManagedPipelines(ctx, dspa, status, reconciler.Log)
	require.NoError(t, err)
	require.True(t, proceed)
	require.False(t, requeue)
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))
}

func TestManagedPipelinesOmittedWithoutImageFails(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	t.Cleanup(viper.Reset)
	viper.Set(config.PipelinesComponentsImagePath, config.DefaultImageValue)
	dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
	dspa.Spec.APIServer.ManagedPipelines = nil
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.ErrorIs(t, err, ErrManagedPipelinesImageUnset)
	require.Nil(t, dspa.Spec.APIServer.ManagedPipelines)
}

func TestManagedPipelinesReconcileDefaulting(t *testing.T) {
	data, err := os.ReadFile("../config/crd/bases/datasciencepipelinesapplications.opendatahub.io_datasciencepipelinesapplications.yaml")
	require.NoError(t, err)
	var crd apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(data, &crd))
	managedSchema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["apiServer"].Properties["managedPipelines"]
	require.Nil(t, managedSchema.Default, "the CRD must not enable pipelines for old operators")
	var schema apiextensions.JSONSchemaProps
	require.NoError(t, apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(crd.Spec.Versions[0].Schema.OpenAPIV3Schema, &schema, nil))
	structural, err := structuralschema.NewStructural(&schema)
	require.NoError(t, err)

	const operatorImage = "quay.io/opendatahub/odh-pipelines-components:odh-stable"
	for _, tt := range []struct {
		name      string
		apiServer string
		optOut    string
		disabled  bool
	}{
		{name: "apiServer omitted"},
		{name: "apiServer empty", apiServer: `{}`},
		{name: "managedPipelines empty", apiServer: `{"managedPipelines":{}}`},
		{name: "explicit null follows default", apiServer: `{"managedPipelines":null}`},
		{name: "explicit enabled", apiServer: `{"managedPipelines":{"enabled":true}}`},
		{name: "explicit disabled", apiServer: `{"managedPipelines":{"enabled":false}}`, disabled: true},
		{name: "null enabled follows default", apiServer: `{"managedPipelines":{"enabled":null}}`},
		{name: "false annotation cannot override spec opt-out", apiServer: `{"managedPipelines":{"enabled":false}}`, optOut: "false", disabled: true},
		{name: "annotation overrides explicit enabled", apiServer: `{"managedPipelines":{"enabled":true}}`, optOut: "true", disabled: true},
		{name: "disabled preserves configured pipelines", apiServer: `{"managedPipelines":{"enabled":false,"image":"custom:latest","pipelines":[{"name":"trainer-ostf"}]}}`, disabled: true},
		{name: "pre-upgrade annotation opt-out", optOut: "true", disabled: true},
		{name: "annotation overrides configured pipelines", apiServer: `{"managedPipelines":{"image":"custom:latest","pipelines":[{"name":"trainer-ostf"}]}}`, optOut: "true", disabled: true},
		{name: "false annotation does not opt out", optOut: "false"},
		{name: "explicit overrides", apiServer: `{"managedPipelines":{"image":"custom:latest","pipelines":[{"name":"trainer-ostf"}],"resources":{"requests":{"cpu":"100m","memory":"128Mi"}},"volumeSizeLimit":"256Mi"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, params, reconciler := CreateNewTestObjects()
			t.Cleanup(viper.Reset)
			viper.Set(config.PipelinesComponentsImagePath, operatorImage)
			dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
			dspa.Name, dspa.Namespace = "testdspa", "testnamespace"
			if tt.optOut != "" {
				dspa.Annotations = map[string]string{"datasciencepipelinesapplications.opendatahub.io/disable-managed-pipelines": tt.optOut}
			}
			data, err := json.Marshal(dspa)
			require.NoError(t, err)
			var object map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &object))
			spec := object["spec"].(map[string]interface{})
			delete(spec, "apiServer")
			if tt.apiServer != "" {
				var apiServer map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(tt.apiServer), &apiServer))
				spec["apiServer"] = apiServer
			}

			defaulting.PruneNonNullableNullsWithoutDefaults(object, structural)
			defaulting.Default(object, structural)
			defaulted := runtime.DeepCopyJSONValue(object)
			defaulting.Default(object, structural)
			assert.Equal(t, defaulted, object, "defaulting must be idempotent")
			apiServer := spec["apiServer"].(map[string]interface{})
			managed, present := apiServer["managedPipelines"]
			if tt.apiServer == "" || tt.apiServer == `{}` {
				require.False(t, present, "omitted configuration must not be defaulted into the stored DSPA")
			} else if tt.apiServer == `{"managedPipelines":null}` {
				require.True(t, present)
				require.Nil(t, managed)
			} else {
				require.True(t, present)
				require.NotNil(t, managed)
				if tt.name == "explicit overrides" {
					var original map[string]interface{}
					require.NoError(t, json.Unmarshal([]byte(tt.apiServer), &original))
					assert.Equal(t, original["managedPipelines"], managed)
				}
			}
			data, err = json.Marshal(object)
			require.NoError(t, err)
			dspa = &dspav1.DataSciencePipelinesApplication{}
			require.NoError(t, json.Unmarshal(data, dspa))
			// Typed updates must not persist reconciliation-only defaults.
			data, err = json.Marshal(dspa)
			require.NoError(t, err)
			var roundTrip map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &roundTrip))
			defaulting.Default(roundTrip, structural)
			assert.Equal(t, managed, roundTrip["spec"].(map[string]interface{})["apiServer"].(map[string]interface{})["managedPipelines"])
			require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
			stored, err := json.Marshal(dspa)
			require.NoError(t, err)
			assert.JSONEq(t, string(data), string(stored), "parameter resolution must not mutate the DSPA")
			require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))
			deployment := &appsv1.Deployment{}
			created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
			require.NoError(t, err)
			require.True(t, created)
			initContainer := getInitManagedPipelinesContainer(t, deployment)
			apiContainer := getDSPipelineAPIServerContainer(deployment)
			require.NotNil(t, apiContainer)
			if tt.disabled {
				require.Nil(t, params.APIServer.ManagedPipelines)
				var sampleConfig map[string]interface{}
				require.NoError(t, json.Unmarshal([]byte(params.SampleConfigJSON), &sampleConfig))
				assert.Empty(t, sampleConfig["pipelines"], "opt-outs must not reference unavailable managed pipeline files")
				assert.Nil(t, initContainer)
				assert.NotContains(t, apiContainer.Args, "--managedPipelinesDir=/config/managed-pipelines")
				_, found := getEnvValue(t, apiContainer, "MANAGED_PIPELINES_UPLOAD_TAGS")
				assert.False(t, found)
				for _, volume := range deployment.Spec.Template.Spec.Volumes {
					assert.NotEqual(t, "managed-pipelines", volume.Name)
				}
				for _, container := range deployment.Spec.Template.Spec.Containers {
					for _, mount := range container.VolumeMounts {
						assert.NotEqual(t, "managed-pipelines", mount.Name)
					}
				}
				return
			}
			require.NotNil(t, initContainer)
			if tt.name == "explicit overrides" {
				assert.Equal(t, "custom:latest", initContainer.Image)
				value, found := getEnvValue(t, initContainer, "PIPELINE_NAMES")
				assert.True(t, found)
				assert.Equal(t, "trainer-ostf", value)
			} else {
				assert.Equal(t, operatorImage, initContainer.Image)
				value, found := getEnvValue(t, initContainer, "ALL_PIPELINES")
				assert.True(t, found)
				assert.Equal(t, "true", value)
			}
			assert.Contains(t, apiContainer.Args, "--managedPipelinesDir=/config/managed-pipelines")
		})
	}
}

func TestManagedPipelinesOptOutSkipsValidation(t *testing.T) {
	for _, annotation := range []bool{true, false} {
		t.Run(fmt.Sprintf("annotation=%t", annotation), func(t *testing.T) {
			ctx, _, reconciler := CreateNewTestObjects()
			dspa := testutil.CreateDSPAWithManagedPipelines("unavailable:test", []dspav1.ManagedPipeline{{Name: "trainer-ostf"}}, nil)
			if annotation {
				dspa.Annotations = map[string]string{dspav1.DisableManagedPipelinesAnnotation: "true"}
			} else {
				dspa.Spec.APIServer.ManagedPipelines.Enabled = ptr.To(false)
			}
			reconciler.ManifestFetcher = &mockPipelineNamesFetcher{err: assert.AnError}
			status := dspastatus.NewDSPAStatus(dspa)
			proceed, requeue, err := reconciler.validateManagedPipelines(ctx, dspa, status, reconciler.Log)
			require.NoError(t, err)
			require.True(t, proceed)
			require.False(t, requeue, "opt-out must bypass manifest fetching")
			condition := findCondition(status.GetConditions(), config.ManagedPipelineValid)
			require.NotNil(t, condition)
			assert.Equal(t, "NotApplicable", condition.Reason)
		})
	}
}

func TestManagedPipelinesOptOutBypassesMissingImage(t *testing.T) {
	for _, annotation := range []bool{true, false} {
		t.Run(fmt.Sprintf("annotation=%t", annotation), func(t *testing.T) {
			ctx, params, reconciler := CreateNewTestObjects()
			t.Cleanup(viper.Reset)
			viper.Set(config.PipelinesComponentsImagePath, "")
			dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
			if annotation {
				dspa.Annotations = map[string]string{dspav1.DisableManagedPipelinesAnnotation: "true"}
			} else {
				dspa.Spec.APIServer.ManagedPipelines.Enabled = ptr.To(false)
			}
			require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
			require.Nil(t, params.APIServer.ManagedPipelines)
			require.NotNil(t, dspa.Spec.APIServer.ManagedPipelines, "opt-out must not erase stored configuration")
		})
	}
}

func TestManagedPipelinesOptOutPreservesIrisSample(t *testing.T) {
	for _, annotation := range []bool{true, false} {
		t.Run(fmt.Sprintf("annotation=%t", annotation), func(t *testing.T) {
			ctx, params, reconciler := CreateNewTestObjects()
			t.Cleanup(viper.Reset)
			viper.Set("ManagedPipelinesMetadata.iris.Name", "[Demo] iris")
			viper.Set("ManagedPipelinesMetadata.iris.Filepath", "/samples/iris.yaml")
			dspa := testutil.CreateDSPAWithManagedPipelines("", []dspav1.ManagedPipeline{{Name: "trainer-ostf"}}, nil)
			dspa.Spec.APIServer.EnableSamplePipeline = true
			if annotation {
				dspa.Annotations = map[string]string{dspav1.DisableManagedPipelinesAnnotation: "true"}
			} else {
				dspa.Spec.APIServer.ManagedPipelines.Enabled = ptr.To(false)
			}
			require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
			require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))
			var sampleConfig struct {
				Pipelines []map[string]string `json:"pipelines"`
			}
			require.NoError(t, json.Unmarshal([]byte(params.SampleConfigJSON), &sampleConfig))
			require.Len(t, sampleConfig.Pipelines, 1)
			assert.Equal(t, "[Demo] iris", sampleConfig.Pipelines[0]["name"])
			assert.Equal(t, "/samples/iris.yaml", sampleConfig.Pipelines[0]["file"])
			deployment := &appsv1.Deployment{}
			created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
			require.NoError(t, err)
			require.True(t, created)
			assert.Nil(t, getInitManagedPipelinesContainer(t, deployment))
		})
	}
}

func TestManagedPipelinesDeploymentTransitions(t *testing.T) {
	ctx, _, reconciler := CreateNewTestObjects()
	t.Cleanup(viper.Reset)
	viper.Set(config.PipelinesComponentsImagePath, "pipelines-components:test")
	dspa := testutil.CreateDSPAWithManagedPipelines("", nil, nil)
	var enabledTemplate *corev1.PodTemplateSpec
	var previousHash string

	for _, tt := range []struct {
		name         string
		enabled      bool
		annotated    bool
		configured   bool
		specDisabled bool
	}{
		{name: "enabled", enabled: true},
		{name: "omitted settings opt-out", annotated: true},
		{name: "omitted settings re-enabled", enabled: true},
		{name: "configured settings opt-out", annotated: true, configured: true},
		{name: "configured settings re-enabled", enabled: true, configured: true},
		{name: "spec opt-out", specDisabled: true},
		{name: "spec opt-out removed", enabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dspa.Spec.APIServer.ManagedPipelines = nil
			if tt.configured {
				dspa.Spec.APIServer.ManagedPipelines = &dspav1.ManagedPipelinesSpec{}
			}
			if tt.specDisabled {
				dspa.Spec.APIServer.ManagedPipelines = &dspav1.ManagedPipelinesSpec{}
				dspa.Spec.APIServer.ManagedPipelines.Enabled = ptr.To(false)
			}
			dspa.Annotations = nil
			if tt.annotated {
				dspa.Annotations = map[string]string{dspav1.DisableManagedPipelinesAnnotation: "true"}
			}
			params := &DSPAParams{}
			require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
			require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))
			deployment := &appsv1.Deployment{}
			created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
			require.NoError(t, err)
			require.True(t, created)
			pod := deployment.Spec.Template.Spec
			initContainer := getInitManagedPipelinesContainer(t, deployment)
			apiContainer := getDSPipelineAPIServerContainer(deployment)
			require.NotNil(t, apiContainer)
			_, hasTags := getEnvValue(t, apiContainer, "MANAGED_PIPELINES_UPLOAD_TAGS")
			assert.Equal(t, tt.enabled, hasTags)
			if tt.enabled {
				require.NotNil(t, initContainer)
				assert.Equal(t, "pipelines-components:test", initContainer.Image)
				assert.Contains(t, apiContainer.Args, "--managedPipelinesDir=/config/managed-pipelines")
				assert.Contains(t, apiContainer.VolumeMounts, corev1.VolumeMount{Name: "managed-pipelines", MountPath: "/config/managed-pipelines"})
				assert.Contains(t, initContainer.VolumeMounts, corev1.VolumeMount{Name: "managed-pipelines", MountPath: "/config/managed-pipelines"})
				hasVolume := false
				for _, volume := range pod.Volumes {
					if volume.Name == "managed-pipelines" {
						hasVolume = true
						require.NotNil(t, volume.EmptyDir)
					}
				}
				assert.True(t, hasVolume)
			} else {
				assert.Nil(t, initContainer)
				assert.NotContains(t, apiContainer.Args, "--managedPipelinesDir=/config/managed-pipelines")
				for _, volume := range pod.Volumes {
					assert.NotEqual(t, "managed-pipelines", volume.Name)
				}
				for _, container := range append(pod.Containers, pod.InitContainers...) {
					for _, mount := range container.VolumeMounts {
						assert.NotEqual(t, "managed-pipelines", mount.Name)
					}
					_, hasTags := getEnvValue(t, &container, "MANAGED_PIPELINES_UPLOAD_TAGS")
					assert.False(t, hasTags)
				}
			}
			hash := deployment.Spec.Template.Annotations["configHash"]
			require.NotEmpty(t, hash)
			if previousHash != "" {
				assert.NotEqual(t, previousHash, hash, "each transition must trigger a rollout")
			}
			previousHash = hash
			if tt.name == "enabled" {
				enabledTemplate = deployment.Spec.Template.DeepCopy()
			} else if tt.enabled {
				assert.Equal(t, enabledTemplate, &deployment.Spec.Template, "re-enabling must restore the original pod template")
			}
		})
	}
}
