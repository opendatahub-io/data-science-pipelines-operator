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
	var out []ManagedPipelineImageEnvVar
	for _, e := range environ {
		idx := strings.IndexByte(e, '=')
		if idx <= 0 {
			continue
		}
		key := e[:idx]
		if !strings.HasPrefix(key, managedPipelineImageEnvPrefix) {
			continue
		}
		if !isValidEnvVarName(key) {
			continue
		}
		out = append(out, ManagedPipelineImageEnvVar{Name: key, Value: e[idx+1:]})
	}
	slices.SortFunc(out, func(a, b ManagedPipelineImageEnvVar) int {
		return strings.Compare(a.Name, b.Name)
	})
	if out == nil {
		return []ManagedPipelineImageEnvVar{}
	}
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
