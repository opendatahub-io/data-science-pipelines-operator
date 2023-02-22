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

var mlPipelineUITemplates = []string{
	"mlpipelines-ui/configmap.yaml.tmpl",
	"mlpipelines-ui/deployment.yaml.tmpl",
	"mlpipelines-ui/role.yaml.tmpl",
	"mlpipelines-ui/rolebinding.yaml.tmpl",
	"mlpipelines-ui/route.yaml.tmpl",
	"mlpipelines-ui/sa-ds-pipeline-ui.yaml.tmpl",
	"mlpipelines-ui/sa_ds-pipelines-viewer.yaml.tmpl",
	"mlpipelines-ui/service.yaml.tmpl",
}

var mlPipelineUIClusterScopedTemplates = []string{
	"mlpipelines-ui/clusterrolebinding.yaml.tmpl",
}

const uIClusterRolebindingTemplate = "mlpipelines-ui/clusterrolebinding.yaml.tmpl"

func (r *DSPipelineReconciler) ReconcileUI(dsp *dspipelinesiov1alpha1.DSPipeline,
	params *DSPipelineParams) error {

	if dsp.Spec.MlPipelineUI.Deploy == false {
		r.Log.Info("Skipping Application of MlPipelineUI Resources")
		return nil
	}

	r.Log.Info("Applying MlPipelineUI Resources")
	for _, template := range mlPipelineUITemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	err := r.ApplyWithoutOwner(params, uIClusterRolebindingTemplate)
	if err != nil {
		return err
	}

	r.Log.Info("Applying MlPipelineUI Resources")
	return nil
}

func (r *DSPipelineReconciler) CleanUpUI(params *DSPipelineParams) error {
	for _, template := range mlPipelineUIClusterScopedTemplates {
		err := r.DeleteResource(params, template)
		if err != nil {
			return err
		}
	}
	return nil
}
