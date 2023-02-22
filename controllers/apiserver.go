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
	ctrl "sigs.k8s.io/controller-runtime"
)

var apiServerTemplates = []string{
	"apiserver/artifact_script.yaml.tmpl",
	"apiserver/role_ds-pipeline.yaml.tmpl",
	"apiserver/role_pipeline-runner.yaml.tmpl",
	"apiserver/rolebinding_ds-pipeline.yaml.tmpl",
	"apiserver/rolebinding_pipeline-runner.yaml.tmpl",
	"apiserver/sa_ds-pipeline.yaml.tmpl",
	"apiserver/sa_pipeline-runner.yaml.tmpl",
	"apiserver/service.yaml.tmpl",
	"apiserver/deployment.yaml.tmpl",
	"apiserver/mlpipeline-minio-artifact.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcileAPIServer(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline, req ctrl.Request, params *DSPipelineParams) error {

	if dsp.Spec.APIServer.Deploy == false {
		r.Log.Info("Skipping Application of APIServer Resources")
		return nil
	}

	r.Log.Info("Applying APIServer Resources")

	for _, template := range apiServerTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying APIServer Resources")
	return nil
}
