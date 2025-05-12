/*

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
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	dspa "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	v1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/types"
)

var apiServerTemplatesDir = "apiserver/default"

const apiServerDefaultResourceNamePrefix = "ds-pipeline-"

// serverRoute is a resource deployed conditionally
// as such it is handled separately
const serverRoute = "apiserver/route/route.yaml.tmpl"

// Sample Pipeline and Config are resources deployed conditionally
// as such it is handled separately
var samplePipelineTemplates = map[string]string{
	"sample-pipeline": "apiserver/sample-pipeline/sample-pipeline.yaml.tmpl",
	"sample-config":   "apiserver/sample-pipeline/sample-config.yaml.tmpl",
}

func (r *DSPAReconciler) GenerateSamplePipelineMetadataBlock(pipeline string) (map[string]string, error) {

	item := make(map[string]string)

	// Get Required Fields
	pName, err := config.GetStringConfig(fmt.Sprintf("ManagedPipelinesMetadata.%s.Name", pipeline))
	if err != nil {
		return nil, err
	}
	pFile, err := config.GetStringConfig(fmt.Sprintf("ManagedPipelinesMetadata.%s.Filepath", pipeline))
	if err != nil {
		return nil, err
	}
	platformVersion := config.GetStringConfigWithDefault("DSPO.PlatformVersion", config.DefaultPlatformVersion)

	// Get optional fields
	pDesc := config.GetStringConfigWithDefault(fmt.Sprintf("ManagedPipelinesMetadata.%s.Description", pipeline), "")
	pVerName := config.GetStringConfigWithDefault(fmt.Sprintf("ManagedPipelinesMetadata.%s.VersionName", pipeline), pName)
	pVerDesc := config.GetStringConfigWithDefault(fmt.Sprintf("ManagedPipelinesMetadata.%s.VersionDescription", pipeline), "")

	// Create Sample Config item
	item["name"] = pName
	item["file"] = pFile
	item["description"] = pDesc
	item["versionName"] = fmt.Sprintf("%s - %s", pVerName, strings.Trim(platformVersion, "\""))
	item["versionDescription"] = pVerDesc

	return item, nil

}

func (r *DSPAReconciler) GetSampleConfig(dsp *dspa.DataSciencePipelinesApplication) (string, error) {
	// Check if InstructLab Pipeline enabled in this DSPA
	enableInstructLabPipeline := false
	if dsp.Spec.APIServer.ManagedPipelines != nil && dsp.Spec.APIServer.ManagedPipelines.InstructLab != nil {
		settingInDSPA := dsp.Spec.APIServer.ManagedPipelines.InstructLab.State
		if settingInDSPA != "" {
			enableInstructLabPipeline = strings.EqualFold(string(settingInDSPA), "Managed")
		}
	}

	return r.generateSampleConfigJSON(enableInstructLabPipeline, dsp.Spec.APIServer.EnableSamplePipeline)
}

func (r *DSPAReconciler) generateSampleConfigJSON(enableInstructLabPipeline, enableIrisPipeline bool) (string, error) {

	// Now generate a sample config
	var pipelineConfig = make([]map[string]string, 0)
	if enableInstructLabPipeline {
		item, err := r.GenerateSamplePipelineMetadataBlock("instructlab")
		if err != nil {
			return "", err
		}
		pipelineConfig = append(pipelineConfig, item)
	}
	if enableIrisPipeline {
		item, err := r.GenerateSamplePipelineMetadataBlock("iris")
		if err != nil {
			return "", err
		}
		pipelineConfig = append(pipelineConfig, item)
	}

	var sampleConfig = make(map[string]any)
	sampleConfig["pipelines"] = pipelineConfig
	sampleConfig["loadSamplesOnRestart"] = true

	// Marshal into a JSON String
	outputJSON, err := json.Marshal(sampleConfig)
	if err != nil {
		return "", err
	}

	return string(outputJSON), nil
}

func (r *DSPAReconciler) ReconcileAPIServer(ctx context.Context, dsp *dspav1.DataSciencePipelinesApplication, params *DSPAParams) error {
	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if !dsp.Spec.APIServer.Deploy {
		r.Log.Info("Skipping Application of APIServer Resources")
		return nil
	}

	log.Info("Generating Sample Config")
	sampleConfigJSON, err := r.GetSampleConfig(dsp)
	if err != nil {
		return err
	}
	params.SampleConfigJSON = sampleConfigJSON

	// Generate configuration hash for rebooting on sample changes
	params.APIServerConfigHash = fmt.Sprintf("%x", sha256.Sum256([]byte(sampleConfigJSON)))

	log.Info("Applying APIServer Resources")
	err = r.ApplyDir(dsp, params, apiServerTemplatesDir)
	if err != nil {
		return err
	}

	if dsp.Spec.APIServer.EnableRoute {
		err := r.Apply(dsp, params, serverRoute)
		if err != nil {
			return err
		}
	} else {
		route := &v1.Route{}
		namespacedNamed := types.NamespacedName{Name: "ds-pipeline-" + dsp.Name, Namespace: dsp.Namespace}
		err := r.DeleteResourceIfItExists(ctx, route, namespacedNamed)
		if err != nil {
			return err
		}
	}

	for _, template := range samplePipelineTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	log.Info("Finished applying APIServer Resources")
	return nil
}
