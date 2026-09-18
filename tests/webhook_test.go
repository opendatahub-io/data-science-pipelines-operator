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

package integration

import (
	"strings"

	mf "github.com/manifestival/manifestival"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Exercise admission through the API server, including a rejection that could
// not succeed if the validating webhook were missing or ignored.
func (suite *IntegrationTestSuite) TestPipelineVersionAdmission() {
	if suite.DSPA.Spec.APIServer.PipelineStore != "kubernetes" {
		suite.T().Skip("PipelineVersion admission applies to Kubernetes pipeline storage")
	}
	manifest, err := mf.NewManifest("resources/test-k8s-pipeline.yaml")
	require.NoError(suite.T(), err)
	var version *unstructured.Unstructured
	for _, resource := range manifest.Resources() {
		object := resource.DeepCopy()
		object.SetNamespace(suite.DSPANamespace)
		switch object.GetKind() {
		case "Pipeline":
			object.SetName("admission-test-pipeline")
		case "PipelineVersion":
			object.SetName("admission-test-pipeline-v1")
			require.NoError(suite.T(), unstructured.SetNestedField(object.Object, "admission-test-pipeline", "spec", "pipelineName"))
			version = object
		}
		require.NoError(suite.T(), suite.Clientmgr.k8sClient.Create(suite.Ctx, object))
		suite.T().Cleanup(func() {
			require.NoError(suite.T(), client.IgnoreNotFound(suite.Clientmgr.k8sClient.Delete(suite.Ctx, object)))
		})
	}
	require.NotNil(suite.T(), version)
	require.NoError(suite.T(), suite.Clientmgr.k8sClient.Get(suite.Ctx, client.ObjectKeyFromObject(version), version))
	labels := version.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["testing.opendatahub.io/admission"] = "allowed"
	version.SetLabels(labels)
	require.NoError(suite.T(), suite.Clientmgr.k8sClient.Update(suite.Ctx, version))
	require.NoError(suite.T(), unstructured.SetNestedField(version.Object, "changed-immutable-pipeline", "spec", "pipelineSpec", "pipelineInfo", "name"))
	err = suite.Clientmgr.k8sClient.Update(suite.Ctx, version)
	require.Error(suite.T(), err)
	require.Contains(suite.T(), strings.ToLower(err.Error()), "immutable", "expected webhook rejection, not a network or RBAC failure")
}
