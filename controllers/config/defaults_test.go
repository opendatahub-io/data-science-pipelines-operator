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

package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestAIPipelinesModuleControllerEnabled(t *testing.T) {
	t.Cleanup(viper.Reset)

	require.False(t, AIPipelinesModuleControllerEnabled())

	viper.Set(EnableAIPipelinesModuleControllerConfigName, true)
	require.True(t, AIPipelinesModuleControllerEnabled())

	viper.Set(EnableAIPipelinesModuleControllerConfigName, false)
	require.False(t, AIPipelinesModuleControllerEnabled())
}

func TestResolveImagePrefersNonEmptyPlatformValue(t *testing.T) {
	t.Cleanup(viper.Reset)

	for configName, envName := range platformRelatedImages {
		t.Run(configName, func(t *testing.T) {
			viper.Set(configName, "standalone.example/image:latest")
			t.Setenv(envName, " platform.example/image@sha256:abc ")
			require.Equal(t, "platform.example/image@sha256:abc", ResolveImage(configName))
		})
	}
}

func TestResolveImageRetainsStandaloneFallbackForEmptyPlatformValue(t *testing.T) {
	t.Cleanup(viper.Reset)

	for configName, envName := range platformRelatedImages {
		t.Run(configName, func(t *testing.T) {
			viper.Set(configName, "standalone.example/image:latest")
			t.Setenv(envName, "   ")
			require.Equal(t, "standalone.example/image:latest", ResolveImage(configName))
		})
	}
}
