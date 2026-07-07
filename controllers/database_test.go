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

package controllers

import (
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

// TestValidateDBExtraParams_AllowedKeys verifies that allowed keys pass validation.
func TestValidateDBExtraParams_AllowedKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"tls only", `{"tls":"true"}`},
		{"charset only", `{"charset":"utf8mb4"}`},
		{"multiple allowed keys", `{"tls":"true","charset":"utf8","parseTime":"True","loc":"Local"}`},
		{"timeout keys", `{"timeout":"30s","readTimeout":"10s","writeTimeout":"10s"}`},
		{"collation and sql_mode", `{"collation":"utf8mb4_unicode_ci","sql_mode":"STRICT_TRANS_TABLES"}`},
		{"postgresql keys", `{"sslmode":"require","connect_timeout":"10","application_name":"dspo"}`},
		{"additional safe keys", `{"clientFoundRows":"true","columnsWithAlias":"true","maxAllowedPacket":"67108864"}`},
		{"empty object", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := config.ValidateDBExtraParams(tt.input)
			assert.NoError(t, err)
			assert.NotNil(t, params)
		})
	}
}

// TestValidateDBExtraParams_DisallowedKeys verifies that dangerous driver-level keys are rejected.
func TestValidateDBExtraParams_DisallowedKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		key   string
	}{
		{"allowAllFiles", `{"allowAllFiles":"true"}`, "allowAllFiles"},
		{"allowCleartextPasswords", `{"allowCleartextPasswords":"true"}`, "allowCleartextPasswords"},
		{"allowOldPasswords", `{"allowOldPasswords":"true"}`, "allowOldPasswords"},
		{"allowFallbackToPlaintext", `{"allowFallbackToPlaintext":"true"}`, "allowFallbackToPlaintext"},
		{"allowNativePasswords", `{"allowNativePasswords":"false"}`, "allowNativePasswords"},
		{"multiStatements", `{"multiStatements":"true"}`, "multiStatements"},
		{"interpolateParams", `{"interpolateParams":"true"}`, "interpolateParams"},
		{"mixed allowed and disallowed", `{"tls":"true","allowAllFiles":"true"}`, "allowAllFiles"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := config.ValidateDBExtraParams(tt.input)
			assert.Error(t, err)
			assert.Nil(t, params)
			assert.Contains(t, err.Error(), tt.key)
			assert.Contains(t, err.Error(), "disallowed")
		})
	}
}

// TestValidateDBExtraParams_InvalidJSON verifies that malformed JSON is rejected.
func TestValidateDBExtraParams_InvalidJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not json", `not-json`},
		{"array instead of object", `["tls"]`},
		{"unclosed brace", `{"tls":"true"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := config.ValidateDBExtraParams(tt.input)
			assert.Error(t, err)
			assert.Nil(t, params)
			assert.Contains(t, err.Error(), "not valid JSON")
		})
	}
}

// TestCreateMySQLConfig_DangerousFlagsExplicitlyDisabled verifies that createMySQLConfig
// sets dangerous driver-level boolean fields to false regardless of extra params.
func TestCreateMySQLConfig_DangerousFlagsExplicitlyDisabled(t *testing.T) {
	cfg := createMySQLConfig("user", "pass", "localhost", "3306", "testdb", map[string]string{
		"tls":     "true",
		"charset": "utf8mb4",
	})

	assert.False(t, cfg.AllowAllFiles, "AllowAllFiles must be explicitly false")
	assert.False(t, cfg.AllowCleartextPasswords, "AllowCleartextPasswords must be explicitly false")
	assert.False(t, cfg.AllowOldPasswords, "AllowOldPasswords must be explicitly false")
	assert.False(t, cfg.AllowFallbackToPlaintext, "AllowFallbackToPlaintext must be explicitly false")
	// AllowNativePasswords should remain true for normal operation
	assert.True(t, cfg.AllowNativePasswords, "AllowNativePasswords must be true for normal MySQL auth")
}

// TestCreateMySQLConfig_ExtraParamsMerged verifies that user-provided extra
// params override defaults and are placed in the Params map.
func TestCreateMySQLConfig_ExtraParamsMerged(t *testing.T) {
	extra := map[string]string{
		"tls":     "skip-verify",
		"charset": "utf8mb4",
	}
	cfg := createMySQLConfig("user", "pass", "host", "3306", "db", extra)

	assert.Equal(t, "skip-verify", cfg.Params["tls"])
	assert.Equal(t, "utf8mb4", cfg.Params["charset"])
	assert.Equal(t, "True", cfg.Params["parseTime"])
}

func TestDeployDatabase(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedDatabaseName := "mariadb-testdspa"

	// Construct DSPA Spec with deployed MariaDB Database
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert Database Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedDatabaseName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileDatabase(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert Database Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedDatabaseName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
}

func TestDontDeployDatabase(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedDatabaseName := "mariadb-testdspa"

	// Construct DSPA Spec with non-deployed MariaDB Database
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: false,
				},
			},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert Database Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedDatabaseName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileDatabase(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert Database Deployment still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedDatabaseName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}
