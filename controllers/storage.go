/*
Copyright 2023.

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

const storageSecret = "minio/secret.yaml.tmpl"

var storageTemplates = []string{
	"minio/deployment.yaml.tmpl",
	"minio/pvc.yaml.tmpl",
	"minio/service.yaml.tmpl",
	storageSecret,
}

// ReconcileStorage will set up Storage Connection.
func (r *DSPAReconciler) ReconcileStorage(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	storageSpecified := dsp.Spec.ObjectStorage != nil
	// Storage field can be specified as an empty obj, confirm that subfields are also specified
	externalStorageSpecified := params.UsingExternalStorage(dsp)
	minioSpecified := !storageSpecified || dsp.Spec.ObjectStorage.Minio != nil
	deployMinio := !storageSpecified || (minioSpecified && dsp.Spec.ObjectStorage.Minio.Deploy)

	// If external storage is specified, it takes precedence
	if externalStorageSpecified {
		r.Log.Info("Deploying external storage secret.")
		// If using external storage, we just need to create the secret
		// for apiserver
		err := r.Apply(dsp, params, storageSecret)
		if err != nil {
			return err
		}
	} else if deployMinio {
		r.Log.Info("Applying object storage resources.")
		for _, template := range storageTemplates {
			err := r.Apply(dsp, params, template)
			if err != nil {
				return err
			}
		}
		// If no storage was not specified, deploy minio by default.
		// Update the CR with the state of minio to accurately portray
		// desired state.
		if !storageSpecified {
			dsp.Spec.ObjectStorage = &dspav1alpha1.ObjectStorage{}
			dsp.Spec.ObjectStorage.Minio = params.Minio.DeepCopy()
			dsp.Spec.ObjectStorage.Minio.Deploy = true
			if err := r.Update(ctx, dsp); err != nil {
				return err
			}
		}
	} else {
		r.Log.Info("No externalstorage detected, and minio disabled. " +
			"skipping application of storage Resources")
		return nil
	}
	r.Log.Info("Finished applying storage Resources")

	return nil
}
