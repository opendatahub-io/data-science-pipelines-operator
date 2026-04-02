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
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
)

// relatedImageEnvPrefix matches env vars forwarded to the managed-pipelines init
// container through DSPO.ManagedPipelinesImages (e.g. RELATED_IMAGE_AUTOML_RUNTIME).
const relatedImageEnvPrefix = "RELATED_IMAGE_"

// ManagedPipelineImageEnvVar is a name/value pair from the operator environment.
type ManagedPipelineImageEnvVar struct {
	Name  string
	Value string
}

// ManagedPipelineImageEnvFromJSON parses a JSON object and returns name/value entries
// whose keys are RELATED_IMAGE_* env var names, sorted by name.
func ManagedPipelineImageEnvFromJSON(raw string, log logr.Logger) ([]ManagedPipelineImageEnvVar, error) {
	m := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("invalid DSPO managed pipeline images JSON mapping: %w", err)
	}
	cleaned := make(map[string]string, len(m))
	for rawKey, rawValue := range m {
		key := strings.TrimSpace(rawKey)
		if !strings.HasPrefix(key, relatedImageEnvPrefix) {
			return nil, fmt.Errorf("invalid env var name %q: must start with %q", key, relatedImageEnvPrefix)
		}
		if len(key) == len(relatedImageEnvPrefix) || !isValidEnvVarName(key) {
			return nil, fmt.Errorf("invalid env var name %q: must contain only [A-Z0-9_] and non-empty suffix", key)
		}
		value := strings.TrimSpace(rawValue)
		if value == "" {
			return nil, fmt.Errorf("empty image value for env var %q", key)
		}
		if prev, exists := cleaned[key]; exists {
			log.Info("duplicate env var key after whitespace trimming, last value wins",
				"key", key, "previousValue", prev, "newValue", value)
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
