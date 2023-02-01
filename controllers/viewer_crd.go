package controllers

import (
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
)

var viewerCRDTemplates = []string{
	"config/internal/viewer-crd/deployment.yaml.tmpl",
	"config/internal/viewer-crd/role.yaml.tmpl",
	"config/internal/viewer-crd/rolebinding.yaml.tmpl",
	"config/internal/viewer-crd/sa.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcileViewerCRD(dsp *dspipelinesiov1alpha1.DSPipeline,
	params *DSPipelineParams) error {

	r.Log.Info("Applying ViewerCRD Resources")

	for _, template := range viewerCRDTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying ViewerCRD Resources")
	return nil
}
