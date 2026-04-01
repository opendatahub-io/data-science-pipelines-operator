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
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Prometheus metrics gauges
var (
	DBAvailableMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_database_available",
			Help: "Data Science Pipelines Application - Database Availability Status",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)
	ObjectStoreAvailableMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_object_store_available",
			Help: "Data Science Pipelines Application - Object Store Availability Status",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)
	APIServerReadyMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_apiserver_ready",
			Help: "Data Science Pipelines Application - APIServer Ready Status",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)
	PersistenceAgentReadyMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_persistenceagent_ready",
			Help: "Data Science Pipelines Application - PersistenceAgent Ready Status",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)
	ScheduledWorkflowReadyMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_scheduledworkflow_ready",
			Help: "Data Science Pipelines Application - ScheduledWorkflow Ready Status",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)
	WorkflowControllerReadyMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_workflowcontroller_ready",
			Help: "Data Science Pipelines Application - WorkflowController Ready Status",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)
	MLMDProxyReadyMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_mlmdproxy_ready",
			Help: "Data Science Pipelines Application - MLMD Proxy Ready Status",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)
	ManagedPipelineValidMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_managed_pipeline_valid",
			Help: "Data Science Pipelines Application - Managed Pipeline Validation Status (1=true, 0=all other states)",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)
	ManagedPipelineValidationStateMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_managed_pipeline_validation_state",
			Help: "Data Science Pipelines Application - Managed Pipeline validation state by reason (one-hot: active reason=1, others=0)",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
			"reason",
		},
	)
	CrReadyMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "data_science_pipelines_application_ready",
			Help: "Data Science Pipelines Application - CustomResource Ready Status",
		},
		[]string{
			"dspa_name",
			"dspa_namespace",
		},
	)

	// allDSPAMetrics is the single source of truth for all DSPA metric gauges.
	// Both InitMetrics and DeleteMetrics use this slice, so adding a new metric
	// here automatically includes it in registration and cleanup.
	allDSPAMetrics = []*prometheus.GaugeVec{
		DBAvailableMetric,
		ObjectStoreAvailableMetric,
		APIServerReadyMetric,
		PersistenceAgentReadyMetric,
		ScheduledWorkflowReadyMetric,
		WorkflowControllerReadyMetric,
		MLMDProxyReadyMetric,
		ManagedPipelineValidMetric,
		CrReadyMetric,
	}

	managedPipelineValidationReasons = []string{
		config.ManagedPipelineValid,
		config.ManagedPipelineInvalid,
		config.ManagedPipelinesFetchError,
		"NotApplicable",
		"Unknown",
		"Other",
	}
)

// InitMetrics registers all DSPA prometheus metrics.
func InitMetrics() {
	for _, m := range allDSPAMetrics {
		metrics.Registry.MustRegister(m)
	}
	metrics.Registry.MustRegister(ManagedPipelineValidationStateMetric)
}

// DeleteMetrics removes all metric label values for a specific DSPA instance.
// This is called during DSPA finalization to prevent stale metrics from
// persisting in Prometheus after the DSPA has been deleted.
func DeleteMetrics(dspaName, dspaNamespace string) {
	for _, m := range allDSPAMetrics {
		m.DeleteLabelValues(dspaName, dspaNamespace)
	}
	for _, reason := range managedPipelineValidationReasons {
		ManagedPipelineValidationStateMetric.DeleteLabelValues(dspaName, dspaNamespace, reason)
	}
}

func setManagedPipelineValidationStateMetric(dspaName, dspaNamespace, reason string) {
	activeReason := normalizeManagedPipelineValidationReason(reason)
	for _, r := range managedPipelineValidationReasons {
		value := 0.0
		if r == activeReason {
			value = 1
		}
		ManagedPipelineValidationStateMetric.WithLabelValues(dspaName, dspaNamespace, r).Set(value)
	}
}

func normalizeManagedPipelineValidationReason(reason string) string {
	for _, r := range managedPipelineValidationReasons {
		if reason == r && r != "Other" {
			return reason
		}
	}
	return "Other"
}
