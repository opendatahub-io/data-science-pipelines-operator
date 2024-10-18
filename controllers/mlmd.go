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
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/util"
)

const (
	mlmdTemplatesDir                   = "ml-metadata"
	mlmdEnvoyRoute                     = mlmdTemplatesDir + "/route/metadata-envoy.route.yaml.tmpl"
	mlmdProxyDefaultResourceNamePrefix = "ds-pipeline-scheduledworkflow-"
	mlmdGrpcService                    = "grpc-service"
)

func (r *DSPAReconciler) ReconcileMLMD(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
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
		// We need to create the service first so OpenShift creates the certificate that we'll use later.
		err := r.ApplyDir(dsp, params, mlmdTemplatesDir+"/"+mlmdGrpcService)
		if err != nil {
			return err
		}

		if params.PodToPodTLS {
			var certificatesExist bool
			certificatesExist, err = params.LoadMlmdCertificates(ctx, r.Client)
			if err != nil {
				return err
			}
			if !certificatesExist {
				return &util.LaggingDependencyCreationError{Message: "MLMD gRPC Server cert secret not found, this is likely because it has not been created yet"}
			}
		}

		err = r.ApplyDir(dsp, params, mlmdTemplatesDir)
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
