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

func (r *DSPAReconciler) GenerateSamplePipelineMetadataBlock(pipeline string, platformVersion string) (map[string]string, error) {

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

	// Get optional fields
	pDesc := config.GetStringConfigWithDefault(fmt.Sprintf("ManagedPipelinesMetadata.%s.Description", pipeline), "")
	pVerName := config.GetStringConfigWithDefault(fmt.Sprintf("ManagedPipelinesMetadata.%s.VersionName", pipeline), pName)
	pVerDesc := config.GetStringConfigWithDefault(fmt.Sprintf("ManagedPipelinesMetadata.%s.VersionDescription", pipeline), "")

	// Create Sample Config item
	item["name"] = pName
	item["file"] = pFile
	item["description"] = pDesc
	item["versionName"] = fmt.Sprintf("%s - %s", pVerName, platformVersion)
	item["versionDescription"] = pVerDesc

	return item, nil
}

// managedPipelineSampleEntry returns a sample_config pipeline entry for the named managed pipeline.
// Uses config (ManagedPipelinesMetadata.<name>) when present; otherwise a minimal entry for volume-loaded YAML.
func (r *DSPAReconciler) managedPipelineSampleEntry(pipelineName string, platformVersion string) map[string]string {
	item, err := r.GenerateSamplePipelineMetadataBlock(pipelineName, platformVersion)
	if err == nil {
		return item
	}
	r.Log.Info("Managed pipeline metadata not found in operator config; using minimal sample_config entry",
		"pipeline", pipelineName, "error", err)
	// No config metadata: use minimal entry so API server loads from /config/managed-pipelines/<name>.yaml
	return map[string]string{
		"name":               pipelineName,
		"file":               fmt.Sprintf("/config/managed-pipelines/%s.yaml", pipelineName),
		"description":        "",
		"versionName":        fmt.Sprintf("%s - %s", pipelineName, platformVersion),
		"versionDescription": "",
	}
}

func (r *DSPAReconciler) generateSampleConfigJSON(dsp *dspav1.DataSciencePipelinesApplication, platformVersion string) (string, error) {
	pipelineConfig := make([]map[string]string, 0)

	if dsp.Spec.APIServer.EnableSamplePipeline {
		item, err := r.GenerateSamplePipelineMetadataBlock("iris", platformVersion)
		if err != nil {
			return "", err
		}
		pipelineConfig = append(pipelineConfig, item)
	}

	// Explicit managed pipeline list: add each to sample_config (API server loads these from sample_config).
	// Omitted list ("all"): do not add managed entries here; API server loads from managed-pipelines.json in volume.
	if mp := dsp.Spec.APIServer.ManagedPipelines; mp != nil && len(mp.Pipelines) > 0 {
		seenManaged := make(map[string]struct{})
		for _, p := range mp.Pipelines {
			key := strings.ToLower(p.Name)
			if _, exists := seenManaged[key]; exists {
				return "", fmt.Errorf("duplicate managed pipeline name %q", p.Name)
			}
			seenManaged[key] = struct{}{}
			if dsp.Spec.APIServer.EnableSamplePipeline && strings.EqualFold(p.Name, "iris") {
				continue // Iris already included above from EnableSamplePipeline
			}
			pipelineConfig = append(pipelineConfig, r.managedPipelineSampleEntry(p.Name, platformVersion))
		}
	}

	sampleConfig := map[string]any{
		"pipelines":            pipelineConfig,
		"loadSamplesOnRestart": true,
	}
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
	sampleConfigJSON, err := r.generateSampleConfigJSON(dsp, params.PlatformVersion)
	if err != nil {
		return err
	}
	params.SampleConfigJSON = sampleConfigJSON

	combinedConfigHashInput := sampleConfigJSON
	if params.APIServerWorkspaceJSON != "" {
		combinedConfigHashInput = sampleConfigJSON + params.APIServerWorkspaceJSON
	}
	if dsp.Spec.APIServer.ManagedPipelines != nil {
		managedSpec, err := json.Marshal(dsp.Spec.APIServer.ManagedPipelines)
		if err != nil {
			return err
		}
		combinedConfigHashInput = combinedConfigHashInput + string(managedSpec)
	}

	// Generate configuration hash for rebooting on sample changes
	params.APIServerConfigHash = fmt.Sprintf("%x", sha256.Sum256([]byte(combinedConfigHashInput)))

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
