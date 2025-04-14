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
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"testing"

	TestUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/require"
)

func (suite *IntegrationTestSuite) TestFetchArtifacts() {
	suite.T().Run("Should successfully fetch and download artifacts", func(t *testing.T) {
		log.Printf("Starting TestFetchArtifacts in namespace: %s", suite.DSPANamespace)

		log.Println("Starting port-forwarding for artifact-service...")

		minioNamespace := suite.DSPANamespace
		minioServiceName := "minio-service" // Default for DSPA namespace

		if suite.MinioNamespace != "default" {
			minioNamespace = suite.MinioNamespace
			minioServiceName = "minio" // Use "minio" for external namespace
		}

		// Start port-forwarding
		minioPortForwardCmd := exec.CommandContext(context.Background(),
			"kubectl", "port-forward", "-n", minioNamespace, fmt.Sprintf("svc/%s", minioServiceName), fmt.Sprintf("%d:9000", 9000))
		minioPortForwardCmd.Stderr = os.Stderr
		minioPortForwardCmd.Stdout = os.Stdout
		err := minioPortForwardCmd.Start()
		require.NoError(t, err, "Failed to start port-forwarding")

		go func() {
			err := minioPortForwardCmd.Wait()
			log.Printf("The Minio port forward ended: %v\n", err)
		}()

		defer func() {
			if minioPortForwardCmd.ProcessState == nil || !minioPortForwardCmd.ProcessState.Exited() {
				_ = minioPortForwardCmd.Process.Kill()
				minioPortForwardCmd.Wait()
			}

			log.Println("Minio port-forward process terminated.")
		}()

		type ResponseArtifact struct {
			ArtifactID   string `json:"artifact_id"`
			DownloadUrl  string `json:"download_url"`
			ArtifactType string `json:"artifact_type"`
		}
		type ResponseArtifactData struct {
			Artifacts []ResponseArtifact `json:"artifacts"`
		}

		name := "Test Iris Pipeline"
		uploadUrl := fmt.Sprintf("%s/apis/v2beta1/pipelines/upload?name=%s", APIServerURL, url.QueryEscape(name))
		log.Printf("Uploading pipeline: %s to URL: %s", name, uploadUrl)

		vals := map[string]string{
			"uploadfile": "@resources/iris_pipeline_without_cache_compiled.yaml",
		}
		bodyUpload, contentTypeUpload := TestUtil.FormFromFile(t, vals)
		response, err := suite.Clientmgr.httpClient.Post(uploadUrl, contentTypeUpload, bodyUpload)
		require.NoError(t, err, "Failed to upload pipeline")
		responseData, err := io.ReadAll(response.Body)
		require.NoError(t, err, "Failed to read response data")
		require.Equal(t, http.StatusOK, response.StatusCode, "Unexpected HTTP status code")
		log.Println("Pipeline uploaded successfully.")

		// Retrieve Pipeline ID
		log.Println("Retrieving Pipeline ID...")
		pipelineID, err := TestUtil.RetrievePipelineId(t, suite.Clientmgr.httpClient, APIServerURL, name)
		require.NoError(t, err, "Failed to retrieve Pipeline ID")
		log.Printf("Pipeline ID: %s", pipelineID)

		// Create a new run
		log.Println("Creating a new pipeline run...")
		runUrl := fmt.Sprintf("%s/apis/v2beta1/runs", APIServerURL)
		bodyRun := TestUtil.FormatRequestBody(t, pipelineID, name)
		contentTypeRun := "application/json"
		response, err = suite.Clientmgr.httpClient.Post(runUrl, contentTypeRun, bytes.NewReader(bodyRun))
		require.NoError(t, err, "Failed to create pipeline run")
		responseData, err = io.ReadAll(response.Body)
		require.NoError(t, err, "Failed to read run response data")
		require.Equal(t, http.StatusOK, response.StatusCode, "Unexpected HTTP status code")
		runID := TestUtil.RetrieveRunID(t, responseData)
		log.Println("Pipeline run created successfully.")

		err = TestUtil.WaitForPipelineRunCompletion(t, suite.Clientmgr.httpClient, APIServerURL)
		require.NoError(t, err, "Pipeline run did not complete successfully")

		// Fetch artifacts from API
		artifactsUrl := fmt.Sprintf("%s/apis/v2beta1/artifacts?run_id=%s&namespace=%s", APIServerURL, runID, suite.DSPANamespace)
		log.Printf("Fetching artifacts from URL: %s", artifactsUrl)
		response, err = suite.Clientmgr.httpClient.Get(artifactsUrl)
		require.NoError(t, err, "Failed to fetch artifacts")
		responseData, err = io.ReadAll(response.Body)
		require.NoError(t, err, "Failed to read artifacts response data")
		require.Equal(t, http.StatusOK, response.StatusCode, "Unexpected HTTP status code")

		// Parse the artifact list
		var responseArtifactsData struct {
			Artifacts []ResponseArtifact `json:"artifacts"`
		}
		err = json.Unmarshal(responseData, &responseArtifactsData)
		require.NoError(t, err, "Failed to parse artifacts response JSON")

		for _, artifact := range responseArtifactsData.Artifacts {
			if artifact.ArtifactType != "system.Model" && artifact.ArtifactType != "sytem.Dataset" {
				continue
			}

			// Fetch download URL
			artifactsByIdUrl := fmt.Sprintf("%s/apis/v2beta1/artifacts/%s?view=DOWNLOAD", APIServerURL, artifact.ArtifactID)
			log.Printf("Fetching download URL for artifact from: %s", artifactsByIdUrl)
			response, err = suite.Clientmgr.httpClient.Get(artifactsByIdUrl)
			require.NoError(t, err, "Failed to fetch download URL")
			responseData, err = io.ReadAll(response.Body)
			require.NoError(t, err, "Failed to read download URL response data")
			require.Equal(t, http.StatusOK, response.StatusCode, "Unexpected HTTP status code")

			var artifactWithDownload ResponseArtifact
			err = json.Unmarshal(responseData, &artifactWithDownload)
			require.NoError(t, err, "Failed to parse download URL response JSON")

			// Modify the URL for local port-forwarding
			parsedURL, err := url.Parse(artifactWithDownload.DownloadUrl)
			require.NoError(t, err, "Failed to parse artifact download URL")

			originalHost := parsedURL.Host
			parsedURL.Host = "127.0.0.1:9000"

			log.Printf("Trying download URL: %s", parsedURL.String())

			// Create and send the request with correct Host header
			req, err := http.NewRequest("GET", parsedURL.String(), nil)
			require.NoError(t, err, "Failed to create request")

			req.Host = originalHost

			// Create a custom HTTP client that ignores TLS verification
			httpClient := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			}

			downloadResp, err := httpClient.Do(req)
			require.NoError(t, err, "Failed to perform request")
			require.Equal(t, http.StatusOK, downloadResp.StatusCode, "Download failed")

			log.Printf("Successfully downloaded artifact from %s", req.URL.String())
		}

		log.Println("TestFetchArtifacts completed successfully.")
	})
}
