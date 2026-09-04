//go:build test_all || test_unit

/*

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYAMLString(t *testing.T) {
	value := "claim\"\nhostNetwork: true"

	encoded, err := yamlString(value)
	require.NoError(t, err)
	assert.Equal(t, `"claim\"\nhostNetwork: true"`, encoded)

	encoded, err = yamlString(&value)
	require.NoError(t, err)
	assert.Equal(t, `"claim\"\nhostNetwork: true"`, encoded)
}

func TestYAMLStringRejectsUnsupportedValues(t *testing.T) {
	_, err := yamlString(42)
	require.ErrorContains(t, err, "cannot encode int as a YAML string")

	var value *string
	_, err = yamlString(value)
	require.ErrorContains(t, err, "cannot encode a nil string pointer")
}
