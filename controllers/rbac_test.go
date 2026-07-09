//go:build test_all || test_unit

/*

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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

// containsWildcard checks if any entry in the slice equals "*".
func containsWildcard(items []string) bool {
	for _, item := range items {
		if item == "*" {
			return true
		}
	}
	return false
}

// checkClusterRoleForWildcards returns true if any rule has a wildcard in verbs,
// resources, or apiGroups. Extracted so both positive and negative tests use the
// same logic.
func checkClusterRoleForWildcards(role *rbacv1.ClusterRole) bool {
	for _, rule := range role.Rules {
		if containsWildcard(rule.Verbs) || containsWildcard(rule.Resources) || containsWildcard(rule.APIGroups) {
			return true
		}
	}
	return false
}

// checkMarkerLineForWildcards returns true if a kubebuilder RBAC marker contains
// verbs=*, resources=*, or groups=*.
func checkMarkerLineForWildcards(line string) bool {
	return strings.Contains(line, "verbs=*") ||
		strings.Contains(line, "resources=*") ||
		strings.Contains(line, "groups=*")
}

// --- Negative tests: verify the detection logic catches wildcards ---

func TestDetectsWildcardVerbs(t *testing.T) {
	role := &rbacv1.ClusterRole{
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
			{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"*"}},
		},
	}
	assert.True(t, checkClusterRoleForWildcards(role), "should detect wildcard in verbs")
}

func TestDetectsWildcardResources(t *testing.T) {
	role := &rbacv1.ClusterRole{
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"kubeflow.org"}, Resources: []string{"*"}, Verbs: []string{"get"}},
		},
	}
	assert.True(t, checkClusterRoleForWildcards(role), "should detect wildcard in resources")
}

func TestDetectsWildcardAPIGroups(t *testing.T) {
	role := &rbacv1.ClusterRole{
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"*"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
		},
	}
	assert.True(t, checkClusterRoleForWildcards(role), "should detect wildcard in apiGroups")
}

func TestPassesCleanRole(t *testing.T) {
	role := &rbacv1.ClusterRole{
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"create", "delete"}},
		},
	}
	assert.False(t, checkClusterRoleForWildcards(role), "should pass a role with no wildcards")
}

func TestDetectsWildcardInMarkerLine(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		expect bool
	}{
		{"verbs wildcard", "//+kubebuilder:rbac:groups=core,resources=pods,verbs=*", true},
		{"resources wildcard", "//+kubebuilder:rbac:groups=kubeflow.org,resources=*,verbs=get", true},
		{"groups wildcard", "//+kubebuilder:rbac:groups=*,resources=deployments,verbs=get", true},
		{"clean marker", "//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch", false},
		{"mixed valid verbs", "//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, checkMarkerLineForWildcards(tt.line))
		})
	}
}

func TestDetectsWildcardInTemplateContent(t *testing.T) {
	withWildcard := `rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ['*']`
	assert.Contains(t, withWildcard, "'*'", "should detect YAML wildcard in template content")

	clean := `rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list"]`
	assert.NotContains(t, clean, "'*'", "should pass clean template content")
}

// --- Positive tests: verify the actual project files are clean ---

// TestNoWildcardsInClusterRoles parses both operator ClusterRoles and verifies
// no rule uses a "*" wildcard.
func TestNoWildcardsInClusterRoles(t *testing.T) {
	clusterRoles := []string{
		"../config/rbac/role.yaml",
		"../config/rbac/argo_role.yaml",
	}
	for _, path := range clusterRoles {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			assert.NoError(t, err, "failed to read %s", path)

			var role rbacv1.ClusterRole
			err = yaml.Unmarshal(data, &role)
			assert.NoError(t, err, "failed to unmarshal %s", path)

			assert.False(t, checkClusterRoleForWildcards(&role),
				"%s contains wildcard '*' in verbs, resources, or apiGroups", path)
		})
	}
}

// TestNoWildcardsInRoleTemplates discovers all Role templates via glob and
// scans them for YAML-style wildcard entries ('*').
func TestNoWildcardsInRoleTemplates(t *testing.T) {
	matches, err := filepath.Glob("../config/internal/**/role*.yaml.tmpl")
	assert.NoError(t, err)
	// Also check nested directories that glob ** doesn't recurse into.
	nested, err := filepath.Glob("../config/internal/*/*/role*.yaml.tmpl")
	assert.NoError(t, err)
	templates := append(matches, nested...)
	assert.NotEmpty(t, templates, "no role templates found — check glob pattern")

	for _, path := range templates {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			assert.NoError(t, err, "failed to read %s", path)

			assert.NotContains(t, string(data), "'*'",
				"%s contains YAML wildcard '*'; use explicit verbs/resources/apiGroups", path)
		})
	}
}

// TestNoWildcardsInKubebuilderMarkers scans all controller source files that
// contain kubebuilder RBAC markers for wildcard values.
func TestNoWildcardsInKubebuilderMarkers(t *testing.T) {
	sourceFiles := []string{
		"dspipeline_controller.go",
		"webhook.go",
	}
	for _, file := range sourceFiles {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			assert.NoError(t, err, "failed to read %s", file)

			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				line := strings.TrimSpace(scanner.Text())
				if !strings.HasPrefix(line, "//+kubebuilder:rbac:") {
					continue
				}
				assert.False(t, checkMarkerLineForWildcards(line),
					fmt.Sprintf("%s:%d: kubebuilder marker contains wildcard: %s", file, lineNum, line))
			}
			assert.NoError(t, scanner.Err())
		})
	}
}

// collectPermissions extracts all (apiGroup, resource, verb) triples from RBAC rules.
func collectPermissions(rules []rbacv1.PolicyRule) map[string]bool {
	perms := make(map[string]bool)
	for _, rule := range rules {
		for _, group := range rule.APIGroups {
			for _, resource := range rule.Resources {
				for _, verb := range rule.Verbs {
					perms[fmt.Sprintf("%s/%s/%s", group, resource, verb)] = true
				}
			}
		}
	}
	return perms
}

// loadOperatorPermissions returns the union of all permissions from the
// operator's ClusterRoles (role.yaml + argo_role.yaml).
func loadOperatorPermissions(t *testing.T) map[string]bool {
	t.Helper()
	perms := make(map[string]bool)
	for _, path := range []string{"../config/rbac/role.yaml", "../config/rbac/argo_role.yaml"} {
		data, err := os.ReadFile(path)
		assert.NoError(t, err)

		var role rbacv1.ClusterRole
		assert.NoError(t, yaml.Unmarshal(data, &role))

		for k, v := range collectPermissions(role.Rules) {
			perms[k] = v
		}
	}
	return perms
}

// parseRoleTemplate reads a Role template, strips Go template expressions
// and conditional lines, and returns the parsed rules.
func parseRoleTemplate(t *testing.T, path string) []rbacv1.PolicyRule {
	t.Helper()
	data, err := os.ReadFile(path)
	assert.NoError(t, err)

	// Remove lines that are purely template control flow ({{- if }}, {{- end }})
	// so the remaining YAML is valid.
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{{-") || trimmed == "{{end}}" {
			continue
		}
		lines = append(lines, line)
	}
	cleaned := regexp.MustCompile(`\{\{[^}]*\}\}`).ReplaceAllString(strings.Join(lines, "\n"), "placeholder")

	var role rbacv1.Role
	if err := yaml.Unmarshal([]byte(cleaned), &role); err != nil {
		t.Fatalf("could not parse %s after stripping template expressions: %v", path, err)
	}
	return role.Rules
}

// TestOperatorRolesAreSupersetOfRoleTemplates verifies that the union of the
// operator's ClusterRoles covers every permission in the namespace-scoped Role
// templates the operator creates. Required by Kubernetes RBAC escalation prevention.
func TestOperatorRolesAreSupersetOfRoleTemplates(t *testing.T) {
	operatorPerms := loadOperatorPermissions(t)

	templates := map[string]string{
		"workflow-controller": "../config/internal/workflow-controller/role.yaml.tmpl",
		"pipeline-runner":     "../config/internal/apiserver/default/role_pipeline-runner.yaml.tmpl",
	}

	for name, path := range templates {
		t.Run(name, func(t *testing.T) {
			rules := parseRoleTemplate(t, path)
			if rules == nil {
				t.Skip("could not parse template")
			}
			templatePerms := collectPermissions(rules)
			for perm := range templatePerms {
				assert.True(t, operatorPerms[perm],
					"%s Role template requires permission %q which is missing from the operator's ClusterRoles", name, perm)
			}
		})
	}
}
