//go:build test_all || test_unit

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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedPipelineImageEnvFromEnviron_PrefixAndSorting(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromEnviron([]string{
		"RELATED_IMAGE_Z=last-alpha",
		"RELATED_IMAGE_A=first-alpha",
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "RELATED_IMAGE_A", got[0].Name)
	assert.Equal(t, "first-alpha", got[0].Value)
	assert.Equal(t, "RELATED_IMAGE_Z", got[1].Name)
	assert.Equal(t, "last-alpha", got[1].Value)
}

func TestManagedPipelineImageEnvFromEnviron_EmptyInput(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromEnviron(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestManagedPipelineImageEnvFromEnviron_IgnoresNonMatchingEntries(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromEnviron([]string{
		"NOT_RELATED_IMAGE_X=img",
		"PATH=/usr/bin",
		"MALFORMED_ENTRY_WITHOUT_EQUALS",
		"RELATED_IMAGE_OK=good",
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "RELATED_IMAGE_OK", got[0].Name)
	assert.Equal(t, "good", got[0].Value)
}

func TestManagedPipelineImageEnvFromEnviron_RejectsBarePrefix(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromEnviron([]string{"RELATED_IMAGE_=bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid RELATED_IMAGE_* env var name")
}

func TestManagedPipelineImageEnvFromEnviron_RejectsLowercaseSuffix(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromEnviron([]string{"RELATED_IMAGE_lower=bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid RELATED_IMAGE_* env var name")
}

func TestManagedPipelineImageEnvFromEnviron_RejectsDashInName(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromEnviron([]string{"RELATED_IMAGE_HAS-DASH=bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid RELATED_IMAGE_* env var name")
}

func TestManagedPipelineImageEnvFromEnviron_IgnoresEmptyValue(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromEnviron([]string{"RELATED_IMAGE_X="})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestManagedPipelineImageEnvFromEnviron_IgnoresWhitespaceOnlyValue(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromEnviron([]string{"RELATED_IMAGE_X=   "})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestManagedPipelineImageEnvFromEnviron_TrimsWhitespaceFromValue(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromEnviron([]string{"RELATED_IMAGE_X=  registry.example/img@sha256:abc  "})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "registry.example/img@sha256:abc", got[0].Value)
}

func TestManagedPipelineImageEnvFromEnviron_LastValueWinsOnDuplicateKey(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromEnviron([]string{
		"RELATED_IMAGE_X=first",
		"RELATED_IMAGE_X=second",
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "RELATED_IMAGE_X", got[0].Name)
	assert.Equal(t, "second", got[0].Value)
}
