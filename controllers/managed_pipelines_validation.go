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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
)

// ManagedPipelineEntry represents a single entry in managed-pipelines.json.
type ManagedPipelineEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Stability   string `json:"stability"`
}

// PipelineNamesFetcher abstracts fetching and parsing of pipeline names from an image.
type PipelineNamesFetcher interface {
	FetchPipelineNames(ctx context.Context, imageRef string) (map[string]bool, error)
}

// ParseManagedPipelinesManifest parses the JSON content of managed-pipelines.json
// and returns the set of valid pipeline names.
func ParseManagedPipelinesManifest(data []byte) (map[string]bool, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("managed-pipelines.json content is empty")
	}

	var entries []ManagedPipelineEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse managed-pipelines.json: %w", err)
	}

	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		trimmed := strings.TrimSpace(e.Name)
		if trimmed != "" {
			names[trimmed] = true
		}
	}
	return names, nil
}

// ValidateManagedPipelineNames checks that every pipeline name in the CR exists
// in the manifest's valid name set. Returns nil if all names are valid.
func ValidateManagedPipelineNames(crPipelines []dspav1.ManagedPipeline, manifestNames map[string]bool) error {
	if len(crPipelines) == 0 {
		return nil
	}

	var invalid []string
	for _, p := range crPipelines {
		if !manifestNames[p.Name] {
			invalid = append(invalid, p.Name)
		}
	}

	if len(invalid) > 0 {
		return fmt.Errorf(
			"pipeline(s) not found in managed-pipelines.json: %s",
			strings.Join(invalid, ", "),
		)
	}
	return nil
}
