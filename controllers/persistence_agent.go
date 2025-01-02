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
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
)

var persistenceAgentTemplatesDir = "persistence-agent"

const persistenceAgentDefaultResourceNamePrefix = "ds-pipeline-persistenceagent-"

func (r *DSPAReconciler) ReconcilePersistenceAgent(dsp *dspav1.DataSciencePipelinesApplication,
	params *DSPAParams) (status string, err error) {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if !dsp.Spec.PersistenceAgent.Deploy {
		log.Info("Skipping Application of PersistenceAgent Resources")
		return "Skipped Application PersistenceAgent Resources", nil
	}

	log.Info("Applying PersistenceAgent Resources")

	err = r.ApplyDir(dsp, params, persistenceAgentTemplatesDir)
	if err != nil {
		return "Failed to apply PersistenceAgent Resources", err
	}

	log.Info("Finished applying PersistenceAgent Resources")
	return "PersistenceAgent Resources Applied", nil
}
