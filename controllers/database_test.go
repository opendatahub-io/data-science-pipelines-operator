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
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgconn"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSetupExternalDatabaseParams(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "pipelines"},
		Data:       map[string][]byte{"password": []byte("db-password")},
	}
	kubeClient := fake.NewClientBuilder().WithObjects(secret).Build()
	for _, tt := range []struct {
		name   string
		engine dspav1.DatabaseEngine
		port   string
		schema string
		extra  string
		want   DBConnection
	}{
		{name: "legacy MySQL", port: "3306", want: DBConnection{Engine: dspav1.DatabaseEngineMySQL, Port: "3306", ExtraParams: `{"tls":"true"}`}},
		{name: "PostgreSQL defaults", engine: dspav1.DatabaseEnginePostgreSQL, want: DBConnection{Engine: dspav1.DatabaseEnginePostgreSQL, Port: "5432", Schema: "public", ExtraParams: `{"sslmode":"verify-full"}`}},
		{name: "PostgreSQL custom settings", engine: dspav1.DatabaseEnginePostgreSQL, port: "15432", schema: "tenant", extra: `{"application_name":"dspo","connect_timeout":"4"}`, want: DBConnection{Engine: dspav1.DatabaseEnginePostgreSQL, Port: "15432", Schema: "tenant", ExtraParams: `{"application_name":"dspo","connect_timeout":"4","sslmode":"verify-full"}`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &dspav1.Database{ExternalDB: &dspav1.ExternalDB{
				Engine: tt.engine, Host: "database.example.com", Port: tt.port, Schema: tt.schema,
				Username: "pipeline-user", DBName: "pipeline-db",
				PasswordSecret: &dspav1.SecretKeyValue{Name: secret.Name, Key: "password"},
			}}
			if tt.extra != "" {
				db.CustomExtraParams = &tt.extra
			}
			dspa := &dspav1.DataSciencePipelinesApplication{Spec: dspav1.DSPASpec{Database: db}}
			params := &DSPAParams{Namespace: secret.Namespace}
			require.NoError(t, params.SetupDBParams(t.Context(), dspa, kubeClient, logr.Discard()))
			assert.Equal(t, tt.want.Engine, params.DBConnection.Engine)
			assert.Equal(t, tt.want.Port, params.DBConnection.Port)
			assert.Equal(t, tt.want.Schema, params.DBConnection.Schema)
			assert.JSONEq(t, tt.want.ExtraParams, params.DBConnection.ExtraParams)
			assert.Equal(t, "db-password", params.DBConnection.DecodedPassword)
		})
	}
}

func TestRejectInvalidPostgreSQLConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*dspav1.Database)
		field  string
	}{
		{"missing credentials", func(db *dspav1.Database) { db.ExternalDB.PasswordSecret = nil }, "passwordSecret"},
		{"managed MariaDB conflict", func(db *dspav1.Database) { db.MariaDB = &dspav1.MariaDB{Deploy: true} }, "mariaDB.deploy"},
		{"invalid port", func(db *dspav1.Database) { db.Port = "65536" }, "externalDB.port"},
		{"missing database", func(db *dspav1.Database) { db.ExternalDB.DBName = "" }, "externalDB.pipelineDBName"},
		{"reserved schema", func(db *dspav1.Database) { db.Schema = "$user" }, "externalDB.schema"},
		{"MySQL option", func(db *dspav1.Database) { db.CustomExtraParams = new(`{"tls":"true"}`) }, "unsupported PostgreSQL parameter"},
		{"invalid TLS mode", func(db *dspav1.Database) { db.CustomExtraParams = new(`{"sslmode":"prefer"}`) }, "customExtraParams.sslmode"},
		{"null option", func(db *dspav1.Database) { db.CustomExtraParams = new(`{"application_name":null}`) }, `value for "application_name" must be a string`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &dspav1.Database{ExternalDB: &dspav1.ExternalDB{
				Engine: dspav1.DatabaseEnginePostgreSQL, Host: "database.example.com", Username: "pipeline-user", DBName: "pipeline-db",
				PasswordSecret: &dspav1.SecretKeyValue{Name: "credentials", Key: "password"},
			}}
			tt.change(db)
			dspa := &dspav1.DataSciencePipelinesApplication{Spec: dspav1.DSPASpec{Database: db}}
			params := &DSPAParams{}
			require.ErrorContains(t, params.SetupDBParams(t.Context(), dspa, nil, logr.Discard()), tt.field)
		})
	}
}

func TestPostgreSQLHealthCheckConfig(t *testing.T) {
	db := DBConnection{Host: "database.example.com", Port: "5432", Username: "db user", DBName: "pipeline/db", Schema: `my"schema`}
	password := "p@ss# word&!"
	options := map[string]string{"sslmode": "verify-full"}
	ca, err := os.ReadFile("testdata/tls/ca-bundle.crt")
	require.NoError(t, err)
	cfg, err := postgreSQLHealthCheckConfig(db, password, options, [][]byte{ca})
	require.NoError(t, err)
	assert.Equal(t, db.Host, cfg.Host)
	assert.Equal(t, db.Username, cfg.User)
	assert.Equal(t, db.DBName, cfg.Database)
	assert.Equal(t, password, cfg.Password)
	assert.Equal(t, `"my""schema"`, cfg.RuntimeParams["search_path"])
	require.NotNil(t, cfg.TLSConfig)
	assert.Equal(t, db.Host, cfg.TLSConfig.ServerName)
	assert.False(t, cfg.TLSConfig.InsecureSkipVerify)
	assert.Empty(t, cfg.Fallbacks)
	_, err = postgreSQLHealthCheckConfig(db, password, options, [][]byte{[]byte("invalid PEM")})
	require.ErrorContains(t, err, "invalid CA certificate")

	options = map[string]string{"sslmode": "disable", "connect_timeout": "4", "application_name": "dspo"}
	cfg, err = postgreSQLHealthCheckConfig(db, password, options, nil)
	require.NoError(t, err)
	assert.Nil(t, cfg.TLSConfig)
	assert.Equal(t, 4*time.Second, cfg.ConnectTimeout)
	assert.Equal(t, "dspo", cfg.RuntimeParams["application_name"])
}

func TestPostgreSQLConnectionError(t *testing.T) {
	for _, tt := range []struct {
		cause error
		want  string
	}{
		{&pgconn.PgError{Code: "28P01", Message: "sensitive details"}, "authentication failed"},
		{&pgconn.PgError{Code: "3D000"}, `database "pipelines" does not exist`},
		{&pgconn.PgError{Code: "42501"}, `role cannot connect to database "pipelines"`},
		{&tls.CertificateVerificationError{}, "TLS negotiation or certificate verification failed"},
		{context.DeadlineExceeded, "timed out"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			err := postgreSQLConnectionError(fmt.Errorf("connecting: %w", tt.cause), "pipelines")
			require.ErrorContains(t, err, tt.want)
			assert.NotContains(t, err.Error(), "sensitive details")
		})
	}
}

// TestValidateDBExtraParams_AllowedKeys verifies that allowed keys pass validation.
func TestValidateDBExtraParams_AllowedKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"tls only", `{"tls":"true"}`},
		{"TLS compatibility modes", `{"tls":"skip-verify"}`},
		{"TLS preferred mode", `{"tls":"preferred"}`},
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

// TestValidateDBExtraParams_InvalidJSON verifies that malformed JSON and non-string values are rejected.
func TestValidateDBExtraParams_InvalidJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not json", `not-json`},
		{"array instead of object", `["tls"]`},
		{"unclosed brace", `{"tls":"true"`},
		{"boolean tls value", `{"tls":true}`},
		{"null", `null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := config.ValidateDBExtraParams(tt.input)
			assert.Error(t, err)
			assert.Nil(t, params)
			assert.Contains(t, err.Error(), "JSON object with string values")
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

	mariadbPolicy := requireNetworkPolicy(t, ctx, reconciler, expectedDatabaseName)
	assertPolicySelects(t, mariadbPolicy, pipelineComponentLabels(expectedDatabaseName))
	assertPortPresent(t, mariadbPolicy, 3306)
	assertDeniesAllEgress(t, mariadbPolicy)
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
