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
	"context"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	storageDeploymentTemplate     = "config/internal/minio/deployment.yaml.tmpl"
	storagePvcTemplate            = "config/internal/minio/pvc.yaml.tmpl"
	storageServiceAccountTemplate = "config/internal/minio/service.yaml.tmpl"
	storageSecretTemplate         = "config/internal/minio/secret.yaml.tmpl"
)

func (r *DSPipelineReconciler) ReconcileStorage(dsp *dspipelinesiov1alpha1.DSPipeline, ctx context.Context,
	req ctrl.Request, params *DSPipelineParams) error {
	r.Log.Info("Applying Storage Resources")

	// If the provided secret does not exist, create it
	secret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      params.S3CredentialsSecretName,
		Namespace: req.Namespace,
	}
	err := r.Get(ctx, namespacedName, secret)
	if err != nil && apierrs.IsNotFound(err) {
		r.Log.Info("Specified storage secret not found, creating...")
		err := r.Apply(dsp, params, storageSecretTemplate)
		if err != nil {
			return err
		}
	} else if err != nil {
		r.Log.Error(err, "Unable to fetch storage secret...")
		return err
	}

	templates := []string{storageDeploymentTemplate, storagePvcTemplate, storageServiceAccountTemplate}
	for _, template := range templates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}
	r.Log.Info("Finished applying Storage Resources")
	return nil
}
