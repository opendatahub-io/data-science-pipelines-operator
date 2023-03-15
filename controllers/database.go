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
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
)

const dbSecret = "mariadb/secret.yaml.tmpl"

var dbTemplates = []string{
	"mariadb/deployment.yaml.tmpl",
	"mariadb/pvc.yaml.tmpl",
	"mariadb/sa.yaml.tmpl",
	"mariadb/service.yaml.tmpl",
	dbSecret,
}

func (r *DSPAReconciler) ReconcileDatabase(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	databaseSpecified := dsp.Spec.Database != nil
	// DB field can be specified as an empty obj, confirm that subfields are also specified
	// By default if Database is empty, we deploy mariadb
	externalDBSpecified := params.UsingExternalDB(dsp)
	mariaDBSpecified := !databaseSpecified || dsp.Spec.Database.MariaDB != nil
	deployMariaDB := !databaseSpecified || (mariaDBSpecified && dsp.Spec.Database.MariaDB.Deploy)

	// If external db is specified, it takes precedence
	if externalDBSpecified {
		r.Log.Info("Deploying external db secret.")
		// If using external DB, we just need to create the secret
		// for apiserver
		err := r.Apply(dsp, params, dbSecret)
		if err != nil {
			return err
		}
	} else if deployMariaDB {
		r.Log.Info("Applying mariaDB resources.")
		for _, template := range dbTemplates {
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
			dsp.Spec.Database.MariaDB = params.MariaDB.DeepCopy()
			dsp.Spec.Database.MariaDB.Deploy = true
			if err := r.Update(ctx, dsp); err != nil {
				return err
			}
		}
	} else {
		r.Log.Info("No externalDB detected, and mariaDB disabled. " +
			"skipping Application of DB Resources")
		return nil
	}
	r.Log.Info("Finished applying Database Resources")

	return nil
}
