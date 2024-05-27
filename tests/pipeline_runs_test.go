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
	"net/http"
	"testing"

	TestUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *IntegrationTestSuite) TestPipelineSuccessfulRun() {

	suite.T().Run("Should create a Pipeline Run", func(t *testing.T) {
		// Retrieve Pipeline ID to create a new run
		pipelineDisplayName := "[Demo] iris-training"
		pipelineID := TestUtil.RetrievePipelineId(t, APIServerURL, pipelineDisplayName)
		postUrl := fmt.Sprintf("%s/apis/v2beta1/runs", APIServerURL)
		body := TestUtil.FormatRequestBody(t, pipelineID, pipelineDisplayName)
		contentType := "application/json"
		// Create a new run
		response, err := http.Post(postUrl, contentType, bytes.NewReader(body))
		require.NoError(t, err)
		responseData, err := io.ReadAll(response.Body)
		responseString := string(responseData)
		loggr.Info(responseString)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
	})

	suite.T().Run("Should successfully complete the Pipeline Run", func(t *testing.T) {
		err := TestUtil.WaitForPipelineRunCompletion(t, APIServerURL)
		require.NoError(t, err)
	})

}
