//go:build test_all || test_unit

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

package controllers

import (
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// resetGlobalDSPAMetricsForTests clears all label sets on shared Prometheus gauges.
// Earlier tests in this package call Reconcile/PublishMetrics and leave time series in
// the process-global registry; tests that use CollectAndCount must start from a clean slate.
func resetGlobalDSPAMetricsForTests(t *testing.T) {
	t.Helper()
	for _, m := range allDSPAMetrics {
		m.Reset()
	}
	ManagedPipelineValidMetric.Reset()
}

// TestDeleteMetrics verifies that DeleteMetrics removes all metric time series
// for a given DSPA instance from the registry.
func TestDeleteMetrics(t *testing.T) {
	resetGlobalDSPAMetricsForTests(t)

	testDSPAName := "test-dspa"
	testNamespace := "test-namespace"

	// Set metric values to simulate normal reconciliation
	for _, m := range allDSPAMetrics {
		m.WithLabelValues(testDSPAName, testNamespace).Set(1)
	}

	// Verify metrics exist before deletion
	for _, m := range allDSPAMetrics {
		assert.Equal(t, 1, testutil.CollectAndCount(m),
			"metric should have exactly one time series before deletion")
	}

	// Delete metrics for the instance
	DeleteMetrics(testDSPAName, testNamespace)

	// Verify all metrics were actually removed from the registry
	for _, m := range allDSPAMetrics {
		assert.Equal(t, 0, testutil.CollectAndCount(m),
			"metric should have zero time series after deletion")
	}

	// Verify idempotency: calling DeleteMetrics again should not panic or error
	require.NotPanics(t, func() {
		DeleteMetrics(testDSPAName, testNamespace)
	}, "DeleteMetrics should be idempotent")
}

// TestDeleteMetrics_EmptyValues verifies that DeleteMetrics handles empty
// string parameters gracefully.
func TestDeleteMetrics_EmptyValues(t *testing.T) {
	require.NotPanics(t, func() {
		DeleteMetrics("", "")
	}, "DeleteMetrics should handle empty strings gracefully")
}

// TestDeleteMetrics_MultipleInstances verifies that DeleteMetrics only removes
// metrics for the specified DSPA instance and preserves other instances.
func TestDeleteMetrics_MultipleInstances(t *testing.T) {
	resetGlobalDSPAMetricsForTests(t)

	dspa1Name := "dspa-instance-1"
	dspa1Namespace := "namespace-1"
	dspa2Name := "dspa-instance-2"
	dspa2Namespace := "namespace-2"

	// Set metrics for two different DSPA instances
	APIServerReadyMetric.WithLabelValues(dspa1Name, dspa1Namespace).Set(1)
	APIServerReadyMetric.WithLabelValues(dspa2Name, dspa2Namespace).Set(1)

	// Delete metrics for only the first instance
	DeleteMetrics(dspa1Name, dspa1Namespace)

	// Verify instance-2's metric value is preserved at 1.0
	metric := APIServerReadyMetric.WithLabelValues(dspa2Name, dspa2Namespace)
	value := testutil.ToFloat64(metric)
	assert.Equal(t, 1.0, value,
		"instance-2 metric value should be preserved after deleting instance-1")

	// Verify instance-1's metric is gone (only instance-2 remains)
	assert.Equal(t, 1, testutil.CollectAndCount(APIServerReadyMetric),
		"only instance-2 time series should remain")

	// Clean up instance-2
	DeleteMetrics(dspa2Name, dspa2Namespace)
}

// TestCleanUpResources_DeletesMetrics verifies that cleanUpResources calls
// DeleteMetrics during DSPA finalization by checking that metrics are removed.
func TestCleanUpResources_DeletesMetrics(t *testing.T) {
	resetGlobalDSPAMetricsForTests(t)

	testName := "cleanup-test-dspa"
	testNamespace := "cleanup-test-ns"

	// Publish metrics (simulating normal operation)
	CrReadyMetric.WithLabelValues(testName, testNamespace).Set(0)
	DBAvailableMetric.WithLabelValues(testName, testNamespace).Set(1)

	// Verify metrics exist before cleanup
	assert.Equal(t, float64(0), testutil.ToFloat64(CrReadyMetric.WithLabelValues(testName, testNamespace)))
	assert.Equal(t, float64(1), testutil.ToFloat64(DBAvailableMetric.WithLabelValues(testName, testNamespace)))
	setManagedPipelineValidMetricByReason(testName, testNamespace, "NotApplicable")
	assert.Equal(
		t,
		float64(1),
		testutil.ToFloat64(ManagedPipelineValidMetric.WithLabelValues(testName, testNamespace, "NotApplicable")),
	)

	params := &DSPAParams{
		Name:      testName,
		Namespace: testNamespace,
	}
	_, _, reconciler := CreateNewTestObjects()

	err := reconciler.cleanUpResources(params)
	require.NoError(t, err)

	// Verify metrics were actually deleted by cleanUpResources
	assert.Equal(t, 0, testutil.CollectAndCount(CrReadyMetric),
		"CrReadyMetric should be removed after cleanUpResources")
	assert.Equal(t, 0, testutil.CollectAndCount(DBAvailableMetric),
		"DBAvailableMetric should be removed after cleanUpResources")
	assert.Equal(t, 0, testutil.CollectAndCount(ManagedPipelineValidMetric),
		"ManagedPipelineValidMetric should be removed after cleanUpResources")
}

func TestSetManagedPipelineValidMetricByReason_OneHotByReason(t *testing.T) {
	testName := "state-test-dspa"
	testNamespace := "state-test-ns"

	setManagedPipelineValidMetricByReason(testName, testNamespace, "NotApplicable")
	assert.Equal(
		t,
		float64(1),
		testutil.ToFloat64(ManagedPipelineValidMetric.WithLabelValues(testName, testNamespace, "NotApplicable")),
	)
	assert.Equal(
		t,
		float64(0),
		testutil.ToFloat64(ManagedPipelineValidMetric.WithLabelValues(testName, testNamespace, config.ManagedPipelineInvalid)),
	)

	setManagedPipelineValidMetricByReason(testName, testNamespace, config.ManagedPipelinesFetchError)
	assert.Equal(
		t,
		float64(1),
		testutil.ToFloat64(ManagedPipelineValidMetric.WithLabelValues(testName, testNamespace, config.ManagedPipelinesFetchError)),
	)
	assert.Equal(
		t,
		float64(0),
		testutil.ToFloat64(ManagedPipelineValidMetric.WithLabelValues(testName, testNamespace, "NotApplicable")),
	)

	DeleteMetrics(testName, testNamespace)
}

func TestPublishMetrics_PublishesManagedPipelineValidationReason(t *testing.T) {
	dspaName := "publish-metric-dspa"
	dspaNamespace := "publish-metric-ns"
	dspa := &dspav1.DataSciencePipelinesApplication{}
	dspa.Name = dspaName
	dspa.Namespace = dspaNamespace
	_, _, reconciler := CreateNewTestObjects()

	cond := metav1.Condition{
		Type:   config.ManagedPipelineValid,
		Status: metav1.ConditionFalse,
		Reason: "NotApplicable",
	}
	metricsMap := map[metav1.Condition]*prometheus.GaugeVec{
		cond: ManagedPipelineValidMetric,
	}

	reconciler.PublishMetrics(dspa, metricsMap)

	assert.Equal(t, float64(1), testutil.ToFloat64(ManagedPipelineValidMetric.WithLabelValues(dspaName, dspaNamespace, "NotApplicable")))
	assert.Equal(t, float64(0), testutil.ToFloat64(ManagedPipelineValidMetric.WithLabelValues(dspaName, dspaNamespace, config.ManagedPipelineInvalid)))
	DeleteMetrics(dspaName, dspaNamespace)
}
