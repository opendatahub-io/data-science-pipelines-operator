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
	env := []string{
		"PATH=/usr/bin",
		"MANAGED_PIPELINE_IMAGE_Z=last-alpha",
		"MANAGED_PIPELINE_IMAGE_A=first-alpha",
		"OTHER=1",
		"MANAGED_PIPELINE_IMAGE_=empty-suffix-rejected",
	}
	got := ManagedPipelineImageEnvFromEnviron(env)
	require.Len(t, got, 2)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_A", got[0].Name)
	assert.Equal(t, "first-alpha", got[0].Value)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_Z", got[1].Name)
	assert.Equal(t, "last-alpha", got[1].Value)
}

func TestManagedPipelineImageEnvFromEnviron_EmptyValueAndEqualsInValue(t *testing.T) {
	t.Parallel()
	env := []string{
		"MANAGED_PIPELINE_IMAGE_FOO=",
		"MANAGED_PIPELINE_IMAGE_BAR=sha256:abc=padding",
	}
	got := ManagedPipelineImageEnvFromEnviron(env)
	require.Len(t, got, 2)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_BAR", got[0].Name)
	assert.Equal(t, "sha256:abc=padding", got[0].Value)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_FOO", got[1].Name)
	assert.Equal(t, "", got[1].Value)
}

func TestManagedPipelineImageEnvFromEnviron_IgnoresMalformedAndNonMatching(t *testing.T) {
	t.Parallel()
	env := []string{
		"",
		"noequals",
		"=onlyvalue",
		"MANAGED_PIPELINE_IMAGE=not-underscore-suffix",
		"PREFIX_MANAGED_PIPELINE_IMAGE_X=1",
	}
	got := ManagedPipelineImageEnvFromEnviron(env)
	assert.Empty(t, got)
}

func TestManagedPipelineImageEnvFromEnviron_RejectsInvalidNameChars(t *testing.T) {
	t.Parallel()
	env := []string{
		"MANAGED_PIPELINE_IMAGE_lower=bad",
		"MANAGED_PIPELINE_IMAGE_OK=good",
		"MANAGED_PIPELINE_IMAGE_has-dash=bad",
		"MANAGED_PIPELINE_IMAGE_has space=bad",
		"MANAGED_PIPELINE_IMAGE_colon:bad=bad",
	}
	got := ManagedPipelineImageEnvFromEnviron(env)
	require.Len(t, got, 1)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_OK", got[0].Name)
	assert.Equal(t, "good", got[0].Value)
}

func TestManagedPipelineImageEnvFromEnviron_DuplicateNamesDeduplicatedLastWins(t *testing.T) {
	t.Parallel()
	env := []string{
		"MANAGED_PIPELINE_IMAGE_FOO=first",
		"MANAGED_PIPELINE_IMAGE_FOO=second",
	}
	got := ManagedPipelineImageEnvFromEnviron(env)
	require.Len(t, got, 1)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_FOO", got[0].Name)
	assert.Equal(t, "second", got[0].Value)
}
