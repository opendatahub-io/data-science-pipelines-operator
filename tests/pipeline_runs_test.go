//go:build test_integration

/*
Copyright 2024.

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
	"bytes"
	"fmt"
	"io"
	"testing"

	TestUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/require"
)

func (suite *IntegrationTestSuite) TestPipelineSuccessfulRun() {

	suite.T().Run("Should create a Pipeline Run", func(t *testing.T) {

		if suite.DSPA.Spec.APIServer.PipelineStore == "kubernetes" {
			t.Log("PipelineStore is set to kubernetes, skipping this test.")
			t.SkipNow()
		}
		suite.runPipelineTest(t, "[Demo] iris-training")
	})

	suite.T().Run("Should create a Pipeline Run using custom pip server", func(t *testing.T) {
		suite.runPipelineTest(t, "test-pipeline-run-with-custom-pip-server")
	})

	suite.T().Run("Should create a k8s Pipeline Run", func(t *testing.T) {
		if suite.DSPA.Spec.APIServer.PipelineStore != "kubernetes" {
			t.Log("PipelineStore is not set to kubernetes, skipping k8s pipeline run")
			t.SkipNow()
		}
		// Apply the Pipeline and PipelineVersion Kubernetes resources
		yamlPath := "resources/test-k8s-pipeline.yaml"
		TestUtil.ApplyPipelineYAML(t, yamlPath, suite.DSPANamespace)

		suite.runPipelineTest(t, "test-k8s-pipeline-run")
	})
}

func (suite *IntegrationTestSuite) runPipelineTest(t *testing.T, pipelineDisplayName string) {
	pipelineID, err := TestUtil.RetrievePipelineId(t, suite.Clientmgr.httpClient, APIServerURL, pipelineDisplayName)
	require.NoError(t, err)
	postUrl := fmt.Sprintf("%s/apis/v2beta1/runs", APIServerURL)
	body := TestUtil.FormatRequestBody(t, pipelineID, pipelineDisplayName)
	contentType := "application/json"

	response, err := suite.Clientmgr.httpClient.Post(postUrl, contentType, bytes.NewReader(body))
	require.NoError(t, err)
	responseData, err := io.ReadAll(response.Body)
	responseString := string(responseData)
	loggr.Info(responseString)
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)

	err = TestUtil.WaitForPipelineRunCompletion(t, suite.Clientmgr.httpClient, APIServerURL)
	require.NoError(t, err)
}
