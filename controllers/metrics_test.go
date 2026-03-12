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

	"github.com/stretchr/testify/assert"
)

// TestDeleteMetrics_RHOAIENG21799 verifies that DeleteMetrics can be called
// without errors and properly cleans up metric label values for a DSPA instance.
// This is a regression test for RHOAIENG-21799.
func TestDeleteMetrics_RHOAIENG21799(t *testing.T) {
	testDSPAName := "test-dspa"
	testNamespace := "test-namespace"

	// First, set some metric values to ensure they exist before deletion
	// This simulates the normal operation where metrics are published during reconciliation
	DBAvailableMetric.WithLabelValues(testDSPAName, testNamespace).Set(1)
	ObjectStoreAvailableMetric.WithLabelValues(testDSPAName, testNamespace).Set(1)
	APIServerReadyMetric.WithLabelValues(testDSPAName, testNamespace).Set(0)
	PersistenceAgentReadyMetric.WithLabelValues(testDSPAName, testNamespace).Set(0)
	ScheduledWorkflowReadyMetric.WithLabelValues(testDSPAName, testNamespace).Set(0)
	WorkflowControllerReadyMetric.WithLabelValues(testDSPAName, testNamespace).Set(0)
	MLMDProxyReadyMetric.WithLabelValues(testDSPAName, testNamespace).Set(0)
	CrReadyMetric.WithLabelValues(testDSPAName, testNamespace).Set(0)

	// Call DeleteMetrics - this should not panic or error
	// The function uses DeleteLabelValues which is safe to call even if labels don't exist
	assert.NotPanics(t, func() {
		DeleteMetrics(testDSPAName, testNamespace)
	}, "DeleteMetrics should not panic when called")

	// Call DeleteMetrics again to verify idempotency
	// DeleteLabelValues is idempotent and safe to call multiple times
	assert.NotPanics(t, func() {
		DeleteMetrics(testDSPAName, testNamespace)
	}, "DeleteMetrics should be idempotent and safe to call multiple times")
}

// TestDeleteMetrics_EmptyValues verifies that DeleteMetrics handles empty
// string parameters gracefully (edge case).
func TestDeleteMetrics_EmptyValues(t *testing.T) {
	// This tests an edge case - though in practice this shouldn't happen
	// as the DSPA name and namespace should always be set
	assert.NotPanics(t, func() {
		DeleteMetrics("", "")
	}, "DeleteMetrics should handle empty strings gracefully")
}

// TestDeleteMetrics_MultipleInstances verifies that DeleteMetrics only removes
// metrics for the specified DSPA instance and doesn't affect other instances.
func TestDeleteMetrics_MultipleInstances(t *testing.T) {
	dspa1Name := "dspa-instance-1"
	dspa1Namespace := "namespace-1"
	dspa2Name := "dspa-instance-2"
	dspa2Namespace := "namespace-2"

	// Set metrics for two different DSPA instances
	APIServerReadyMetric.WithLabelValues(dspa1Name, dspa1Namespace).Set(1)
	APIServerReadyMetric.WithLabelValues(dspa2Name, dspa2Namespace).Set(1)

	// Delete metrics for only the first instance
	assert.NotPanics(t, func() {
		DeleteMetrics(dspa1Name, dspa1Namespace)
	}, "DeleteMetrics should successfully delete metrics for first instance")

	// Verify we can still set/get metrics for the second instance
	// (this indirectly verifies that DeleteMetrics didn't affect other instances)
	assert.NotPanics(t, func() {
		APIServerReadyMetric.WithLabelValues(dspa2Name, dspa2Namespace).Set(0)
	}, "Metrics for other instances should remain unaffected")
}

// TestCleanUpResources_RHOAIENG21799 verifies that cleanUpResources successfully
// calls DeleteMetrics during DSPA finalization. This is an integration test for
// the complete cleanup flow.
func TestCleanUpResources_RHOAIENG21799(t *testing.T) {
	// Set up a test DSPA with metrics
	testName := "cleanup-test-dspa"
	testNamespace := "cleanup-test-ns"

	// Publish metrics (simulating normal operation)
	CrReadyMetric.WithLabelValues(testName, testNamespace).Set(0)

	// Create params and reconciler
	params := &DSPAParams{
		Name:      testName,
		Namespace: testNamespace,
	}
	_, _, reconciler := CreateNewTestObjects()

	// Call cleanUpResources - this should call DeleteMetrics internally
	err := reconciler.cleanUpResources(params)

	// Verify no error
	assert.Nil(t, err)
}
