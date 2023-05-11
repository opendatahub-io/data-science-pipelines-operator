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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Prometheus metrics gauges
var (
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
)

// InitMetrics initialize prometheus metrics
func InitMetrics() {
	metrics.Registry.MustRegister(APIServerReadyMetric,
		PersistenceAgentReadyMetric,
		ScheduledWorkflowReadyMetric,
		CrReadyMetric)
}
