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
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
)

func (db *DBConnection) setupPostgreSQL(database *dspav1.Database) error {
	if database.MariaDB != nil && database.MariaDB.Deploy {
		return errors.New("spec.database.mariaDB.deploy must be false when externalDB.engine is postgresql")
	}
	if db.Host == "" || strings.ContainsAny(db.Host, "/,\\ \t\r\n\x00") {
		return errors.New("spec.database.externalDB.host must specify a single PostgreSQL hostname or IP address")
	}
	if strings.TrimSpace(db.Username) == "" || strings.ContainsRune(db.Username, 0) {
		return errors.New("spec.database.externalDB.username must specify a PostgreSQL user")
	}
	if strings.TrimSpace(db.DBName) == "" || strings.ContainsRune(db.DBName, 0) {
		return errors.New("spec.database.externalDB.pipelineDBName must specify a PostgreSQL database")
	}

	db.Engine = dspav1.DatabaseEnginePostgreSQL
	setStringDefault("5432", &db.Port)
	port, err := strconv.ParseUint(db.Port, 10, 16)
	if err != nil || port == 0 {
		return errors.New("spec.database.externalDB.port must be an integer from 1 to 65535")
	}
	db.Port = strconv.FormatUint(port, 10)

	db.Schema = database.ExternalDB.Schema
	setStringDefault("public", &db.Schema)
	if strings.TrimSpace(db.Schema) == "" || len(db.Schema) > 63 || strings.ContainsRune(db.Schema, 0) || db.Schema == "$user" {
		return errors.New("spec.database.externalDB.schema must specify a PostgreSQL schema name of up to 63 bytes")
	}

	options := map[string]string{"sslmode": "verify-full"}
	if database.CustomExtraParams != nil {
		var custom map[string]*string
		if err := json.Unmarshal([]byte(*database.CustomExtraParams), &custom); err != nil || custom == nil {
			return errors.New("spec.database.customExtraParams must be a JSON object with string values")
		}
		for key, entry := range custom {
			if entry == nil {
				return fmt.Errorf("spec.database.customExtraParams value for %q must be a string", key)
			}
			value := *entry
			switch key {
			case "sslmode":
				switch value {
				case "verify-full", "require", "disable":
				default:
					return errors.New("spec.database.customExtraParams.sslmode must be verify-full, require, or disable for PostgreSQL")
				}
			case "connect_timeout":
				if _, err := strconv.ParseUint(value, 10, 31); err != nil {
					return errors.New("spec.database.customExtraParams.connect_timeout must be a nonnegative number of seconds")
				}
			case "application_name":
				if strings.ContainsRune(value, 0) {
					return errors.New("spec.database.customExtraParams.application_name cannot contain a NUL character")
				}
			default:
				return fmt.Errorf("spec.database.customExtraParams contains unsupported PostgreSQL parameter %q", key)
			}
			options[key] = value
		}
	}

	extraParams, err := json.Marshal(options)
	if err != nil {
		return err
	}
	db.ExtraParams = string(extraParams)
	return nil
}

func (p *DSPAParams) setAPIServerPostgreSQLConfig() error {
	db := p.DBConnection
	var options map[string]string
	if err := json.Unmarshal([]byte(db.ExtraParams), &options); err != nil {
		return fmt.Errorf("decode PostgreSQL connection options: %w", err)
	}
	options["search_path"] = pgx.Identifier{db.Schema}.Sanitize()
	if options["sslmode"] == "verify-full" {
		options["sslrootcert"] = "system"
		if p.CustomCABundle != nil {
			options["sslrootcert"] = p.PiplinesCABundleMountPath
		}
	}
	extraParams, err := json.Marshal(options)
	if err != nil {
		return err
	}

	connection := map[string]string{
		"Host":   db.Host,
		"Port":   db.Port,
		"User":   db.Username,
		"DBName": db.DBName,
	}
	configJSON := map[string]any{"SkipDatabaseCreation": true, "ExtraParams": json.RawMessage(extraParams)}
	env := map[string]string{
		"DBDRIVERNAME": "pgx",
		"DBCONFIG_POSTGRESQLCONFIG_SKIPDATABASECREATION": "true",
		"DBCONFIG_POSTGRESQLCONFIG_EXTRAPARAMS":          strings.ReplaceAll(string(extraParams), "$", "$$"),
	}
	for key, value := range connection {
		configJSON[key] = value
		// Escape dollar signs so Kubernetes passes literal connection settings.
		env["DBCONFIG_POSTGRESQLCONFIG_"+strings.ToUpper(key)] = strings.ReplaceAll(value, "$", "$$")
	}
	encoded, err := json.Marshal(configJSON)
	if err != nil {
		return err
	}
	p.APIServerPostgreSQLConfigJSON = string(encoded)
	p.APIServerPostgreSQLEnv = env
	return nil
}

func postgreSQLHealthCheckConfig(db DBConnection, password string, options map[string]string, caCerts [][]byte) (*pgx.ConnConfig, error) {
	params := url.Values{
		"host":                 {db.Host},
		"port":                 {db.Port},
		"user":                 {db.Username},
		"password":             {password},
		"dbname":               {db.DBName},
		"sslmode":              {options["sslmode"]},
		"sslrootcert":          {""},
		"sslcert":              {""},
		"sslkey":               {""},
		"sslnegotiation":       {"postgres"},
		"target_session_attrs": {"any"},
	}
	if params.Get("sslmode") == "verify-full" {
		params.Set("sslrootcert", "system")
	}
	connectTimeout := options["connect_timeout"]
	if connectTimeout == "" {
		connectTimeout = "0"
	}
	params.Set("connect_timeout", connectTimeout)

	cfg, err := pgx.ParseConfig("postgresql://?" + params.Encode())
	if err != nil {
		// Driver configuration errors can embed the DSN.
		return nil, errors.New("could not configure PostgreSQL connection or TLS trust")
	}
	if cfg.TLSConfig != nil {
		cfg.TLSConfig.MinVersion = tls.VersionTLS12
	}
	if params.Get("sslmode") == "verify-full" {
		for _, pem := range caCerts {
			if !cfg.TLSConfig.RootCAs.AppendCertsFromPEM(pem) {
				return nil, errors.New("PostgreSQL TLS configuration contains an invalid CA certificate")
			}
		}
	}
	// Keep the probe independent of any session settings inherited by the operator.
	cfg.RuntimeParams = map[string]string{"search_path": pgx.Identifier{db.Schema}.Sanitize()}
	if name := options["application_name"]; name != "" {
		cfg.RuntimeParams["application_name"] = name
	}
	return cfg, nil
}

func checkPostgreSQL(db DBConnection, password string, options map[string]string, caCerts [][]byte, timeout time.Duration) error {
	cfg, err := postgreSQLHealthCheckConfig(db, password, options, caCerts)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return postgreSQLConnectionError(err, db.DBName)
	}
	defer conn.Close(ctx)

	const query = `SELECT pg_catalog.has_schema_privilege(oid, 'USAGE'),
		pg_catalog.has_schema_privilege(oid, 'CREATE')
		FROM pg_catalog.pg_namespace WHERE nspname::text = $1`
	var canUseSchema, canCreateInSchema bool
	err = conn.QueryRow(ctx, query, db.Schema).Scan(&canUseSchema, &canCreateInSchema)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("PostgreSQL schema %q does not exist in database %q", db.Schema, db.DBName)
	case err != nil:
		return fmt.Errorf("could not check PostgreSQL schema %q in database %q", db.Schema, db.DBName)
	case !canUseSchema:
		return fmt.Errorf("PostgreSQL role requires USAGE on schema %q in database %q", db.Schema, db.DBName)
	case !canCreateInSchema:
		return fmt.Errorf("PostgreSQL role requires CREATE on schema %q in database %q", db.Schema, db.DBName)
	default:
		return nil
	}
}

func postgreSQLConnectionError(err error, database string) error {
	if errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) {
		return fmt.Errorf("PostgreSQL connection to database %q timed out", database)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case strings.HasPrefix(pgErr.Code, "28"):
			return errors.New("PostgreSQL authentication failed")
		case pgErr.Code == "3D000":
			return fmt.Errorf("PostgreSQL database %q does not exist", database)
		case pgErr.Code == "42501":
			return fmt.Errorf("PostgreSQL role cannot connect to database %q", database)
		}
	}
	var certErr *tls.CertificateVerificationError
	// pgx reports a server's refusal to negotiate TLS as an untyped error.
	if errors.As(err, &certErr) || strings.Contains(err.Error(), "tls error:") {
		return errors.New("PostgreSQL TLS negotiation or certificate verification failed")
	}
	return fmt.Errorf("could not connect to PostgreSQL database %q", database)
}
