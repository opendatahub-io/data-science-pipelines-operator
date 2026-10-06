//go:build test_all || test_unit

package testUtil

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type pipelineListTransport func(*http.Request) (*http.Response, error)

func (f pipelineListTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRetrievePipelineIdWaitsForPipelineVisibility(t *testing.T) {
	requests := 0
	client := http.Client{Transport: pipelineListTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/apis/v2beta1/pipelines", req.URL.Path)
		requests++
		body := `{"pipelines":[{"pipeline_id":"managed-id","display_name":"managed-pipeline"}]}`
		if requests == 2 {
			body = `{"pipelines":[{"pipeline_id":"managed-id","display_name":"managed-pipeline"},{"pipeline_id":"uploaded-id","display_name":"test-pipeline"}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}

	id, err := RetrievePipelineId(t, client, "http://pipelines.test", "test-pipeline")
	require.NoError(t, err)
	require.Equal(t, "uploaded-id", id)
	require.Equal(t, 2, requests)
}

func TestRetrievePipelineIdRejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "HTTP error", status: http.StatusForbidden, body: `{"error":"forbidden"}`},
		{name: "invalid JSON", status: http.StatusOK, body: `invalid JSON`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := http.Client{Transport: pipelineListTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			id, err := RetrievePipelineId(t, client, "http://pipelines.test", "test-pipeline")
			require.Error(t, err)
			require.Empty(t, id)
		})
	}
}

func TestRetrievePipelineIdResponseSizeLimit(t *testing.T) {
	const maxSize = 10 << 20
	const pipelineJSON = `{"pipelines":[{"pipeline_id":"test-id","display_name":"test-pipeline"}]}`

	for _, tc := range []struct {
		name string
		size int
	}{
		{name: "at limit", size: maxSize},
		{name: "over limit", size: maxSize + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Valid JSON with trailing whitespace verifies that size, not decoding, rejects the response.
			body := pipelineJSON + strings.Repeat(" ", tc.size-len(pipelineJSON))
			client := http.Client{Transport: pipelineListTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}

			id, err := RetrievePipelineId(t, client, "http://pipelines.test", "test-pipeline")
			if tc.size > maxSize {
				require.EqualError(t, err, "pipeline list response exceeds the 10 MiB size limit")
				require.Empty(t, id)
			} else {
				require.NoError(t, err)
				require.Equal(t, "test-id", id)
			}
		})
	}
}
