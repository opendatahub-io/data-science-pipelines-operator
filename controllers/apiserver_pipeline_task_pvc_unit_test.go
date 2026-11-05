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

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

// TestAPIServerDeploymentTransitions_PipelineTaskPVCDefaultStorageClass verifies
// that spec.apiServer.pipelineTaskPVCDefaultStorageClass propagates to the
// API server deployment as DEFAULT_PVC_STORAGE_CLASS across the full field
// lifecycle: unset, set, changed, emptied, and removed. Unset and empty must
// leave the env var absent so the backend falls back to Kubernetes defaulting.
func TestAPIServerDeploymentTransitions_PipelineTaskPVCDefaultStorageClass(t *testing.T) {
	ctx, _, reconciler := CreateNewTestObjects()
	dspa := testutil.CreateEmptyDSPA()
	dspa.Spec.APIServer.Deploy = true

	for _, tt := range []struct {
		name string
		set  bool
		// value is only meaningful when set is true
		value    string
		expected string
	}{
		{name: "unset"},
		{name: "set", set: true, value: "gp3-csi", expected: "gp3-csi"},
		{name: "changed", set: true, value: "repro-dashboard-default", expected: "repro-dashboard-default"},
		{name: "emptied"},
		{name: "removed"},
		{name: "set again", set: true, value: "gp3-csi", expected: "gp3-csi"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dspa.Spec.APIServer.PipelineTaskPVCDefaultStorageClass = ""
			if tt.set {
				dspa.Spec.APIServer.PipelineTaskPVCDefaultStorageClass = tt.value
			}

			params := &DSPAParams{}
			require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
			require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

			deployment := &appsv1.Deployment{}
			created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
			require.NoError(t, err)
			require.True(t, created)

			apiContainer := getDSPipelineAPIServerContainer(deployment)
			require.NotNil(t, apiContainer)
			value, found := getEnvValue(t, apiContainer, "DEFAULT_PVC_STORAGE_CLASS")
			assert.Equal(t, tt.set, found,
				"DEFAULT_PVC_STORAGE_CLASS must be present only when pipelineTaskPVCDefaultStorageClass is set")
			if tt.set {
				assert.Equal(t, tt.expected, value)
			}
		})
	}
}

// TestAPIServerDeploymentTemplate_PipelineTaskPVCDefaultStorageClassEscaped
// verifies that a value containing YAML metacharacters (quotes, newlines) is
// rendered as a single escaped YAML scalar and cannot inject keys into the
// container spec.
func TestAPIServerDeploymentTemplate_PipelineTaskPVCDefaultStorageClassEscaped(t *testing.T) {
	ctx, _, reconciler := CreateNewTestObjects()
	dspa := testutil.CreateEmptyDSPA()
	dspa.Spec.APIServer.Deploy = true
	// Closing quote plus newlined keys would escape the quoted scalar and
	// inject a securityContext into the container if the value were rendered
	// unescaped.
	hostile := "x\"\n  securityContext:\n    privileged: true"
	dspa.Spec.APIServer.PipelineTaskPVCDefaultStorageClass = hostile

	params := &DSPAParams{}
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, apiServerDefaultResourceNamePrefix+dspa.Name, dspa.Namespace)
	require.NoError(t, err)
	require.True(t, created)

	apiContainer := getDSPipelineAPIServerContainer(deployment)
	require.NotNil(t, apiContainer)
	value, found := getEnvValue(t, apiContainer, "DEFAULT_PVC_STORAGE_CLASS")
	require.True(t, found)
	assert.Equal(t, hostile, value,
		"hostile value must be preserved verbatim as the env value")
}
