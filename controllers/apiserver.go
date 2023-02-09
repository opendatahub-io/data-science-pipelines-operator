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
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
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

// This is hardcoded in kfp-tekton, apiserver will always use this hardcoded secret for tekton resources
const minioArtifactSecret = "apiserver/mlpipeline-minio-artifact.yaml.tmpl"
const minioArtifactSecretName = "mlpipeline-minio-artifact"

func (r *DSPipelineReconciler) ReconcileAPIServer(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline, req ctrl.Request, params *DSPipelineParams) error {
	r.Log.Info("Applying APIServer Resources")

	for _, template := range apiServerTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	// Todo: We should switch to only using this secret for now otherwise the 2 storage secrets will go
	// out of sync if one is deleted because the password will be re-generated.
	// can also just check if the specified secret exists and copy those values onto the hardcoded secret.
	secret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      minioArtifactSecretName,
		Namespace: req.Namespace,
	}
	err := r.Get(ctx, namespacedName, secret)
	if err != nil && apierrs.IsNotFound(err) {
		r.Log.Info("Specified minio artifact secret not found, creating...")
		err := r.Apply(dsp, params, minioArtifactSecret)
		if err != nil {
			return err
		}
	} else if err != nil {
		r.Log.Error(err, "Unable to fetch minio artifact secret...")
		return err
	}

	r.Log.Info("Finished applying APIServer Resources")
	return nil
}
