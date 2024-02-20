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

	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	v1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const apiServerDefaultResourceNamePrefix = "ds-pipeline-"

var apiServerTemplates = []string{
	"apiserver/artifact_script.yaml.tmpl",
	"apiserver/server-config.yaml.tmpl",
	"apiserver/role_ds-pipeline.yaml.tmpl",
	"apiserver/role_pipeline-runner.yaml.tmpl",
	"apiserver/role_ds-pipeline-user-access.yaml.tmpl",
	"apiserver/rolebinding_ds-pipeline.yaml.tmpl",
	"apiserver/rolebinding_pipeline-runner.yaml.tmpl",
	"apiserver/sa_ds-pipeline.yaml.tmpl",
	"apiserver/sa_pipeline-runner.yaml.tmpl",
	"apiserver/service.yaml.tmpl",
	"apiserver/deployment.yaml.tmpl",
	"apiserver/monitor.yaml.tmpl",
}

// serverRoute is a resource deployed conditionally
// as such it is handled separately
const serverRoute = "apiserver/route.yaml.tmpl"

// Sample Pipeline and Config are resources deployed conditionally
// as such it is handled separately
var samplePipelineTemplates = map[string]string{
	"sample-pipeline": "apiserver/sample-pipeline.yaml.tmpl",
	"sample-config":   "apiserver/sample-config.yaml.tmpl",
}

func (r *DSPAReconciler) ReconcileAPIServer(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication, params *DSPAParams) error {
	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if !dsp.Spec.APIServer.Deploy {
		r.Log.Info("Skipping Application of APIServer Resources")
		return nil
	}

	log.Info("Applying APIServer Resources")

	for _, template := range apiServerTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
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

	for cmName, template := range samplePipelineTemplates {
		if dsp.Spec.APIServer.EnableSamplePipeline {
			err := r.Apply(dsp, params, template)
			if err != nil {
				return err
			}
		} else {
			cm := &corev1.ConfigMap{}
			namespacedNamed := types.NamespacedName{Name: cmName + "-" + dsp.Name, Namespace: dsp.Namespace}
			err := r.DeleteResourceIfItExists(ctx, cm, namespacedNamed)
			if err != nil {
				return err
			}
		}
	}

	log.Info("Finished applying APIServer Resources")
	return nil
}
