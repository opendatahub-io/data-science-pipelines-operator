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
	cryptoTls "crypto/tls"
	"crypto/x509"
	"database/sql"
	b64 "encoding/base64"
	"fmt"

	"time"

	"errors"

	"github.com/go-logr/logr"
	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"k8s.io/apimachinery/pkg/util/json"
	"os"
)

const dbSecret = "mariadb/generated-secret/secret.yaml.tmpl"

var mariadbTemplates = []string{
	"mariadb/default/deployment.yaml.tmpl",
	"mariadb/default/pvc.yaml.tmpl",
	"mariadb/default/service.yaml.tmpl",
	"mariadb/default/mariadb-sa.yaml.tmpl",
	"mariadb/default/networkpolicy.yaml.tmpl",
}

func tLSClientConfig(pems [][]byte) (*cryptoTls.Config, error) {
	rootCertPool := x509.NewCertPool()

	if f := os.Getenv("SSL_CERT_FILE"); f != "" {
		data, err := os.ReadFile(f)
		if err == nil {
			rootCertPool.AppendCertsFromPEM(data)
		}
	}

	for _, pem := range pems {
		if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
			return nil, fmt.Errorf("error parsing CA Certificate, ensure provided certs are in valid PEM format")
		}
	}

	tlsConfig := &cryptoTls.Config{
		RootCAs: rootCertPool,
	}
	return tlsConfig, nil
}

func createMySQLConfig(user, password string, mysqlServiceHost string,
	mysqlServicePort string, dbName string, mysqlExtraParams map[string]string) *mysql.Config {

	params := map[string]string{
		"charset":   "utf8",
		"parseTime": "True",
		"loc":       "Local",
	}

	for k, v := range mysqlExtraParams {
		params[k] = v
	}

	return &mysql.Config{
		User:                 user,
		Passwd:               password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%s", mysqlServiceHost, mysqlServicePort),
		Params:               params,
		DBName:               dbName,
		AllowNativePasswords: true,
	}
}

var ConnectAndQueryDatabase = func(
	host string,
	log logr.Logger,
	port, username, password, dbname, tls string,
	dbConnectionTimeout time.Duration,
	pemCerts [][]byte,
	extraParams map[string]string) (bool, error) {

	mysqlConfig := createMySQLConfig(
		username,
		password,
		host,
		port,
		"",
		extraParams,
	)

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), dbConnectionTimeout)
	defer cancel()

	var tlsConfig *cryptoTls.Config
	switch tls {
	case "false", "":
		// don't set anything
	case "true":
		var err error
		// if pemCerts is empty, that is OK, we still add OS certs to the tls config
		tlsConfig, err = tLSClientConfig(pemCerts)
		if err != nil {
			log.Info(fmt.Sprintf("Encountered error when processing custom ca bundle, Error: %v", err))
			return false, err
		}
	case "skip-verify", "preferred":
		tlsConfig = &cryptoTls.Config{InsecureSkipVerify: true}
	default:
		// Unknown config, default to don't set anything
	}

	// Only register tls config in the case of: "true", "skip-verify", "preferred"
	if tlsConfig != nil {
		err := mysql.RegisterTLSConfig("custom", tlsConfig)
		// If ExtraParams{"tls": ".."} is set, that should take precedent over mysqlConfig.TLSConfig
		// so we need to make sure we're setting our tls config to be used instead if it exists
		if _, ok := mysqlConfig.Params["tls"]; ok {
			mysqlConfig.Params["tls"] = "custom"
		}
		// Just to be safe, we also set it here, fallback from mysqlConfig.Params["tls"] not being set
		mysqlConfig.TLSConfig = "custom"
		if err != nil {
			return false, err
		}
	}

	db, err := sql.Open("mysql", mysqlConfig.FormatDSN())
	if err != nil {
		return false, err
	}
	defer db.Close()

	testStatement := "SELECT 1;"
	_, err = db.QueryContext(ctx, testStatement)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *DSPAReconciler) isDatabaseAccessible(dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) (bool, error) {
	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if params.DatabaseHealthCheckDisabled(dsp) {
		infoMessage := "Database health check disabled, assuming database is available and ready."
		log.V(1).Info(infoMessage)
		return true, nil
	}

	log.Info("Performing Database Health Check")
	databaseSpecified := dsp.Spec.Database != nil
	usingExternalDB := params.UsingExternalDB(dsp)
	usingMariaDB := !databaseSpecified || dsp.Spec.Database.MariaDB != nil
	if !usingMariaDB && !usingExternalDB {
		errorMessage := "Could not connect to Database: Unsupported Type"
		log.Info(errorMessage)
		return false, errors.New(errorMessage)
	}

	decodePass, _ := b64.StdEncoding.DecodeString(params.DBConnection.Password)
	dbConnectionTimeout := config.GetDurationConfigWithDefault(config.DBConnectionTimeoutConfigName, config.DefaultDBConnectionTimeout)

	var extraParamsJson map[string]string
	err := json.Unmarshal([]byte(params.DBConnection.ExtraParams), &extraParamsJson)
	if err != nil {
		log.Info(fmt.Sprintf("Could not parse tls config in ExtraParams, if setting CustomExtraParams, ensure the JSON string is well-formed. Error: %v", err))
		return false, err
	}

	// tls can be true, false, skip-verify, preferred
	// we default to true if it's an externalDB, false otherwise
	// (if not specified via CustomExtraParams)
	tls := "false"
	if usingExternalDB {
		tls = "true"
	}

	// Override tls with the value in ExtraParams, if specified
	// If users have specified a CustomExtraParams field, have to
	// check for "tls" existence because users may choose to  leave
	// out the "tls" param
	if val, ok := extraParamsJson["tls"]; ok {
		tls = val
	}

	log.V(1).Info(fmt.Sprintf("Attempting Database Heath Check connection (with timeout: %s)", dbConnectionTimeout))

	dbHealthCheckPassed, err := ConnectAndQueryDatabase(
		params.DBConnection.Host,
		log,
		params.DBConnection.Port,
		params.DBConnection.Username,
		string(decodePass),
		params.DBConnection.DBName,
		tls,
		dbConnectionTimeout,
		params.APICustomPemCerts,
		extraParamsJson)

	if err != nil {
		log.Info(fmt.Sprintf("Unable to connect to Database: %v", err))
		return false, err
	}

	if dbHealthCheckPassed {
		log.Info("Database Health Check Successful")
	}

	return dbHealthCheckPassed, err
}

func (r *DSPAReconciler) ReconcileDatabase(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)
	databaseSpecified := dsp.Spec.Database != nil
	// DB field can be specified as an empty obj, confirm that subfields are also specified
	// By default if Database is empty, we deploy mariadb
	externalDBSpecified := params.UsingExternalDB(dsp)
	mariaDBSpecified := dsp.Spec.Database.MariaDB != nil
	defaultDBRequired := !databaseSpecified || (!externalDBSpecified && !mariaDBSpecified)

	deployMariaDB := mariaDBSpecified && dsp.Spec.Database.MariaDB.Deploy
	// Default DB is currently MariaDB as well, but storing these bools seperately in case that changes
	deployDefaultDB := !databaseSpecified || defaultDBRequired

	externalDBCredentialsProvided := externalDBSpecified && (dsp.Spec.Database.ExternalDB.PasswordSecret != nil)
	mariaDBCredentialsProvided := mariaDBSpecified && (dsp.Spec.Database.MariaDB.PasswordSecret != nil)
	databaseCredentialsProvided := externalDBCredentialsProvided || mariaDBCredentialsProvided

	// If external db is specified, it takes precedence
	if externalDBSpecified {
		log.Info("Using externalDB, bypassing database deployment.")
	} else if deployMariaDB || deployDefaultDB {
		if !databaseCredentialsProvided {
			err := r.Apply(dsp, params, dbSecret)
			if err != nil {
				return err
			}
		}
		log.Info("Applying mariaDB resources.")
		for _, template := range mariadbTemplates {
			err := r.Apply(dsp, params, template)
			if err != nil {
				return err
			}
		}
		// If no database was not specified, deploy mariaDB by default.
		// Update the CR with the state of mariaDB to accurately portray
		// desired state.
		if !databaseSpecified {
			dsp.Spec.Database = &dspav1alpha1.Database{}
		}
		if !databaseSpecified || defaultDBRequired {
			dsp.Spec.Database.MariaDB = params.MariaDB.DeepCopy()
			dsp.Spec.Database.MariaDB.Deploy = true
			if err := r.Update(ctx, dsp); err != nil {
				return err
			}
		}
	} else {
		log.Info("No externalDB detected, and mariaDB disabled. " +
			"skipping Application of DB Resources")
		return nil
	}
	log.Info("Finished applying Database Resources")

	return nil
}
