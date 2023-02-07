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
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
)

var mlPipelineUITemplates = []string{
	"config/internal/mlpipelines-ui/configmap.yaml.tmpl",
	"config/internal/mlpipelines-ui/deployment.yaml.tmpl",
	"config/internal/mlpipelines-ui/role.yaml.tmpl",
	"config/internal/mlpipelines-ui/rolebinding.yaml.tmpl",
	"config/internal/mlpipelines-ui/route.yaml.tmpl",
	"config/internal/mlpipelines-ui/sa-ds-pipeline-ui.yaml.tmpl",
	"config/internal/mlpipelines-ui/sa_ds-pipelines-viewer.yaml.tmpl",
	"config/internal/mlpipelines-ui/service.yaml.tmpl",
}

var mlPipelineUIClusterScopedTemplates = []string{
	"config/internal/mlpipelines-ui/clusterrolebinding.yaml.tmpl",
}

const uIClusterRolebindingTemplate = "config/internal/mlpipelines-ui/clusterrolebinding.yaml.tmpl"

func (r *DSPipelineReconciler) ReconcileUI(dsp *dspipelinesiov1alpha1.DSPipeline,
	params *DSPipelineParams) error {

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
