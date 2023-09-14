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
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
)

const persistenceAgentDefaultResourceNamePrefix = "ds-pipeline-persistenceagent-"

var persistenceAgentTemplates = []string{
	"persistence-agent/deployment.yaml.tmpl",
	"persistence-agent/sa.yaml.tmpl",
	"persistence-agent/role.yaml.tmpl",
	"persistence-agent/rolebinding.yaml.tmpl",
}

func (r *DSPAReconciler) ReconcilePersistenceAgent(dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if !dsp.Spec.PersistenceAgent.Deploy {
		log.Info("Skipping Application of PersistenceAgent Resources")
		return nil
	}

	log.Info("Applying PersistenceAgent Resources")

	err := r.ApplyAll(dsp, params, persistenceAgentTemplates)
	if err != nil {
		return err
	}

	log.Info("Finished applying PersistenceAgent Resources")
	return nil
}
