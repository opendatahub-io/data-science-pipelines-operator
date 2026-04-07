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

func TestManagedPipelineImageEnvFromJSON_PrefixAndSorting(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_Z":"last-alpha","RELATED_IMAGE_A":"first-alpha"}`)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "RELATED_IMAGE_A", got[0].Name)
	assert.Equal(t, "first-alpha", got[0].Value)
	assert.Equal(t, "RELATED_IMAGE_Z", got[1].Name)
	assert.Equal(t, "last-alpha", got[1].Value)
}

func TestManagedPipelineImageEnvFromJSON_EmptyObject(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromJSON(`{}`)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestManagedPipelineImageEnvFromJSON_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_X":`)
	require.Error(t, err)
}

func TestManagedPipelineImageEnvFromJSON_RejectsInvalidNameChars(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_lower":"bad"}`)
	require.Error(t, err)

	_, err = ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_has-dash":"bad"}`)
	require.Error(t, err)

	_, err = ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_OK":"good"}`)
	require.NoError(t, err)
}

func TestManagedPipelineImageEnvFromJSON_RejectsMissingRelatedImagePrefix(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`{"NOT_RELATED_IMAGE_X":"img"}`)
	require.Error(t, err)
}

func TestManagedPipelineImageEnvFromJSON_RejectsEmptyString(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(``)
	require.Error(t, err)
}

func TestManagedPipelineImageEnvFromJSON_RejectsJSONArray(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`["RELATED_IMAGE_X"]`)
	require.Error(t, err)
}

func TestManagedPipelineImageEnvFromJSON_RejectsNonStringValues(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_X":123}`)
	require.Error(t, err)
}

func TestManagedPipelineImageEnvFromJSON_RejectsBarePrefix(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_":"img"}`)
	require.Error(t, err)
}

func TestManagedPipelineImageEnvFromJSON_RejectsEmptyValue(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_X":""}`)
	require.Error(t, err)
}

func TestManagedPipelineImageEnvFromJSON_RejectsWhitespaceOnlyValue(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_X":"   "}`)
	require.Error(t, err)
}

func TestManagedPipelineImageEnvFromJSON_TrimsWhitespaceFromValue(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromJSON(`{"RELATED_IMAGE_X":"  registry.example/img@sha256:abc  "}`)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "registry.example/img@sha256:abc", got[0].Value)
}

func TestManagedPipelineImageEnvFromJSON_TrimsWhitespaceFromKey(t *testing.T) {
	t.Parallel()
	got, err := ManagedPipelineImageEnvFromJSON(`{"  RELATED_IMAGE_X  ":"img"}`)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "RELATED_IMAGE_X", got[0].Name)
}

func TestManagedPipelineImageEnvFromJSON_WhitespaceKeyCollisionReturnsError(t *testing.T) {
	t.Parallel()
	_, err := ManagedPipelineImageEnvFromJSON(`{"  RELATED_IMAGE_X  ":"first","RELATED_IMAGE_X":"second"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate env var key")
}
