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
	"slices"
	"strings"
)

// managedPipelineImageEnvPrefix matches variables forwarded from the operator process
// environment to the managed-pipelines init container (e.g. MANAGED_PIPELINE_IMAGE_TOOLBOX).
const managedPipelineImageEnvPrefix = "MANAGED_PIPELINE_IMAGE_"

// ManagedPipelineImageEnvVar is a name/value pair from the operator environment.
type ManagedPipelineImageEnvVar struct {
	Name  string
	Value string
}

// ManagedPipelineImageEnvFromEnviron returns every environment entry whose name starts with
// managedPipelineImageEnvPrefix and whose name contains only uppercase ASCII letters, digits,
// and underscores, sorted by name.
// The returned slice is always non-nil (empty when no entries match). Callers that store
// the result conditionally (e.g. only when managed pipelines is enabled) may leave the
// target field nil.
func ManagedPipelineImageEnvFromEnviron(environ []string) []ManagedPipelineImageEnvVar {
	seen := map[string]string{}
	for _, e := range environ {
		idx := strings.IndexByte(e, '=')
		if idx <= 0 {
			continue
		}
		key := e[:idx]
		if !strings.HasPrefix(key, managedPipelineImageEnvPrefix) {
			continue
		}
		if len(key) == len(managedPipelineImageEnvPrefix) || !isValidEnvVarName(key) {
			continue
		}
		// Last occurrence wins for duplicated keys.
		seen[key] = e[idx+1:]
	}
	out := make([]ManagedPipelineImageEnvVar, 0, len(seen))
	for key, value := range seen {
		out = append(out, ManagedPipelineImageEnvVar{Name: key, Value: value})
	}
	slices.SortFunc(out, func(a, b ManagedPipelineImageEnvVar) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// isValidEnvVarName returns true if name consists only of [A-Z0-9_].
func isValidEnvVarName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return len(name) > 0
}
