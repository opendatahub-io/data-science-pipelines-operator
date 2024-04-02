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
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
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

func RetrievePipelineId(t *testing.T, APIServerURL string, PipelineDisplayName string) string {
	response, err := http.Get(fmt.Sprintf("%s/apis/v2beta1/pipelines", APIServerURL))
	require.NoError(t, err)
	responseData, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var pipelineData Pipeline
	var pipelineID string
	err = json.Unmarshal(responseData, &pipelineData)
	require.NoError(t, err)
	for _, pipeline := range pipelineData.Pipelines {
		if pipeline.DisplayName == PipelineDisplayName {
			pipelineID = pipeline.PipelineID
			break
		}
	}
	return pipelineID
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

func WaitForPipelineRunCompletion(t *testing.T, APIServerURL string) error {
	timeout := time.After(6 * time.Minute)
	ticker := time.NewTicker(6 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for pipeline run completion")
		case <-ticker.C:
			// Check the status of the pipeline run
			status, err := CheckPipelineRunStatus(t, APIServerURL)
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

func CheckPipelineRunStatus(t *testing.T, APIServerURL string) (string, error) {
	response, err := http.Get(fmt.Sprintf("%s/apis/v2beta1/runs", APIServerURL))
	require.NoError(t, err)
	responseData, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var data map[string]interface{}
	var state string
	err = json.Unmarshal(responseData, &data)
	require.NoError(t, err)

	// Extracting the Run state
	runs := data["runs"].([]interface{})
	for _, run := range runs {
		runData := run.(map[string]interface{})
		state = runData["state"].(string)
	}
	return state, nil
}
