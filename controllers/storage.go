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
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
)

var storageTemplates = []string{
	"minio/deployment.yaml.tmpl",
	"minio/pvc.yaml.tmpl",
	"minio/service.yaml.tmpl",
	"minio/secret.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcileStorage(dsp *dspipelinesiov1alpha1.DSPipeline,
	params *DSPipelineParams) error {

	if dsp.Spec.ObjectStorage.Minio.Deploy == false {
		r.Log.Info("Skipping Application of ObjectStorage Resources")
		return nil
	}

	r.Log.Info("Applying Storage Resources")

	for _, template := range storageTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying Storage Resources")
	return nil
}
