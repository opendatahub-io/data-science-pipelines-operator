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
	"encoding/json"
	"errors"
	"fmt"
	TestUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

func (suite *IntegrationTestSuite) TestFetchArtifacts() {

	suite.T().Run("Should successfully fetch and download artifacts", func(t *testing.T) {

		if !suite.DSPA.Spec.APIServer.EnableRoute {
			t.Skip("Skipping the test because the download the artifact requires enabling route.")
		}

		if suite.DSPA.Spec.ObjectStorage.Minio != nil {
			t.Skip("The Minio deployed is used for developing/testing purpose and won't work for the download artifact")
		}

		type ResponseArtifact struct {
			ArtifactID  string `json:"artifact_id"`
			DownloadUrl string `json:"download_url"`
		}
		type ResponseArtifactData struct {
			Artifacts []ResponseArtifact `json:"artifacts"`
		}

		name := "Test Iris Pipeline"
		uploadUrl := fmt.Sprintf("%s/apis/v2beta1/pipelines/upload?name=%s", APIServerURL, url.QueryEscape(name))
		vals := map[string]string{
			"uploadfile": "@resources/iris_pipeline_without_cache_compiled.yaml",
		}
		bodyUpload, contentTypeUpload := TestUtil.FormFromFile(t, vals)
		response, err := suite.Clientmgr.httpClient.Post(uploadUrl, contentTypeUpload, bodyUpload)
		require.NoError(t, err)
		responseData, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// Retrieve Pipeline ID to create a new run
		pipelineID, err := TestUtil.RetrievePipelineId(t, suite.Clientmgr.httpClient, APIServerURL, name)
		require.NoError(t, err)

		// Create a new run
		runUrl := fmt.Sprintf("%s/apis/v2beta1/runs", APIServerURL)
		bodyRun := TestUtil.FormatRequestBody(t, pipelineID, name)
		contentTypeRun := "application/json"
		response, err = suite.Clientmgr.httpClient.Post(runUrl, contentTypeRun, bytes.NewReader(bodyRun))
		require.NoError(t, err)
		responseData, err = io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		err = TestUtil.WaitForPipelineRunCompletion(t, suite.Clientmgr.httpClient, APIServerURL)
		require.NoError(t, err)

		// fetch artifacts
		artifactsUrl := fmt.Sprintf("%s/apis/v2beta1/artifacts?namespace=%s", APIServerURL, suite.DSPANamespace)
		response, err = suite.Clientmgr.httpClient.Get(artifactsUrl)
		require.NoError(t, err)
		responseData, err = io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// iterate over the artifacts
		var responseArtifactsData ResponseArtifactData
		err = json.Unmarshal([]byte(string(responseData)), &responseArtifactsData)
		if err != nil {
			t.Errorf("Error unmarshaling JSON: %v", err)
			return
		}
		hasDownloadError := false
		for _, artifact := range responseArtifactsData.Artifacts {
			// get the artifact by ID
			artifactsByIdUrl := fmt.Sprintf("%s/apis/v2beta1/artifacts/%s", APIServerURL, artifact.ArtifactID)
			response, err = suite.Clientmgr.httpClient.Get(artifactsByIdUrl)
			require.NoError(t, err)
			responseData, err = io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, response.StatusCode)

			// get download url
			artifactsByIdUrl = fmt.Sprintf("%s/apis/v2beta1/artifacts/%s?view=DOWNLOAD", APIServerURL, artifact.ArtifactID)
			response, err = suite.Clientmgr.httpClient.Get(artifactsByIdUrl)
			require.NoError(t, err)
			responseData, err = io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, response.StatusCode)
			loggr.Info(string(responseData))

			var responseArtifactData ResponseArtifact
			err = json.Unmarshal([]byte(string(responseData)), &responseArtifactData)
			if err != nil {
				t.Errorf("Error unmarshaling JSON: %v", err)
				return
			}

			content, err := downloadFile(responseArtifactData.DownloadUrl, "/tmp/download", suite.Clientmgr.httpClient)

			require.NoError(t, err)
			// There were an issue in the past that the URL was returning Access Denied
			if strings.Contains(content, "Access Denied") {
				hasDownloadError = true
				loggr.Error(errors.New("error downloading the artifact"), content)
			}
		}
		if hasDownloadError {
			t.Errorf("Error downloading the artifacts. Double check the error messages in the log")
		}
	})
}

func downloadFile(url, filepath string, httpClient http.Client) (string, error) {
	// Create an HTTP GET request to fetch the file from the URL
	response, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch the file: %w", err)
	}
	defer response.Body.Close()

	// Check if the response status is OK (200)
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file: status code %d", response.StatusCode)
	}

	// Read the content from the response body
	content, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}

	// Create the file
	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Write the content to the file
	_, err = file.Write(content)
	if err != nil {
		return "", fmt.Errorf("failed to write content to file: %w", err)
	}

	return string(content), nil
}
