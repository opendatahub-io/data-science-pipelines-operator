package controllers

import (
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
)

var persistenceAgentTemplates = []string{
	"config/internal/persistence-agent/deployment.yaml.tmpl",
	"config/internal/persistence-agent/sa.yaml.tmpl",
	"config/internal/persistence-agent/role.yaml.tmpl",
	"config/internal/persistence-agent/rolebinding.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcilePersistenceAgent(dsp *dspipelinesiov1alpha1.DSPipeline,
	params *DSPipelineParams) error {

	r.Log.Info("Applying PersistenceAgent Resources")

	for _, template := range persistenceAgentTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying PersistenceAgent Resources")
	return nil
}
