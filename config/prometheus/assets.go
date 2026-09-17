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

// RuleName preserves the resource name produced by the standalone Kustomize
// installation so modular deployments can adopt it without replacement.
const RuleName = "data-science-pipelines-operator-datasciencepipelines-prometheusrules"

// Rule returns a fresh PrometheusRule configured for the module namespace.
func Rule(namespace string) (*unstructured.Unstructured, error) {
	object := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(prometheusRuleYAML, &object.Object); err != nil {
		return nil, fmt.Errorf("decode embedded PrometheusRule: %w", err)
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
