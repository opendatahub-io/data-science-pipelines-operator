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
	"encoding/json"
	"fmt"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
)

var workflowControllerTemplatesDir = "workflow-controller"

const workflowControllerDefaultResourceNamePrefix = "ds-pipeline-workflow-controller-"

type ArgoWorkflowsControllersConfig struct {
	ManagementState string `json:"managementState"`
}

func (c *ArgoWorkflowsControllersConfig) GetManagementState() string {
	if c.ManagementState == "" {
		return config.DefaultArgoWorkflowsControllersManagementState
	}
	return c.ManagementState
}

func (r *DSPAReconciler) ReconcileWorkflowController(dsp *dspav1.DataSciencePipelinesApplication,
	params *DSPAParams) (bool, error) {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	// Get the management state for the WorkflowController subcomponent from the config
	// Expected format example: {"managementState":"Managed"}
	dspoArgoWorkflowsControllersJSON := config.GetStringConfigWithDefault("DSPO.ArgoWorkflowsControllers", config.DefaultArgoWorkflowsControllers)

	var argoWorkflowsControllersConfig ArgoWorkflowsControllersConfig
	if err := json.Unmarshal([]byte(dspoArgoWorkflowsControllersJSON), &argoWorkflowsControllersConfig); err != nil {
		log.Info(fmt.Sprintf("Unable to parse Argo Workflows Controller management state, using default value: %s", config.DefaultArgoWorkflowsControllersManagementState))
		log.Info(fmt.Sprintf("Error: %s", err))
		argoWorkflowsControllersConfig = ArgoWorkflowsControllersConfig{
			ManagementState: config.DefaultArgoWorkflowsControllersManagementState,
		}
	}

	// Conditionally deploy the WorkflowController resource depending on the speciified management state
	// Managed (or blank) - deploy the WorkflowController subcomponent
	// Removed - skip deploying, and remove if already present, the WorkflowController subcomponent
	// All other values - Invalid configuration, return an error
	workflowControllerEnabled := false
	switch argoWorkflowsControllersConfig.GetManagementState() {
	case "Managed", "":
		if dsp.Spec.WorkflowController == nil || !dsp.Spec.WorkflowController.Deploy {
			log.Info("Skipping Application of WorkflowController Resources")
			return workflowControllerEnabled, nil
		}

		log.Info("Applying WorkflowController Resources")
		workflowControllerEnabled = true
		err := r.ApplyDir(dsp, params, workflowControllerTemplatesDir)
		if err != nil {
			return workflowControllerEnabled, err
		}

	case "Removed":
		log.Info("Removing WorkflowController Resources (if present)")
		err := r.DeleteResourceDir(params, workflowControllerTemplatesDir)
		if err != nil {
			return workflowControllerEnabled, err
		}

	default:
		err := fmt.Errorf("invalid management state for WorkflowController Resources: %s", argoWorkflowsControllersConfig.GetManagementState())
		return workflowControllerEnabled, err
	}

	log.Info("Finished applying WorkflowController Resources")
	return workflowControllerEnabled, nil
}
