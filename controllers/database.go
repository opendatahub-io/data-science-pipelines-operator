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
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
)

var dbTemplates = []string{
	"mariadb/deployment.yaml.tmpl",
	"mariadb/pvc.yaml.tmpl",
	"mariadb/sa.yaml.tmpl",
	"mariadb/secret.yaml.tmpl",
	"mariadb/service.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcileDatabase(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline,
	params *DSPipelineParams) error {

	// If no database was specified, DSPO will deploy mariaDB by default
	// As such DSPO needs to update the CR with the state of the mariaDB
	// to match desired with live states.
	if dsp.Spec.Database == nil || (dsp.Spec.Database.MariaDB == nil && !params.UsingExternalDB(dsp)) {
		dsp.Spec.Database = &dspipelinesiov1alpha1.Database{}
		dsp.Spec.Database.MariaDB = params.MariaDB.DeepCopy()
		dsp.Spec.Database.MariaDB.Deploy = true
		if err := r.Update(ctx, dsp); err != nil {
			return err
		}
	}

	if dsp.Spec.Database.MariaDB.Deploy == false {
		r.Log.Info("Skipping Application of MariaDB Resources")
		return nil
	}

	r.Log.Info("Applying Database Resources")
	for _, template := range dbTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}
	r.Log.Info("Finished applying Database Resources")

	return nil
}
