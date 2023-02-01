package controllers

import (
	"context"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

var apiServerTemplates = []string{
	"config/internal/apiserver/artifact_script.yaml.tmpl",
	"config/internal/apiserver/role_ds-pipeline.yaml.tmpl",
	"config/internal/apiserver/role_pipeline-runner.yaml.tmpl",
	"config/internal/apiserver/rolebinding_ds-pipeline.yaml.tmpl",
	"config/internal/apiserver/rolebinding_pipeline-runner.yaml.tmpl",
	"config/internal/apiserver/sa_ds-pipeline.yaml.tmpl",
	"config/internal/apiserver/sa_pipeline-runner.yaml.tmpl",
	"config/internal/apiserver/service.yaml.tmpl",
	"config/internal/apiserver/deployment.yaml.tmpl",
}

// This is hardcoded in kfp-tekton, apiserver will always use this hardcoded secret for tekton resources
const minioArtifactSecret = "config/internal/apiserver/mlpipeline-minio-artifact.yaml.tmpl"
const minioArtifactSecretName = "mlpipeline-minio-artifact"

func (r *DSPipelineReconciler) ReconcileAPIServer(dsp *dspipelinesiov1alpha1.DSPipeline, ctx context.Context, req ctrl.Request, params *DSPipelineParams) error {
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
