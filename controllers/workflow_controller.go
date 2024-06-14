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

var workflowControllerTemplatesDir = "workflow-controller"

func (r *DSPAReconciler) ReconcileWorkflowController(dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if dsp.Spec.WorkflowController == nil || !dsp.Spec.WorkflowController.Deploy {
		log.Info("Skipping Application of WorkflowController Resources")
		return nil
	}

	log.Info("Applying WorkflowController Resources")

	err := r.ApplyDir(dsp, params, workflowControllerTemplatesDir)
	if err != nil {
		return err
	}

	log.Info("Finished applying WorkflowController Resources")
	return nil
}
