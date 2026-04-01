//go:build test_all || test_unit

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

package controllers

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	oci "github.com/google/go-containerregistry/pkg/v1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/dspastatus"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// --- extractManifestFromImage mocks and tests ---

type mockContainerImage struct {
	oci.Image
	layers    []oci.Layer
	layersErr error
}

func (m *mockContainerImage) Layers() ([]oci.Layer, error) {
	return m.layers, m.layersErr
}

type mockContainerLayer struct {
	oci.Layer
	reader io.ReadCloser
	err    error
}

func (m *mockContainerLayer) Uncompressed() (io.ReadCloser, error) {
	return m.reader, m.err
}

func TestExtractManifestFromImage_LayerOpenError(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)

	failLayer := &mockContainerLayer{err: fmt.Errorf("layer read failure")}
	img := &mockContainerImage{layers: []oci.Layer{failLayer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open layer")
	assert.Contains(t, err.Error(), "test-image:latest")
}

func TestExtractManifestFromImage_TarScanError(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)

	corruptReader := io.NopCloser(bytes.NewReader([]byte("not a tar")))
	layer := &mockContainerLayer{reader: corruptReader}
	img := &mockContainerImage{layers: []oci.Layer{layer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan layer")
}

func TestExtractManifestFromImage_OversizedTopLayer_NoFallback(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)

	oversizedContent := make([]byte, maxManagedPipelinesManifestSize+1)
	newerLayer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, managedPipelinesJSONPath, oversizedContent)),
	}
	validContent := []byte(`[{"name":"good-pipeline"}]`)
	olderLayer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, managedPipelinesJSONPath, validContent)),
	}
	img := &mockContainerImage{layers: []oci.Layer{olderLayer, newerLayer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limit")
}

func TestExtractManifestFromImage_FileFound(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)

	content := []byte(`[{"name":"test-pipeline"}]`)
	layer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, managedPipelinesJSONPath, content)),
	}
	img := &mockContainerImage{layers: []oci.Layer{layer}}

	data, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestExtractManifestFromImage_FileNotFound(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)

	layer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, "app/other-file.json", []byte("data"))),
	}
	img := &mockContainerImage{layers: []oci.Layer{layer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in image")
}

// --- ParseManagedPipelinesManifest tests ---

func TestParseManagedPipelinesManifest_ValidJSON(t *testing.T) {
	entries := []ManagedPipelineEntry{
		{Name: "pipeline_a", Description: "desc a", Path: "path/a", Stability: "alpha"},
		{Name: "pipeline_b", Description: "desc b", Path: "path/b", Stability: "beta"},
	}
	data, err := json.Marshal(entries)
	require.NoError(t, err)

	names, err := ParseManagedPipelinesManifest(data)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"pipeline_a": true, "pipeline_b": true}, names)
}

func TestParseManagedPipelinesManifest_EmptyArray(t *testing.T) {
	data := []byte(`[]`)

	names, err := ParseManagedPipelinesManifest(data)
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestParseManagedPipelinesManifest_MalformedJSON(t *testing.T) {
	data := []byte(`not valid json`)

	_, err := ParseManagedPipelinesManifest(data)
	assert.Error(t, err)
}

func TestParseManagedPipelinesManifest_EntryWithEmptyName(t *testing.T) {
	data := []byte(`[{"name":"","description":"d","path":"p","stability":"alpha"},{"name":"valid","description":"d","path":"p","stability":"alpha"}]`)

	names, err := ParseManagedPipelinesManifest(data)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"valid": true}, names)
	assert.NotContains(t, names, "")
}

func TestParseManagedPipelinesManifest_SingleEntry(t *testing.T) {
	data := []byte(`[{"name":"only_one","description":"d","path":"p","stability":"alpha"}]`)

	names, err := ParseManagedPipelinesManifest(data)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"only_one": true}, names)
}

func TestParseManagedPipelinesManifest_NilInput(t *testing.T) {
	_, err := ParseManagedPipelinesManifest(nil)
	assert.Error(t, err)
}

func TestParseManagedPipelinesManifest_EmptyInput(t *testing.T) {
	_, err := ParseManagedPipelinesManifest([]byte{})
	assert.Error(t, err)
}

// --- ValidateManagedPipelineNames tests ---

func TestValidateManagedPipelineNames_AllValid(t *testing.T) {
	crPipelines := []dspav1.ManagedPipeline{
		{Name: "pipeline_a"},
		{Name: "pipeline_b"},
	}
	manifestNames := map[string]bool{"pipeline_a": true, "pipeline_b": true, "pipeline_c": true}

	err := ValidateManagedPipelineNames(crPipelines, manifestNames)
	assert.NoError(t, err)
}

func TestValidateManagedPipelineNames_OneInvalid(t *testing.T) {
	crPipelines := []dspav1.ManagedPipeline{
		{Name: "pipeline_a"},
		{Name: "nonexistent"},
	}
	manifestNames := map[string]bool{"pipeline_a": true, "pipeline_b": true}

	err := ValidateManagedPipelineNames(crPipelines, manifestNames)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
	assert.Contains(t, err.Error(), "managed-pipelines.json")
}

func TestValidateManagedPipelineNames_MultipleInvalid(t *testing.T) {
	crPipelines := []dspav1.ManagedPipeline{
		{Name: "bad_one"},
		{Name: "pipeline_a"},
		{Name: "bad_two"},
	}
	manifestNames := map[string]bool{"pipeline_a": true}

	err := ValidateManagedPipelineNames(crPipelines, manifestNames)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad_one")
	assert.Contains(t, err.Error(), "bad_two")
}

func TestValidateManagedPipelineNames_EmptyCRList(t *testing.T) {
	manifestNames := map[string]bool{"pipeline_a": true}

	err := ValidateManagedPipelineNames(nil, manifestNames)
	assert.NoError(t, err)

	err = ValidateManagedPipelineNames([]dspav1.ManagedPipeline{}, manifestNames)
	assert.NoError(t, err)
}

func TestValidateManagedPipelineNames_EmptyManifest(t *testing.T) {
	crPipelines := []dspav1.ManagedPipeline{
		{Name: "pipeline_a"},
	}

	err := ValidateManagedPipelineNames(crPipelines, map[string]bool{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline_a")
}

func TestValidateManagedPipelineNames_NilManifest(t *testing.T) {
	crPipelines := []dspav1.ManagedPipeline{
		{Name: "pipeline_a"},
	}

	err := ValidateManagedPipelineNames(crPipelines, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline_a")
}

func TestValidateManagedPipelineNames_CaseSensitive(t *testing.T) {
	crPipelines := []dspav1.ManagedPipeline{
		{Name: "Pipeline_A"},
	}
	manifestNames := map[string]bool{"pipeline_a": true}

	err := ValidateManagedPipelineNames(crPipelines, manifestNames)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Pipeline_A")
}

// --- Status condition tests ---

func TestManagedPipelineValid_SetValid(t *testing.T) {
	dspa := testutil.CreateEmptyDSPA()
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	status.SetManagedPipelineValid()

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond, "ManagedPipelineValid condition must be present")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

func TestManagedPipelineValid_SetInvalid(t *testing.T) {
	dspa := testutil.CreateEmptyDSPA()
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	status.SetManagedPipelineInvalid(
		fmt.Errorf("pipeline \"foo\" not found in managed-pipelines.json"),
		config.ManagedPipelineInvalid,
	)

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond, "ManagedPipelineValid condition must be present")
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelineInvalid, cond.Reason)
	assert.Contains(t, cond.Message, "foo")
	assert.Contains(t, cond.Message, "managed-pipelines.json")
}

func TestManagedPipelineValid_SetNotApplicable(t *testing.T) {
	dspa := testutil.CreateEmptyDSPA()
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	status.SetManagedPipelineNotApplicable()

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond, "ManagedPipelineValid condition must be present")
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, "NotApplicable", cond.Reason)
}

func newAllReadyStatus(t *testing.T) dspastatus.DSPAStatus {
	t.Helper()
	dspa := testutil.CreateEmptyDSPA()
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	status.SetDatabaseReady()
	status.SetObjStoreReady()
	status.SetApiServerStatus(dspastatus.BuildTrueCondition(config.APIServerReady, "ready"))
	status.SetPersistenceAgentStatus(dspastatus.BuildTrueCondition(config.PersistenceAgentReady, "ready"))
	status.SetScheduledWorkflowStatus(dspastatus.BuildTrueCondition(config.ScheduledWorkflowReady, "ready"))
	status.SetWorkflowControllerReady()
	status.SetMLMDProxyStatus(dspastatus.BuildTrueCondition(config.MLMDProxyReady, "ready"))
	status.SetWebhookReady()
	return status
}

func TestManagedPipelineInvalid_MakesOverallNotReady(t *testing.T) {
	status := newAllReadyStatus(t)
	status.SetManagedPipelineInvalid(
		fmt.Errorf("pipeline \"bad\" not found in managed-pipelines.json"),
		config.ManagedPipelineInvalid,
	)

	conditions := status.GetConditions()
	crReady := findCondition(conditions, config.CrReady)
	require.NotNil(t, crReady)
	assert.Equal(t, metav1.ConditionFalse, crReady.Status)
}

func TestManagedPipelineNotApplicable_DoesNotBlockReady(t *testing.T) {
	status := newAllReadyStatus(t)
	status.SetManagedPipelineNotApplicable()

	conditions := status.GetConditions()
	crReady := findCondition(conditions, config.CrReady)
	require.NotNil(t, crReady)
	assert.Equal(t, metav1.ConditionTrue, crReady.Status)
}

func TestManagedPipelineFetchError_DoesNotBlockReady(t *testing.T) {
	status := newAllReadyStatus(t)
	status.SetManagedPipelineInvalid(
		fmt.Errorf("connection refused"),
		config.ManagedPipelinesFetchError,
	)

	conditions := status.GetConditions()

	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelinesFetchError, cond.Reason)

	crReady := findCondition(conditions, config.CrReady)
	require.NotNil(t, crReady)
	assert.Equal(t, metav1.ConditionTrue, crReady.Status,
		"transient fetch error must not degrade CrReady when deployment proceeds")
}

// --- validateManagedPipelines reconciler method tests ---

type mockPipelineNamesFetcher struct {
	names map[string]bool
	err   error
}

func (m *mockPipelineNamesFetcher) FetchPipelineNames(_ context.Context, _ string) (map[string]bool, error) {
	return m.names, m.err
}

func TestValidateManagedPipelines_FetchError_DoesNotBlockReconcile(t *testing.T) {
	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", []dspav1.ManagedPipeline{{Name: "p1"}}, nil)
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	reconciler := &DSPAReconciler{
		Log:             ctrl.Log,
		ManifestFetcher: &mockPipelineNamesFetcher{err: fmt.Errorf("connection refused")},
	}

	proceed, err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	require.NoError(t, err, "fetch errors should not block reconciliation")
	require.True(t, proceed, "fetch errors are transient; API server deployment should proceed")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelinesFetchError, cond.Reason)
	assert.Contains(t, cond.Message, "connection refused")
}

func TestValidateManagedPipelines_ValidationError_BlocksAPIServer(t *testing.T) {
	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", []dspav1.ManagedPipeline{{Name: "bad"}}, nil)
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	reconciler := &DSPAReconciler{
		Log:             ctrl.Log,
		ManifestFetcher: &mockPipelineNamesFetcher{names: map[string]bool{"good": true}},
	}

	proceed, err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	require.NoError(t, err, "validation failures are permanent and must not trigger controller-runtime retries")
	require.False(t, proceed, "validation failures must block API server deployment")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelineInvalid, cond.Reason)
	assert.Contains(t, cond.Message, "bad", "invalid pipeline name must appear in condition message")
}

func TestValidateManagedPipelines_AllValid_SetsCondition(t *testing.T) {
	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", []dspav1.ManagedPipeline{{Name: "p1"}}, nil)
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	reconciler := &DSPAReconciler{
		Log:             ctrl.Log,
		ManifestFetcher: &mockPipelineNamesFetcher{names: map[string]bool{"p1": true}},
	}

	proceed, err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	require.NoError(t, err)
	require.True(t, proceed, "valid pipelines should allow API server deployment")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

func TestValidateManagedPipelines_NotApplicable(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(dspa *dspav1.DataSciencePipelinesApplication)
	}{
		{
			name:   "no pipelines configured",
			mutate: func(_ *dspav1.DataSciencePipelinesApplication) {},
		},
		{
			name:   "nil APIServer",
			mutate: func(dspa *dspav1.DataSciencePipelinesApplication) { dspa.Spec.APIServer = nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dspa := testutil.CreateEmptyDSPA()
			tt.mutate(dspa)
			dspa.Status.Conditions = make([]metav1.Condition, 11)
			status := dspastatus.NewDSPAStatus(dspa)
			reconciler := &DSPAReconciler{Log: ctrl.Log}

			proceed, err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
			require.NoError(t, err)
			require.True(t, proceed, "not-applicable should allow API server deployment")

			conditions := status.GetConditions()
			cond := findCondition(conditions, config.ManagedPipelineValid)
			require.NotNil(t, cond)
			assert.Equal(t, "NotApplicable", cond.Reason)
		})
	}
}

func TestValidateManagedPipelines_NilFetcher_SetsConditionDoesNotPanic(t *testing.T) {
	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", []dspav1.ManagedPipeline{{Name: "p1"}}, nil)
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	reconciler := &DSPAReconciler{Log: ctrl.Log, ManifestFetcher: nil}

	proceed, err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	require.NoError(t, err, "nil fetcher should not panic or block reconciliation")
	require.True(t, proceed, "nil fetcher is transient; API server deployment should proceed")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelinesFetchError, cond.Reason)
}

// --- Permanent vs transient error classification tests ---

func TestValidateManagedPipelines_PermanentFetchError_BlocksDeployment(t *testing.T) {
	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", []dspav1.ManagedPipeline{{Name: "p1"}}, nil)
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	reconciler := &DSPAReconciler{
		Log:             ctrl.Log,
		ManifestFetcher: &mockPipelineNamesFetcher{err: &permanentError{err: fmt.Errorf("managed-pipelines.json not found in image")}},
	}

	proceed, err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	require.NoError(t, err, "permanent errors should not trigger controller-runtime retries")
	require.False(t, proceed, "permanent misconfiguration must block API server deployment")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelineInvalid, cond.Reason)
}

func TestResolveImageDigest_InvalidRef_ReturnsPermanentError(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	_, _, err := fetcher.resolveImageDigest(context.Background(), "@@invalid")
	require.Error(t, err)

	var pe *permanentError
	assert.True(t, errors.As(err, &pe), "invalid image ref must be a permanent error")
}

func TestResolveImageDigest_AllowlistDenial_ReturnsPermanentError(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, []string{"quay.io"})
	_, _, err := fetcher.resolveImageDigest(context.Background(), "evil.io/img:latest")
	require.Error(t, err)

	var pe *permanentError
	assert.True(t, errors.As(err, &pe), "allowlist denial must be a permanent error")
}

func TestExtractManifestFromImage_NotFound_ReturnsPermanentError(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	layer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, "app/other-file.json", []byte("data"))),
	}
	img := &mockContainerImage{layers: []oci.Layer{layer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err)

	var pe *permanentError
	assert.True(t, errors.As(err, &pe), "file-not-found must be a permanent error")
}

func TestExtractManifestFromImage_Whiteout_ReturnsPermanentError(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	olderLayer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, managedPipelinesJSONPath, []byte(`[{"name":"old"}]`))),
	}
	whiteoutLayer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, "app/.wh.managed-pipelines.json", nil)),
	}
	img := &mockContainerImage{layers: []oci.Layer{olderLayer, whiteoutLayer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err)

	var pe *permanentError
	assert.True(t, errors.As(err, &pe), "whiteout must be a permanent error")
}

func TestExtractManifestFromImage_LayerOpenError_NotPermanent(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	failLayer := &mockContainerLayer{err: fmt.Errorf("layer read failure")}
	img := &mockContainerImage{layers: []oci.Layer{failLayer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err)

	var pe *permanentError
	assert.False(t, errors.As(err, &pe), "layer I/O error must NOT be a permanent error")
}

func TestExtractManifestFromImage_TarScanError_NotPermanent(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	corruptReader := io.NopCloser(bytes.NewReader([]byte("not a tar")))
	layer := &mockContainerLayer{reader: corruptReader}
	img := &mockContainerImage{layers: []oci.Layer{layer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err)

	var pe *permanentError
	assert.False(t, errors.As(err, &pe), "tar scan error must NOT be a permanent error")
}

func TestExtractFileFromTar_OversizedEntry(t *testing.T) {
	_, err := extractFileFromTar(
		newTarWithEntry(t, "app/managed-pipelines.json", make([]byte, maxManagedPipelinesManifestSize+1)),
		"app/managed-pipelines.json",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limit")
}

func TestExtractFileFromTar_ExactLimit(t *testing.T) {
	payload := make([]byte, maxManagedPipelinesManifestSize)
	data, err := extractFileFromTar(
		newTarWithEntry(t, "app/managed-pipelines.json", payload),
		"app/managed-pipelines.json",
	)
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Len(t, data, int(maxManagedPipelinesManifestSize))
}

func TestObservedGeneration_SetOnNewConditions(t *testing.T) {
	dspa := testutil.CreateEmptyDSPA()
	dspa.Generation = 42
	dspa.Status.Conditions = nil // no previous conditions at all
	status := dspastatus.NewDSPAStatus(dspa)
	status.SetManagedPipelineValid()

	conditions := status.GetConditions()
	for _, c := range conditions {
		assert.Equal(t, int64(42), c.ObservedGeneration, "ObservedGeneration must be set for condition %s", c.Type)
	}
}

func TestRegistryAllowlist_BlocksDisallowedRegistry(t *testing.T) {
	dspa := testutil.CreateDSPAWithManagedPipelines("evil.registry.io/img:latest", []dspav1.ManagedPipeline{{Name: "p1"}}, nil)
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)

	fetcher := NewOCIManifestFetcher(ctrl.Log, []string{"quay.io", "registry.redhat.io"})
	reconciler := &DSPAReconciler{Log: ctrl.Log, ManifestFetcher: fetcher}

	proceed, err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	require.NoError(t, err, "permanent errors should not trigger controller-runtime retries")
	require.False(t, proceed, "allowlist denial is permanent; must block API server deployment")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelineInvalid, cond.Reason)
	assert.Contains(t, cond.Message, "not in the allowed list")
}

func TestRegistryAllowlist_EmptyAllowsAll(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	assert.Empty(t, fetcher.AllowedRegistries)
}

func TestIsRegistryAllowed(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, []string{"quay.io", "registry.redhat.io"})
	assert.True(t, fetcher.isRegistryAllowed("quay.io"))
	assert.True(t, fetcher.isRegistryAllowed("QUAY.IO"))
	assert.True(t, fetcher.isRegistryAllowed("registry.redhat.io"))
	assert.False(t, fetcher.isRegistryAllowed("evil.io"))
	assert.False(t, fetcher.isRegistryAllowed("docker.io"))
}

func TestCopyStringBoolMap(t *testing.T) {
	original := map[string]bool{"a": true, "b": true}
	cp := copyStringBoolMap(original)

	assert.Equal(t, original, cp)

	cp["c"] = true
	delete(cp, "a")
	assert.True(t, original["a"], "mutation of copy must not affect original")
	assert.False(t, original["c"], "mutation of copy must not affect original")
}

func TestCopyStringBoolMap_Nil(t *testing.T) {
	cp := copyStringBoolMap(nil)
	assert.NotNil(t, cp)
	assert.Empty(t, cp)
}

func TestIsWhiteoutFor(t *testing.T) {
	target := "app/managed-pipelines.json"

	assert.True(t, isWhiteoutFor("app/.wh.managed-pipelines.json", target))
	assert.True(t, isWhiteoutFor("app/.wh..wh..opq", target))
	assert.False(t, isWhiteoutFor("app/managed-pipelines.json", target))
	assert.False(t, isWhiteoutFor("app/other-file.json", target))
	assert.False(t, isWhiteoutFor("other/.wh.managed-pipelines.json", target))
}

func TestExtractFileFromTar_Whiteout_ReturnsErrWhiteout(t *testing.T) {
	tarBuf := newTarWithEntry(t, "app/.wh.managed-pipelines.json", nil)

	data, err := extractFileFromTar(tarBuf, managedPipelinesJSONPath)
	assert.ErrorIs(t, err, errWhiteout)
	assert.Nil(t, data)
}

func TestExtractFileFromTar_OpaqueWhiteout_ReturnsErrWhiteout(t *testing.T) {
	tarBuf := newTarWithEntry(t, "app/.wh..wh..opq", nil)

	data, err := extractFileFromTar(tarBuf, managedPipelinesJSONPath)
	assert.ErrorIs(t, err, errWhiteout)
	assert.Nil(t, data)
}

func TestExtractFileFromTar_WhiteoutBeforeFile_ReturnsFile(t *testing.T) {
	content := []byte(`[{"name":"replacement"}]`)
	tarBuf := newTarWithEntries(t,
		tarEntry{name: "app/.wh.managed-pipelines.json", content: nil},
		tarEntry{name: managedPipelinesJSONPath, content: content},
	)

	data, err := extractFileFromTar(tarBuf, managedPipelinesJSONPath)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestExtractFileFromTar_FileBeforeWhiteout_ReturnsFile(t *testing.T) {
	content := []byte(`[{"name":"replacement"}]`)
	tarBuf := newTarWithEntries(t,
		tarEntry{name: managedPipelinesJSONPath, content: content},
		tarEntry{name: "app/.wh.managed-pipelines.json", content: nil},
	)

	data, err := extractFileFromTar(tarBuf, managedPipelinesJSONPath)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestExtractFileFromTar_OpaqueWhiteoutBeforeFile_ReturnsFile(t *testing.T) {
	content := []byte(`[{"name":"replacement"}]`)
	tarBuf := newTarWithEntries(t,
		tarEntry{name: "app/.wh..wh..opq", content: nil},
		tarEntry{name: managedPipelinesJSONPath, content: content},
	)

	data, err := extractFileFromTar(tarBuf, managedPipelinesJSONPath)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestExtractFileFromTar_Found(t *testing.T) {
	content := []byte(`[{"name":"test"}]`)
	tarBuf := newTarWithEntry(t, managedPipelinesJSONPath, content)

	data, err := extractFileFromTar(tarBuf, managedPipelinesJSONPath)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestExtractFileFromTar_NotFound(t *testing.T) {
	tarBuf := newTarWithEntry(t, "app/other-file.json", []byte("data"))

	data, err := extractFileFromTar(tarBuf, managedPipelinesJSONPath)
	assert.ErrorIs(t, err, errNotFound)
	assert.Nil(t, data)
}

func TestExtractManifestFromImage_WhiteoutInUpperLayer_StopsSearch(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)

	validContent := []byte(`[{"name":"old-pipeline"}]`)
	olderLayer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, managedPipelinesJSONPath, validContent)),
	}
	whiteoutLayer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, "app/.wh.managed-pipelines.json", nil)),
	}
	img := &mockContainerImage{layers: []oci.Layer{olderLayer, whiteoutLayer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err, "should NOT fall through to lower layer with deleted content")
	assert.Contains(t, err.Error(), "not found")
}

func TestExtractManifestFromImage_WhiteoutAndReplacementInSameLayer(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)

	replacement := []byte(`[{"name":"new-pipeline"}]`)
	layer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntries(t,
			tarEntry{name: "app/.wh.managed-pipelines.json", content: nil},
			tarEntry{name: managedPipelinesJSONPath, content: replacement},
		)),
	}
	img := &mockContainerImage{layers: []oci.Layer{layer}}

	data, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.NoError(t, err)
	assert.Equal(t, replacement, data)
}

func TestExtractManifestFromImage_OpaqueWhiteoutInUpperLayer_StopsSearch(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)

	validContent := []byte(`[{"name":"old-pipeline"}]`)
	olderLayer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, managedPipelinesJSONPath, validContent)),
	}
	opaqueLayer := &mockContainerLayer{
		reader: io.NopCloser(newTarWithEntry(t, "app/.wh..wh..opq", nil)),
	}
	img := &mockContainerImage{layers: []oci.Layer{olderLayer, opaqueLayer}}

	_, err := fetcher.extractManifestFromImage(img, "test-image:latest")
	require.Error(t, err, "should NOT fall through when directory is opaque")
	assert.Contains(t, err.Error(), "not found")
}

// --- Cache TTL tests ---

func TestGetCached_HitWithinTTL(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	now := time.Now()
	fetcher.nowFunc = func() time.Time { return now }
	fetcher.putCache("sha256:abc123", map[string]bool{"pipeline_a": true})

	fetcher.nowFunc = func() time.Time { return now.Add(cacheTTL - time.Minute) }
	cached := fetcher.getCached("sha256:abc123")
	require.NotNil(t, cached, "entry within TTL must be returned")
	assert.Equal(t, map[string]bool{"pipeline_a": true}, cached)
}

func TestGetCached_MissAfterExpiry(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	now := time.Now()
	fetcher.nowFunc = func() time.Time { return now }
	fetcher.putCache("sha256:abc123", map[string]bool{"pipeline_a": true})

	fetcher.nowFunc = func() time.Time { return now.Add(cacheTTL + time.Minute) }
	cached := fetcher.getCached("sha256:abc123")
	assert.Nil(t, cached, "entry past TTL must be treated as miss")
}

func TestGetCached_ExpiredEntryEvicted(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	now := time.Now()
	fetcher.nowFunc = func() time.Time { return now }
	fetcher.putCache("sha256:abc123", map[string]bool{"pipeline_a": true})

	fetcher.nowFunc = func() time.Time { return now.Add(cacheTTL + time.Minute) }
	_ = fetcher.getCached("sha256:abc123")

	fetcher.mu.Lock()
	_, exists := fetcher.cache["sha256:abc123"]
	fetcher.mu.Unlock()
	assert.False(t, exists, "expired entry must be evicted from cache map")
}

func TestGetCached_UnknownDigest(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	cached := fetcher.getCached("sha256:unknown")
	assert.Nil(t, cached)
}

func TestGetCached_ExactTTLBoundary(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	now := time.Now()
	fetcher.nowFunc = func() time.Time { return now }
	fetcher.putCache("sha256:abc123", map[string]bool{"pipeline_a": true})

	fetcher.nowFunc = func() time.Time { return now.Add(cacheTTL) }
	cached := fetcher.getCached("sha256:abc123")
	require.NotNil(t, cached, "entry at exactly TTL must still be valid")
	assert.Equal(t, map[string]bool{"pipeline_a": true}, cached)
}

func TestPutCache_OverwriteRefreshesTTL(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	now := time.Now()
	fetcher.nowFunc = func() time.Time { return now }
	fetcher.putCache("sha256:abc123", map[string]bool{"pipeline_a": true})

	refresh := now.Add(cacheTTL - time.Minute)
	fetcher.nowFunc = func() time.Time { return refresh }
	fetcher.putCache("sha256:abc123", map[string]bool{"pipeline_a": true, "pipeline_b": true})

	fetcher.nowFunc = func() time.Time { return refresh.Add(cacheTTL) }
	cached := fetcher.getCached("sha256:abc123")
	require.NotNil(t, cached, "overwritten entry should use refreshed timestamp")
	assert.Equal(t, map[string]bool{"pipeline_a": true, "pipeline_b": true}, cached)
}

func TestGetCached_DefensiveCopy(t *testing.T) {
	fetcher := NewOCIManifestFetcher(ctrl.Log, nil)
	fetcher.putCache("sha256:abc123", map[string]bool{"pipeline_a": true})

	cached := fetcher.getCached("sha256:abc123")
	cached["mutated"] = true

	cached2 := fetcher.getCached("sha256:abc123")
	assert.NotContains(t, cached2, "mutated", "getCached must return defensive copies")
}

type tarEntry struct {
	name    string
	content []byte
}

func newTarWithEntries(t testing.TB, entries ...tarEntry) io.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: e.name, Size: int64(len(e.content))}))
		if len(e.content) > 0 {
			_, err := tw.Write(e.content)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return &buf
}

func newTarWithEntry(t testing.TB, name string, content []byte) io.Reader {
	t.Helper()
	return newTarWithEntries(t, tarEntry{name: name, content: content})
}

// findCondition returns the condition with the given type, or nil if not found.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
