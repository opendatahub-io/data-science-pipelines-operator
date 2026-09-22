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

// Package prometheus embeds monitoring resources owned by the AIPipelines
// module controller.
package prometheus

import (
	_ "embed"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

//go:embed datasciencepipelines-prometheusrules.yaml
var prometheusRuleYAML []byte

//go:embed monitor/monitor.yaml
var coreServiceMonitorYAML []byte

//go:embed rhoai/rhoai_metrics_service_monitor.yaml
var rhoaiServiceMonitorYAML []byte

//go:embed rhoai/rhoai_metrics_reader_role_binding.yaml
var rhoaiMetricsReaderRoleBindingYAML []byte

// RuleName preserves the resource name produced by the standalone Kustomize
// installation so modular deployments can adopt it without replacement.
const RuleName = "data-science-pipelines-operator-datasciencepipelines-prometheusrules"

// CoreServiceMonitorName is the identity used by OpenShift user-workload monitoring.
const CoreServiceMonitorName = "data-science-pipelines-operator-service-monitor"

// RHOAIServiceMonitorName is the identity used by Cluster Observability Operator.
const RHOAIServiceMonitorName = "data-science-pipelines-operator-rhoai-service-monitor"

// RHOAIMetricsReaderRoleBindingName grants the COO monitoring stack access to metrics.
const RHOAIMetricsReaderRoleBindingName = "data-science-pipelines-operator-rhoai-prometheus"

func decodeAsset(data []byte, description string) (*unstructured.Unstructured, error) {
	object := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(data, &object.Object); err != nil {
		return nil, fmt.Errorf("decode embedded %s: %w", description, err)
	}
	return object, nil
}

// Rule returns a fresh PrometheusRule configured for the module namespace.
func Rule(namespace string) (*unstructured.Unstructured, error) {
	object, err := decodeAsset(prometheusRuleYAML, "PrometheusRule")
	if err != nil {
		return nil, err
	}
	// The installation kustomization historically applied this prefix. Keep the
	// same identity so modular deployments adopt the existing rule in place.
	object.SetName(RuleName)
	object.SetNamespace(namespace)

	groups, found, err := unstructured.NestedSlice(object.Object, "spec", "groups")
	if err != nil {
		return nil, fmt.Errorf("read PrometheusRule groups: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("embedded PrometheusRule has no groups")
	}
	for _, group := range groups {
		groupMap, ok := group.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("embedded PrometheusRule contains an invalid group")
		}
		rules, ok := groupMap["rules"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("embedded PrometheusRule group contains invalid rules")
		}
		for _, rule := range rules {
			ruleMap, ok := rule.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("embedded PrometheusRule contains an invalid rule")
			}
			labels, ok := ruleMap["labels"].(map[string]interface{})
			if !ok {
				labels = map[string]interface{}{}
				ruleMap["labels"] = labels
			}
			labels["namespace"] = namespace
		}
	}
	if err := unstructured.SetNestedSlice(object.Object, groups, "spec", "groups"); err != nil {
		return nil, fmt.Errorf("set PrometheusRule groups: %w", err)
	}

	return object, nil
}

// CoreServiceMonitor returns the OpenShift in-cluster monitoring object. The
// platform-facing overlay intentionally excludes it; the module controller
// owns it and creates it only when the API is available.
func CoreServiceMonitor(namespace string) (*unstructured.Unstructured, error) {
	object, err := decodeAsset(coreServiceMonitorYAML, "core ServiceMonitor")
	if err != nil {
		return nil, err
	}
	object.SetName(CoreServiceMonitorName)
	object.SetNamespace(namespace)
	endpoint := map[string]interface{}{
		"path":            "/metrics",
		"port":            "https",
		"scheme":          "https",
		"bearerTokenFile": "/var/run/secrets/kubernetes.io/serviceaccount/token",
		"tlsConfig": map[string]interface{}{
			"ca": map[string]interface{}{
				"configMap": map[string]interface{}{
					"name": "openshift-service-ca.crt",
					"key":  "service-ca.crt",
				},
			},
			"serverName": "data-science-pipelines-operator-service." + namespace + ".svc",
		},
	}
	if err := unstructured.SetNestedSlice(object.Object, []interface{}{endpoint}, "spec", "endpoints"); err != nil {
		return nil, fmt.Errorf("configure core ServiceMonitor endpoint: %w", err)
	}
	return object, nil
}

// RHOAIServiceMonitor returns the COO monitoring object for the platform namespaces.
func RHOAIServiceMonitor(applicationsNamespace, monitoringNamespace string) (*unstructured.Unstructured, error) {
	object, err := decodeAsset(rhoaiServiceMonitorYAML, "RHOAI ServiceMonitor")
	if err != nil {
		return nil, err
	}
	object.SetName(RHOAIServiceMonitorName)
	object.SetNamespace(monitoringNamespace)
	if err := unstructured.SetNestedStringSlice(
		object.Object, []string{applicationsNamespace}, "spec", "namespaceSelector", "matchNames",
	); err != nil {
		return nil, fmt.Errorf("configure RHOAI ServiceMonitor namespace selector: %w", err)
	}
	endpoints, found, err := unstructured.NestedSlice(object.Object, "spec", "endpoints")
	if err != nil {
		return nil, fmt.Errorf("read RHOAI ServiceMonitor endpoints: %w", err)
	}
	if !found || len(endpoints) != 1 {
		return nil, fmt.Errorf("embedded RHOAI ServiceMonitor must have one endpoint")
	}
	endpoint, ok := endpoints[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("embedded RHOAI ServiceMonitor has an invalid endpoint")
	}
	tlsConfig, ok := endpoint["tlsConfig"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("embedded RHOAI ServiceMonitor has an invalid TLS config")
	}
	tlsConfig["serverName"] = "data-science-pipelines-operator-service." + applicationsNamespace + ".svc"
	if err := unstructured.SetNestedSlice(object.Object, endpoints, "spec", "endpoints"); err != nil {
		return nil, fmt.Errorf("configure RHOAI ServiceMonitor endpoints: %w", err)
	}
	return object, nil
}

// RHOAIMetricsReaderRoleBinding returns the COO metrics-reader binding.
func RHOAIMetricsReaderRoleBinding(monitoringNamespace string) (*unstructured.Unstructured, error) {
	object, err := decodeAsset(rhoaiMetricsReaderRoleBindingYAML, "RHOAI metrics reader ClusterRoleBinding")
	if err != nil {
		return nil, err
	}
	object.SetName(RHOAIMetricsReaderRoleBindingName)
	subjects, found, err := unstructured.NestedSlice(object.Object, "subjects")
	if err != nil {
		return nil, fmt.Errorf("read RHOAI metrics reader subjects: %w", err)
	}
	if !found || len(subjects) != 1 {
		return nil, fmt.Errorf("embedded RHOAI metrics reader ClusterRoleBinding must have one subject")
	}
	subject, ok := subjects[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("embedded RHOAI metrics reader ClusterRoleBinding has an invalid subject")
	}
	subject["namespace"] = monitoringNamespace
	if err := unstructured.SetNestedSlice(object.Object, subjects, "subjects"); err != nil {
		return nil, fmt.Errorf("configure RHOAI metrics reader subject: %w", err)
	}
	return object, nil
}
