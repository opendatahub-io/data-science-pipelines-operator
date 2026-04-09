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
	"fmt"
	"slices"
	"strings"
)

// relatedImageEnvPrefix matches env vars forwarded to the managed-pipelines init
// container (e.g. RELATED_IMAGE_AUTOML_RUNTIME).
const relatedImageEnvPrefix = "RELATED_IMAGE_"

// ManagedPipelineImageEnvVar is a name/value pair from the operator environment.
type ManagedPipelineImageEnvVar struct {
	Name  string
	Value string
}

// ManagedPipelineImageEnvFromEnviron returns RELATED_IMAGE_* env vars from an
// os.Environ()-style list ("KEY=VALUE"), sorted by key. Returns an error if
// any RELATED_IMAGE_* prefixed var has an invalid name; empty values are ignored.
func ManagedPipelineImageEnvFromEnviron(environ []string) ([]ManagedPipelineImageEnvVar, error) {
	cleaned := make(map[string]string)
	for _, raw := range environ {
		key, value, found := strings.Cut(raw, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if !strings.HasPrefix(key, relatedImageEnvPrefix) {
			continue
		}
		if len(key) == len(relatedImageEnvPrefix) || !isValidEnvVarName(key) {
			return nil, fmt.Errorf("invalid RELATED_IMAGE_* env var name %q: must contain only [A-Z0-9_] with a non-empty suffix after the prefix", key)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned[key] = value
	}
	out := make([]ManagedPipelineImageEnvVar, 0, len(cleaned))
	for key, value := range cleaned {
		out = append(out, ManagedPipelineImageEnvVar{Name: key, Value: value})
	}
	slices.SortFunc(out, func(a, b ManagedPipelineImageEnvVar) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
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
