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
)

const (
	mlmdTemplatesDir                   = "ml-metadata"
	mlmdEnvoyRoute                     = mlmdTemplatesDir + "/route/metadata-envoy.route.yaml.tmpl"
	mlmdProxyDefaultResourceNamePrefix = "ds-pipeline-scheduledworkflow-"
	mlmdGrpcService                    = "grpc-service"
)

func (r *DSPAReconciler) ReconcileMLMD(ctx context.Context, dsp *dspav1.DataSciencePipelinesApplication,
	params *DSPAParams) (status string, err error) {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if (params.MLMD == nil || !params.MLMD.Deploy) && (dsp.Spec.MLMD == nil || !dsp.Spec.MLMD.Deploy) {
		r.Log.Info("Skipping Application of ML-Metadata (MLMD) Resources")
		return "MLMD Resource Application Skipped", nil
	}

	log.Info("Applying ML-Metadata (MLMD) Resources")

	// We need to create the service first so OpenShift creates the certificate that we'll use later.
	err = r.ApplyDir(dsp, params, mlmdTemplatesDir+"/"+mlmdGrpcService)
	if err != nil {
		return "MLMD Service Failed to create", err
	}

	if params.PodToPodTLS {
		var certificatesExist bool
		certificatesExist, err = params.LoadMlmdCertificates(ctx, r.Client)
		if err != nil {
			return "Failed to load MLMD Certificate", err
		}

		if !certificatesExist {
			return "Secret containing the certificate for MLMD gRPC Server was not created yet", nil
		}
	}

	err = r.ApplyDir(dsp, params, mlmdTemplatesDir)
	if err != nil {
		return "Failed to apply MLMD Resources", err
	}

	if dsp.Spec.MLMD == nil || dsp.Spec.MLMD.Envoy == nil || dsp.Spec.MLMD.Envoy.DeployRoute {
		err = r.Apply(dsp, params, mlmdEnvoyRoute)
		if err != nil {
			return "Failed to apply MLMD Envoy Route", err
		}
	}

	log.Info("Finished applying MLMD Resources")
	return "MLMD Resources Applied", nil
}
