//go:build test_all || test_unit

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
		"MANAGED_PIPELINE_IMAGE_=empty-suffix-ok",
	}
	got := ManagedPipelineImageEnvFromEnviron(env)
	require.Len(t, got, 3)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_", got[0].Name)
	assert.Equal(t, "empty-suffix-ok", got[0].Value)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_A", got[1].Name)
	assert.Equal(t, "first-alpha", got[1].Value)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_Z", got[2].Name)
	assert.Equal(t, "last-alpha", got[2].Value)
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

func TestManagedPipelineImageEnvFromEnviron_DuplicateNamesPreserved(t *testing.T) {
	t.Parallel()
	env := []string{
		"MANAGED_PIPELINE_IMAGE_FOO=first",
		"MANAGED_PIPELINE_IMAGE_FOO=second",
	}
	got := ManagedPipelineImageEnvFromEnviron(env)
	require.Len(t, got, 2, "duplicate names are preserved; real OS environments do not have duplicates")
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_FOO", got[0].Name)
	assert.Equal(t, "MANAGED_PIPELINE_IMAGE_FOO", got[1].Name)
	values := []string{got[0].Value, got[1].Value}
	assert.ElementsMatch(t, []string{"first", "second"}, values)
}
