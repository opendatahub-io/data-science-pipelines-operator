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
	"fmt"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var mlPipelineUITemplates = []string{
	"mlpipelines-ui/configmap.yaml.tmpl",
	"mlpipelines-ui/deployment.yaml.tmpl",
	"mlpipelines-ui/role.yaml.tmpl",
	"mlpipelines-ui/rolebinding.yaml.tmpl",
	"mlpipelines-ui/route.yaml.tmpl",
	"mlpipelines-ui/sa-ds-pipeline-ui.yaml.tmpl",
	"mlpipelines-ui/sa_ds-pipelines-viewer.yaml.tmpl",
	"mlpipelines-ui/service.yaml.tmpl",
}

const mlPipelinesUICookieSecret = "mlpipelines-ui/secret.yaml.tmpl"

func (r *DSPAReconciler) ReconcileUI(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	if !dsp.Spec.MlPipelineUI.Deploy {
		r.Log.Info("Skipping Application of MlPipelineUI Resources")
		return nil
	}

	r.Log.Info("Applying MlPipelineUI Resources")
	for _, template := range mlPipelineUITemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	cookieSecret := &corev1.Secret{}
	cookieSecretNN := types.NamespacedName{
		Name:      fmt.Sprintf("ds-pipelines-ui-%s-cookie-secret", dsp.Name),
		Namespace: dsp.Namespace,
	}
	err := r.CreateIfDoesNotItExists(ctx, cookieSecret, cookieSecretNN, params, mlPipelinesUICookieSecret, dsp)
	if err != nil {
		return err
	}

	r.Log.Info("Applying MlPipelineUI Resources")
	return nil
}
