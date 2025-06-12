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
	admv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
)

var webhookTemplatesDir = "webhook/"

const operatorName = "data-science-pipelines-operator-controller-manager"

// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=create
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;update;list;watch;patch;delete,resourceNames=pipelineversions.pipelines.kubeflow.org
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=create
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;update;list;watch;patch;delete,resourceNames=pipelineversions.pipelines.kubeflow.org

func (r *DSPAReconciler) ReconcileWebhook(ctx context.Context, params *DSPAParams) error {
	log := r.Log.WithValues("namespace", params.DSPONamespace)
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

func (r *DSPAReconciler) CleanUpWebhookIfUnused(ctx context.Context, dspa *dspav1.DataSciencePipelinesApplication, params *DSPAParams) error {
	log := r.Log.WithValues("namespace", params.DSPONamespace, "dspa_name", dspa.Name)
	hasK8sDSPAs, err := r.checkAvailableKubernetesDSPAs(ctx, dspa.Name, dspa.Namespace)
	if err != nil {
		log.Error(err, "Failed to check for other DSPAs with 'kubernetes' storage")
		return err
	}

	if !hasK8sDSPAs {
		log.Info("No other DSPAs with PipelineStore 'kubernetes' found. Cleaning up webhook resources.")
		if err := r.cleanupWebhookResources(ctx, params.DSPONamespace); err != nil {
			log.Error(err, "Failed to clean up webhook resources")
			return err
		}
	}

	log.Info("Webhook resources cleanup complete.")
	return nil
}

func (r *DSPAReconciler) cleanupWebhookResources(ctx context.Context, namespace string) error {
	log := r.Log.WithValues("namespace", namespace)
	webhookConfigName := "pipelineversions.pipelines.kubeflow.org"
	webhookNamespaced := types.NamespacedName{Name: k8sWebhookName, Namespace: namespace}

	// Delete MutatingWebhookConfiguration
	if err := r.DeleteResourceIfItExists(ctx, &admv1.MutatingWebhookConfiguration{}, types.NamespacedName{Name: webhookConfigName}); err != nil {
		log.Error(err, "Failed to delete MutatingWebhookConfiguration")
		return err
	}

	// Delete ValidatingWebhookConfiguration
	if err := r.DeleteResourceIfItExists(ctx, &admv1.ValidatingWebhookConfiguration{}, types.NamespacedName{Name: webhookConfigName}); err != nil {
		log.Error(err, "Failed to delete ValidatingWebhookConfiguration")
		return err
	}

	// Delete Deployment
	if err := r.DeleteResourceIfItExists(ctx, &appsv1.Deployment{}, webhookNamespaced); err != nil {
		log.Error(err, "Failed to delete Deployment")
		return err
	}

	// Delete ClusterRole
	if err := r.DeleteResourceIfItExists(ctx, &rbacv1.ClusterRole{}, types.NamespacedName{Name: k8sWebhookName}); err != nil {
		log.Error(err, "Failed to delete ClusterRole")
		return err
	}

	// Delete ClusterRoleBinding
	if err := r.DeleteResourceIfItExists(ctx, &rbacv1.ClusterRoleBinding{}, types.NamespacedName{Name: k8sWebhookName}); err != nil {
		log.Error(err, "Failed to delete ClusterRoleBinding")
		return err
	}

	// Delete Service
	if err := r.DeleteResourceIfItExists(ctx, &corev1.Service{}, webhookNamespaced); err != nil {
		log.Error(err, "Failed to delete Service")
		return err
	}

	// Delete ServiceAccount
	if err := r.DeleteResourceIfItExists(ctx, &corev1.ServiceAccount{}, webhookNamespaced); err != nil {
		log.Error(err, "Failed to delete ServiceAccount")
		return err
	}

	return nil
}
