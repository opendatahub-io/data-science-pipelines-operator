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
	"encoding/json"
	"errors"
	"fmt"
	TestUtil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func (suite *IntegrationTestSuite) TestFetchArtifacts() {

	suite.T().Run("Should successfully fetch artifacts", func(t *testing.T) {

		podName, err := getPodName(clientmgr.clientset, DSPANamespace, "app=ds-pipeline-"+DSPANamespace)
		require.NoError(t, err)

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

		response, err := http.Post(uploadUrl, contentTypeUpload, bodyUpload)
		require.NoError(t, err)
		responseData, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, response.StatusCode)

		// Retrieve Pipeline ID to create a new run
		pipelineID, err := TestUtil.RetrievePipelineId(t, APIServerURL, name)
		require.NoError(t, err)

		// Create a new run
		runUrl := fmt.Sprintf("%s/apis/v2beta1/runs", APIServerURL)
		bodyRun := TestUtil.FormatRequestBody(t, pipelineID, name)
		contentTypeRun := "application/json"
		response, err = http.Post(runUrl, contentTypeRun, bytes.NewReader(bodyRun))
		require.NoError(t, err)
		responseData, err = io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		err = TestUtil.WaitForPipelineRunCompletion(t, APIServerURL)
		require.NoError(t, err)

		// fetch artifacts
		artifactsUrl := fmt.Sprintf("%s/apis/v2beta1/artifacts?namespace=%s", APIServerURL, suite.DSPANamespace)
		response, err = http.Get(artifactsUrl)
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
		has_download_error := false
		for _, artifact := range responseArtifactsData.Artifacts {
			// get the artifact by ID
			artifactsByIdUrl := fmt.Sprintf("%s/apis/v2beta1/artifacts/%s", APIServerURL, artifact.ArtifactID)
			response, err = http.Get(artifactsByIdUrl)
			require.NoError(t, err)
			responseData, err = io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, response.StatusCode)

			// get download url
			artifactsByIdUrl = fmt.Sprintf("%s/apis/v2beta1/artifacts/%s?view=DOWNLOAD", APIServerURL, artifact.ArtifactID)
			response, err = http.Get(artifactsByIdUrl)
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

			downloadUrl, err := getDownloadUrl(responseArtifactData.DownloadUrl)
			if err != nil {
				t.Errorf("Error retrieving the download url: %v", err)
				return
			}

			output, err := execCmdExample(clientmgr.clientset, podName, DSPANamespace, "curl --insecure "+downloadUrl)
			require.NoError(t, err)
			// simple logic in order to demonstrate the issue. it wont be like that once the pr becomes ready for review
			if strings.Contains(output, "Access Denied") {
				has_download_error = true
				loggr.Error(errors.New("error downloading the artifact"), output)
			}
		}
		if has_download_error {
			t.Errorf("Error downloading the artifacts. double check the error messages in the log")
		}

	})
}

func getDownloadUrl(downloadUrl string) (string, error) {
	// the test is running on kind. And it is returning the service
	downloadParsedURL, err := url.Parse(downloadUrl)
	if err != nil {
		return "", err
	}
	downloadParsedURL.RawQuery = url.QueryEscape(downloadParsedURL.RawQuery)
	return downloadParsedURL.String(), nil
}

func execCmdExample(client kubernetes.Interface, podName, namespace string, command string) (string, error) {
	cmd := []string{
		"sh",
		"-c",
		command,
	}
	req := client.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}
	req.VersionedParams(
		option,
		k8sscheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return "", err
	}
	var stderrBuffer bytes.Buffer
	var stdoutBuffer bytes.Buffer

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdoutBuffer,
		Stderr: &stderrBuffer,
	})
	if err != nil {
		return "", err
	}
	return stdoutBuffer.String(), nil
}

func getPodName(client kubernetes.Interface, namespace, labelSelector string) (string, error) {
	pods, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found with the label %s", labelSelector)
	}
	return pods.Items[0].Name, nil
}
