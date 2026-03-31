//go:build test_all || test_unit

package controllers

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/dspastatus"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

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

	err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	assert.NoError(t, err, "fetch errors should not block reconciliation")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelinesFetchError, cond.Reason)
	assert.Contains(t, cond.Message, "connection refused")
}

func TestValidateManagedPipelines_ValidationError_BlocksReconcile(t *testing.T) {
	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", []dspav1.ManagedPipeline{{Name: "bad"}}, nil)
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	reconciler := &DSPAReconciler{
		Log:             ctrl.Log,
		ManifestFetcher: &mockPipelineNamesFetcher{names: map[string]bool{"good": true}},
	}

	err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	require.Error(t, err, "validation errors should block reconciliation")
	assert.Contains(t, err.Error(), "bad")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelineInvalid, cond.Reason)
}

func TestValidateManagedPipelines_AllValid_SetsCondition(t *testing.T) {
	dspa := testutil.CreateDSPAWithManagedPipelines("img:latest", []dspav1.ManagedPipeline{{Name: "p1"}}, nil)
	dspa.Status.Conditions = make([]metav1.Condition, 11)
	status := dspastatus.NewDSPAStatus(dspa)
	reconciler := &DSPAReconciler{
		Log:             ctrl.Log,
		ManifestFetcher: &mockPipelineNamesFetcher{names: map[string]bool{"p1": true}},
	}

	err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	assert.NoError(t, err)

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

			err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
			assert.NoError(t, err)

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

	err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	assert.NoError(t, err, "nil fetcher should not panic or block reconciliation")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelinesFetchError, cond.Reason)
}

func TestExtractFileFromTar_OversizedEntry(t *testing.T) {
	_, _, err := extractFileFromTar(
		newTarWithEntry("app/managed-pipelines.json", make([]byte, maxManagedPipelinesManifestSize+1)),
		"app/managed-pipelines.json",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limit")
}

func TestExtractFileFromTar_ExactLimit(t *testing.T) {
	payload := make([]byte, maxManagedPipelinesManifestSize)
	data, found, err := extractFileFromTar(
		newTarWithEntry("app/managed-pipelines.json", payload),
		"app/managed-pipelines.json",
	)
	require.NoError(t, err)
	require.True(t, found)
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

	err := reconciler.validateManagedPipelines(context.Background(), dspa, status, ctrl.Log)
	assert.NoError(t, err, "fetch errors should not block reconciliation")

	conditions := status.GetConditions()
	cond := findCondition(conditions, config.ManagedPipelineValid)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, config.ManagedPipelinesFetchError, cond.Reason)
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

func newTarWithEntry(name string, content []byte) io.Reader {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(content))})
	_, _ = tw.Write(content)
	_ = tw.Close()
	return &buf
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
