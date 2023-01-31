package controllers

import (
	"context"
	"fmt"
	mf "github.com/manifestival/manifestival"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
	"github.com/opendatahub-io/ds-pipelines-controller/controllers/config"
)

var templates = []string{
	"config/internal/apiserver/deployment.yaml.tmpl",
	"config/internal/apiserver/service.yaml.tmpl",
	"config/internal/apiserver/sa.yaml.tmpl",
	"config/internal/apiserver/artifact_script.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcileAPIServer(dsp *dspipelinesiov1alpha1.DSPipeline, req context.Context, params *DSPipelineParams) error {
	r.Log.Info("Applying APIServer apiServerFields")

	for _, template := range templates {
		tmplManifest, err := config.Manifest(r.Client, template, params)
		if err != nil {
			return fmt.Errorf("error loading template yaml: %w", err)
		}
		tmplManifest, err = tmplManifest.Transform(
			mf.InjectOwner(dsp),
		)
		if err = tmplManifest.Apply(); err != nil {
			return err
		}
	}
	return nil
}
