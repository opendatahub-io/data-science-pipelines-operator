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
	v1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	mlmdTemplatesDir = "ml-metadata"
	mlmdEnvoyRoute   = mlmdTemplatesDir + "/route/metadata-envoy.route.yaml.tmpl"
)

func (r *DSPAReconciler) ReconcileMLMD(dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if (params.MLMD == nil || !params.MLMD.Deploy) && (dsp.Spec.MLMD == nil || !dsp.Spec.MLMD.Deploy) {
		r.Log.Info("Skipping Application of ML-Metadata (MLMD) Resources")
		return nil
	}

	log.Info("Applying ML-Metadata (MLMD) Resources")

	if params.UsingV1Pipelines(dsp) {
		if dsp.Spec.MLMD != nil {
			err := r.ApplyDir(dsp, params, mlmdTemplatesDir)
			if err != nil {
				return err
			}

			if dsp.Spec.MLMD.Envoy == nil || dsp.Spec.MLMD.Envoy.DeployRoute {
				err = r.Apply(dsp, params, mlmdEnvoyRoute)
				if err != nil {
					return err
				}
			}
		}

		err := r.ApplyDir(dsp, params, mlmdTemplatesDir+"/v1")
		if err != nil {
			return err
		}
	} else {
		err := r.ApplyDir(dsp, params, mlmdTemplatesDir)
		if err != nil {
			return err
		}

		if dsp.Spec.MLMD == nil || dsp.Spec.MLMD.Envoy == nil || dsp.Spec.MLMD.Envoy.DeployRoute {
			err = r.Apply(dsp, params, mlmdEnvoyRoute)
			if err != nil {
				return err
			}
		}
	}

	log.Info("Finished applying MLMD Resources")
	return nil
}

func (r *DSPAReconciler) GetEnvoyServiceHostname(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication) (string, error) {
	service := &corev1.Service{}
	namespacedNamed := types.NamespacedName{Name: "ds-pipeline-md-" + dsp.Name, Namespace: dsp.Namespace}
	err := r.Get(ctx, namespacedNamed, service)
	if err != nil {
		return "", err
	}

	// Loop over all Service ports, if a secured port is found
	// set port and scheme to its secured ones and skip the loop
	serviceScheme := ""
	servicePort := ""
	for i := 0; i < len(service.Spec.Ports); i++ {
		servicePort = fmt.Sprintf("%d", service.Spec.Ports[i].Port)
		if servicePort == "8443" || servicePort == "443" {
			// If a secured port is found, just set scheme to 'https://' and skip the loop
			serviceScheme = "https://"
			break
		} else {
			serviceScheme = "http://"
		}
	}

	return serviceScheme + service.Name + "." + service.Namespace + ".svc.cluster.local:" + servicePort, nil
}

func (r *DSPAReconciler) GetEnvoyRouteHostname(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication) (string, error) {
	route := &v1.Route{}
	namespacedNamed := types.NamespacedName{Name: "ds-pipeline-md-" + dsp.Name, Namespace: dsp.Namespace}
	err := r.Get(ctx, namespacedNamed, route)
	if err != nil {
		return "", err
	}

	serviceScheme := ""
	if route.Spec.TLS != nil {
		serviceScheme = "https://"
	} else {
		serviceScheme = "http://"
	}

	return serviceScheme + route.Spec.Host, nil
}
