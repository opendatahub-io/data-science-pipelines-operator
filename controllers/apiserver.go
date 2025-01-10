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

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	v1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
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

func (r *DSPAReconciler) ReconcileAPIServer(ctx context.Context, dsp *dspav1.DataSciencePipelinesApplication, params *DSPAParams) error {
	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if !dsp.Spec.APIServer.Deploy {
		r.Log.Info("Skipping Application of APIServer Resources")
		return nil
	}

	log.Info("Applying APIServer Resources")
	err := r.ApplyDir(dsp, params, apiServerTemplatesDir)
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
