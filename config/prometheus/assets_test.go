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

package prometheus_test

import (
	"testing"

	monitoringassets "github.com/opendatahub-io/data-science-pipelines-operator/config/prometheus"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRuleUsesModuleNamespace(t *testing.T) {
	rule, err := monitoringassets.Rule("opendatahub")
	require.NoError(t, err)
	require.Equal(t, "monitoring.rhobs/v1", rule.GetAPIVersion())
	require.Equal(t, "PrometheusRule", rule.GetKind())
	require.Equal(t, monitoringassets.RuleName, rule.GetName())
	require.Equal(t, "opendatahub", rule.GetNamespace())

	groups, found, err := unstructured.NestedSlice(rule.Object, "spec", "groups")
	require.NoError(t, err)
	require.True(t, found)
	for _, group := range groups {
		for _, item := range group.(map[string]interface{})["rules"].([]interface{}) {
			labels := item.(map[string]interface{})["labels"].(map[string]interface{})
			require.Equal(t, "opendatahub", labels["namespace"])
		}
	}
}
