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
	"fmt"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var apiServerTemplates = []string{
	"apiserver/artifact_script.yaml.tmpl",
	"apiserver/role_ds-pipeline.yaml.tmpl",
	"apiserver/role_pipeline-runner.yaml.tmpl",
	"apiserver/rolebinding_ds-pipeline.yaml.tmpl",
	"apiserver/rolebinding_pipeline-runner.yaml.tmpl",
	"apiserver/sa_ds-pipeline.yaml.tmpl",
	"apiserver/sa_pipeline-runner.yaml.tmpl",
	"apiserver/service.yaml.tmpl",
	"apiserver/deployment.yaml.tmpl",
}

// Resources that are deployed conditionally
// as such they are handled separately

const serverRoute = "apiserver/route.yaml.tmpl"
const apiServerCookieSecret = "apiserver/secret.yaml.tmpl"

func (r *DSPAReconciler) ReconcileAPIServer(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication, params *DSPAParams) error {

	if !dsp.Spec.APIServer.Deploy {
		r.Log.Info("Skipping Application of APIServer Resources")
		return nil
	}

	r.Log.Info("Applying APIServer Resources")

	for _, template := range apiServerTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	cookieSecret := &corev1.Secret{}
	cookieSecretNN := types.NamespacedName{
		Name:      fmt.Sprintf("ds-pipelines-%s-cookie-secret", dsp.Name),
		Namespace: dsp.Namespace,
	}

	if dsp.Spec.APIServer.EnableRoute {
		err := r.Apply(dsp, params, serverRoute)
		if err != nil {
			return err
		}
		err = r.CreateIfDoesNotItExists(ctx, cookieSecret, cookieSecretNN, params, apiServerCookieSecret, dsp)
		if err != nil {
			return err
		}

	} else {
		route := &routev1.Route{}
		routeNN := types.NamespacedName{Name: "ds-pipeline-" + dsp.Name, Namespace: dsp.Namespace}
		err := r.DeleteResourceIfItExists(ctx, route, routeNN)
		if err != nil {
			return err
		}

		err = r.DeleteResourceIfItExists(ctx, cookieSecret, cookieSecretNN)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying APIServer Resources")
	return nil
}
