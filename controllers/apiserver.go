package controllers

import (
	"context"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
)

var apiServerTemplates = []string{
	"config/internal/apiserver/artifact_script.yaml.tmpl",
	"config/internal/apiserver/role_ds-pipeline.yaml.tmpl",
	"config/internal/apiserver/role_pipeline-runner.yaml.tmpl",
	"config/internal/apiserver/rolebinding_ds-pipeline.yaml.tmpl",
	"config/internal/apiserver/rolebinding_pipeline-runner.yaml.tmpl",
	"config/internal/apiserver/sa_ds-pipeline.yaml.tmpl",
	"config/internal/apiserver/sa_pipeline-runner.yaml.tmpl",
	"config/internal/apiserver/service.yaml.tmpl",
	"config/internal/apiserver/deployment.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcileAPIServer(dsp *dspipelinesiov1alpha1.DSPipeline, req context.Context, params *DSPipelineParams) error {
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
