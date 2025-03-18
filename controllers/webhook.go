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
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

var webhookTemplatesDir = "webhook/"

const operatorName = "data-science-pipelines-operator-controller-manager"

//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete

func (r *DSPAReconciler) ReconcileWebhook(ctx context.Context, dsp *dspav1.DataSciencePipelinesApplication, params *DSPAParams) error {
	log := r.Log.WithValues("namespace", params.DSPONamespace).WithValues("dspa_name", dsp.Name)

	if !dsp.Spec.APIServer.Deploy {
		r.Log.Info("Skipping Application of Webhook Resources")
		return nil
	}

	dataSciencePipelinesOperator := appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Namespace: params.DSPONamespace, Name: operatorName}, &dataSciencePipelinesOperator)
	if err != nil {
		return err
	}

	log.Info("Applying Webhook Resources")
	err = r.ApplyDir(&dataSciencePipelinesOperator, params, webhookTemplatesDir)
	if err != nil {
		return err
	}

	log.Info("Finished applying Webhook Resources")
	return nil
}
