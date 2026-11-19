//go:build test_integration

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

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	testutil "github.com/opendatahub-io/data-science-pipelines-operator/tests/util"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

func (suite *IntegrationTestSuite) TestPostgreSQLPipelineRun() {
	t := suite.T()
	dsn := os.Getenv("DSPO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set DSPO_TEST_POSTGRES_DSN to verify pipeline persistence in PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(suite.Ctx, 8*time.Minute)
	defer cancel()

	dspa := &dspav1.DataSciencePipelinesApplication{}
	require.NoError(t, suite.Clientmgr.k8sClient.Get(ctx, types.NamespacedName{
		Name: suite.DSPA.Name, Namespace: suite.DSPANamespace,
	}, dspa))
	require.NotNil(t, dspa.Spec.Database)
	postgres := dspa.Spec.Database.ExternalDB
	require.NotNil(t, postgres)
	require.Equal(t, dspav1.DatabaseEnginePostgreSQL, postgres.Engine)
	require.NotEmpty(t, postgres.Schema, "configure a non-default PostgreSQL schema")
	require.NotEqual(t, "public", postgres.Schema)
	require.False(t, dspa.Spec.Database.DisableHealthCheck)
	if extra := dspa.Spec.Database.CustomExtraParams; extra != nil {
		var options map[string]string
		require.NoError(t, json.Unmarshal([]byte(*extra), &options))
		if mode, set := options["sslmode"]; set {
			require.Equal(t, "verify-full", mode)
		}
	}

	dbConfig, err := pgx.ParseConfig(dsn)
	require.True(t, err == nil, "could not parse DSPO_TEST_POSTGRES_DSN")
	require.NotNil(t, dbConfig.TLSConfig, "configure verified TLS in DSPO_TEST_POSTGRES_DSN")
	require.False(t, dbConfig.TLSConfig.InsecureSkipVerify)
	require.NotEmpty(t, dbConfig.TLSConfig.ServerName, "use TLS hostname verification")
	require.Empty(t, dbConfig.Fallbacks, "use one PostgreSQL endpoint with verified TLS")
	require.Equal(t, postgres.DBName, dbConfig.Database)
	require.Equal(t, postgres.Username, dbConfig.User)
	db, err := pgx.ConnectConfig(ctx, dbConfig)
	require.NoError(t, err)
	defer db.Close(ctx)
	var canCreateDB, superuser, canConnectMaintenanceDB bool
	require.NoError(t, db.QueryRow(ctx, `SELECT rolcreatedb, rolsuper, has_database_privilege('postgres', 'CONNECT')
		FROM pg_catalog.pg_roles WHERE rolname = current_user`).Scan(&canCreateDB, &superuser, &canConnectMaintenanceDB))
	require.False(t, canCreateDB)
	require.False(t, superuser)
	require.False(t, canConnectMaintenanceDB)

	pipelineYAML, err := os.ReadFile("resources/postgresql-smoke-pipeline.yaml")
	require.NoError(t, err)
	pipelineJSON, err := yaml.YAMLToJSON(pipelineYAML)
	require.NoError(t, err)
	name := fmt.Sprintf("postgresql-smoke-%d", time.Now().UnixNano())
	var experiment struct {
		ID string `json:"experiment_id"`
	}
	require.NoError(t, suite.requestPostgreSQLSmokeAPI(ctx, http.MethodPost, "/experiments", map[string]string{
		"display_name": name, "namespace": suite.DSPANamespace,
	}, &experiment))
	require.NotEmpty(t, experiment.ID)

	type smokeRun struct {
		ID    string `json:"run_id"`
		State string `json:"state"`
	}
	var run smokeRun
	require.NoError(t, suite.requestPostgreSQLSmokeAPI(ctx, http.MethodPost, "/runs", map[string]any{
		"display_name": name, "experiment_id": experiment.ID,
		"pipeline_spec": json.RawMessage(pipelineJSON), "runtime_config": struct{}{},
	}, &run))
	require.NotEmpty(t, run.ID)
	runID := run.ID
	require.NoError(t, testutil.WaitFor(ctx, 6*time.Minute, PollInterval, func() (bool, error) {
		var observed smokeRun
		if err := suite.requestPostgreSQLSmokeAPI(ctx, http.MethodGet, "/runs/"+url.PathEscape(runID), nil, &observed); err != nil {
			return false, err
		}
		if observed.ID != runID {
			return false, fmt.Errorf("expected run %s, got %q", runID, observed.ID)
		}
		switch observed.State {
		case "SUCCEEDED":
			return true, nil
		case "FAILED", "SKIPPED", "CANCELING", "CANCELED", "PAUSED":
			return false, fmt.Errorf("pipeline run %s reached state %s", runID, observed.State)
		default:
			return false, nil
		}
	}))

	// Query the exact schema and UUID returned by the API so another run or a
	// table in the default search path cannot satisfy the test.
	runsTable := pgx.Identifier{postgres.Schema, "run_details"}.Sanitize()
	var storedState string
	require.NoError(t, db.QueryRow(ctx, `SELECT "State" FROM `+runsTable+` WHERE "UUID" = $1`, runID).Scan(&storedState))
	require.Equal(t, "SUCCEEDED", storedState)
}

func (suite *IntegrationTestSuite) requestPostgreSQLSmokeAPI(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, APIServerURL+"/apis/v2beta1"+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := suite.Clientmgr.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("%s %s returned HTTP %d: %s", method, path, response.StatusCode, message)
	}
	return json.NewDecoder(response.Body).Decode(output)
}
