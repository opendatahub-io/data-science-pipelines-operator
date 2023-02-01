package controllers

import (
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
)

var persistenceAgentTemplates = []string{
	"config/internal/persistence-agent/deployment.yaml.tmpl",
	"config/internal/persistence-agent/sa.yaml.tmpl",
}

var persistenceAgentClusterScopedTemplates = []string{
	"config/internal/persistence-agent/clusterrole.yaml.tmpl",
	"config/internal/persistence-agent/clusterrolebinding.yaml.tmpl",
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

	// Namespace resource should not own a cluster scoped resource
	for _, template := range persistenceAgentClusterScopedTemplates {
		err := r.ApplyWithoutOwner(params, template)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying PersistenceAgent Resources")
	return nil
}

func (r *DSPipelineReconciler) CleanUpPersistenceAgent(params *DSPipelineParams) error {
	for _, template := range persistenceAgentClusterScopedTemplates {
		err := r.DeleteResource(params, template)
		if err != nil {
			return err
		}
	}
	return nil
}
