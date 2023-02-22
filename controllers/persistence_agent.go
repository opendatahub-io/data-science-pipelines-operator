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

var persistenceAgentTemplates = []string{
	"persistence-agent/deployment.yaml.tmpl",
	"persistence-agent/sa.yaml.tmpl",
	"persistence-agent/role.yaml.tmpl",
	"persistence-agent/rolebinding.yaml.tmpl",
}

func (r *DSPipelineReconciler) ReconcilePersistenceAgent(dsp *dspipelinesiov1alpha1.DSPipeline,
	params *DSPipelineParams) error {

	if dsp.Spec.PersistenceAgent.Deploy == false {
		r.Log.Info("Skipping Application of PersistenceAgent Resources")
		return nil
	}

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
