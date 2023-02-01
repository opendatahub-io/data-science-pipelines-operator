package controllers

import (
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
)

var scheduledWorkflowTemplates = []string{
	"config/internal/scheduled-workflow/deployment.yaml.tmpl",
	"config/internal/scheduled-workflow/role.yaml.tmpl",
	"config/internal/scheduled-workflow/rolebinding.yaml.tmpl",
	"config/internal/scheduled-workflow/sa.yaml.tmpl",
}

var scheduledWorkflowClusterScopedTemplates = []string{
	"config/internal/scheduled-workflow/clusterrole.yaml.tmpl",
	"config/internal/scheduled-workflow/clusterrolebinding.yaml.tmpl",
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

	// Namespace resource should not own a cluster scoped resource
	for _, template := range scheduledWorkflowClusterScopedTemplates {
		err := r.ApplyWithoutOwner(params, template)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying ScheduledWorkflow Resources")
	return nil
}

func (r *DSPipelineReconciler) CleanUpScheduledWorkflow(params *DSPipelineParams) error {
	for _, template := range scheduledWorkflowClusterScopedTemplates {
		err := r.DeleteResource(params, template)
		if err != nil {
			return err
		}
	}
	return nil
}
