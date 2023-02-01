package controllers

import (
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
)

var scheduledWorkflowTemplates = []string{
	"config/internal/scheduled-workflow/deployment.yaml.tmpl",
	"config/internal/scheduled-workflow/role.yaml.tmpl",
	"config/internal/scheduled-workflow/rolebinding.yaml.tmpl",
	"config/internal/scheduled-workflow/sa.yaml.tmpl",
	"config/internal/scheduled-workflow/role.yaml.tmpl",
	"config/internal/scheduled-workflow/rolebinding.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcileScheduledWorkflow(dsp *dspipelinesiov1alpha1.DSPipeline,
	params *DSPipelineParams) error {

	r.Log.Info("Applying ScheduledWorkflow Resources")

	for _, template := range scheduledWorkflowTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying ScheduledWorkflow Resources")
	return nil
}
