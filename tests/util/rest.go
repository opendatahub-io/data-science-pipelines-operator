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

package testUtil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type PipelineRequest struct {
	DisplayName              string `json:"display_name"`
	PipelineVersionReference struct {
		PipelineID string `json:"pipeline_id"`
	} `json:"pipeline_version_reference"`
}
type Pipeline struct {
	Pipelines []struct {
		PipelineID  string `json:"pipeline_id"`
		DisplayName string `json:"display_name"`
	} `json:"pipelines"`
}
type PipelineRunResponse struct {
	RunID string `json:"run_id"`
}

// FormFromFile creates a multipart form data from the provided form map where the values are paths to files.
// It returns a buffer containing the encoded form data and the content type of the form.
// Requires passing the testing.T object for error handling with Testify.
func FormFromFile(t *testing.T, form map[string]string) (*bytes.Buffer, string) {
	body := new(bytes.Buffer)
	mp := multipart.NewWriter(body)
	defer mp.Close()

	for key, val := range form {
		if strings.HasPrefix(val, "@") {
			val = val[1:]
			file, err := os.Open(val)
			require.NoError(t, err, "Opening file failed")
			defer file.Close()

			part, err := mp.CreateFormFile(key, val)
			require.NoError(t, err, "Creating form file failed")

			_, err = io.Copy(part, file)
			require.NoError(t, err, "Copying file content failed")
		} else {
			err := mp.WriteField(key, val)
			require.NoError(t, err, "Writing form field failed")
		}
	}

	return body, mp.FormDataContentType()
}

func RetrievePipelineId(t *testing.T, httpClient http.Client, APIServerURL string, PipelineDisplayName string) (string, error) {
	response, err := httpClient.Get(fmt.Sprintf("%s/apis/v2beta1/pipelines", APIServerURL))
	require.NoError(t, err)
	responseData, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var pipelineData Pipeline
	var pipelineID *string
	err = json.Unmarshal(responseData, &pipelineData)
	require.NoError(t, err)
	for _, pipeline := range pipelineData.Pipelines {
		if pipeline.DisplayName == PipelineDisplayName {
			pipelineID = &pipeline.PipelineID
			break
		}
	}

	if pipelineID != nil {
		return *pipelineID, nil
	} else {
		return "", errors.New("pipeline not found")
	}
}

func FormatRequestBody(t *testing.T, pipelineID string, PipelineDisplayName string) []byte {
	requestBody := PipelineRequest{
		DisplayName: PipelineDisplayName,
		PipelineVersionReference: struct {
			PipelineID string `json:"pipeline_id"`
		}{PipelineID: pipelineID},
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)
	return body
}

func WaitForPipelineRunCompletion(t *testing.T, httpClient http.Client, APIServerURL string) error {
	timeout := time.After(6 * time.Minute)
	ticker := time.NewTicker(6 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for pipeline run completion")
		case <-ticker.C:
			// Check the status of the pipeline run
			status, err := CheckPipelineRunStatus(t, httpClient, APIServerURL)
			require.NoError(t, err)
			switch status {
			case "SUCCEEDED":
				return nil
			case "SKIPPED", "FAILED", "CANCELING", "CANCELED", "PAUSED":
				return fmt.Errorf("pipeline run status: %s", status)
			}
		}
	}
}

func CheckPipelineRunStatus(t *testing.T, httpClient http.Client, APIServerURL string) (string, error) {
	response, err := httpClient.Get(fmt.Sprintf("%s/apis/v2beta1/runs", APIServerURL))
	require.NoError(t, err)
	responseData, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var data map[string]interface{}
	var state string
	err = json.Unmarshal(responseData, &data)
	require.NoError(t, err)

	if data["runs"] == nil {
		// No runs found
		return "", nil
	}

	// Extracting the Run state
	runs := data["runs"].([]interface{})
	for _, run := range runs {
		runData := run.(map[string]interface{})
		state = runData["state"].(string)
	}
	return state, nil
}

// RetrieveRunID extracts the run ID from the pipeline run creation response.
func RetrieveRunID(t *testing.T, responseData []byte) string {
	var runResponse PipelineRunResponse
	err := json.Unmarshal(responseData, &runResponse)
	require.NoError(t, err, "Failed to parse run response JSON")

	if runResponse.RunID == "" {
		t.Fatalf("Run ID is empty in response: %s", string(responseData))
	}
	return runResponse.RunID
}

func ApplyPipelineYAML(t *testing.T, yamlPath, namespace string) {
	// Validate inputs to prevent command injection
	if strings.ContainsAny(yamlPath, "|;&$`") || strings.ContainsAny(namespace, "|;&$`") {
		t.Fatalf("Invalid characters in yamlPath or namespace")
	}
	if !strings.HasSuffix(yamlPath, ".yaml") && !strings.HasSuffix(yamlPath, ".yml") {
		t.Fatalf("yamlPath must be a YAML file")
	}

	cmd := exec.Command("kubectl", "apply", "-f", yamlPath, "-n", namespace)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "failed to apply pipeline YAML (%s):\n%s", yamlPath, string(out))
}
