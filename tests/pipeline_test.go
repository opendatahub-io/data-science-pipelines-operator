//go:build test_integration

/*
Copyright 2023.

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
	"fmt"
	"io"
	"net/url"
	"testing"

	TestUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *IntegrationTestSuite) TestAPIServerDeployment() {
	suite.T().Run("Should successfully fetch pipelines", func(t *testing.T) {
		response, err := suite.Clientmgr.httpClient.Get(fmt.Sprintf("%s/apis/v2beta1/pipelines", APIServerURL))
		require.NoError(t, err)

		responseData, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
		loggr.Info(string(responseData))
	})

	suite.T().Run("Should successfully upload a pipeline", func(t *testing.T) {

		name := "test-pipeline-run"
		postUrl := fmt.Sprintf("%s/apis/v2beta1/pipelines/upload?name=%s", APIServerURL, url.QueryEscape(name))
		vals := map[string]string{
			"uploadfile": "@resources/test-pipeline-run.yaml",
		}
		body, contentType := TestUtil.FormFromFile(t, vals)

		response, err := suite.Clientmgr.httpClient.Post(postUrl, contentType, body)
		require.NoError(t, err)
		responseData, err := io.ReadAll(response.Body)
		responseString := string(responseData)
		loggr.Info(responseString)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
	})

	suite.T().Run("Should successfully upload a pipeline with custom pip server", func(t *testing.T) {

		name := "test-pipeline-run-with-custom-pip-server"
		postUrl := fmt.Sprintf("%s/apis/v2beta1/pipelines/upload?name=%s", APIServerURL, url.QueryEscape(name))
		vals := map[string]string{
			"uploadfile": "@resources/test-pipeline-with-custom-pip-server-run.yaml",
		}
		body, contentType := TestUtil.FormFromFile(t, vals)

		response, err := suite.Clientmgr.httpClient.Post(postUrl, contentType, body)
		require.NoError(t, err)
		responseData, err := io.ReadAll(response.Body)
		responseString := string(responseData)
		loggr.Info(responseString)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
	})
}
