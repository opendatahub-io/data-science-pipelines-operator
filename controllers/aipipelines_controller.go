/*
Copyright 2026.

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

	"github.com/go-logr/logr"
	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	conditionTypeDSPOReady          = "DSPOReady"
	conditionTypeArgoReady          = "ArgoWorkflowsControllersReady"
	conditionTypeConfigurationValid = "ConfigurationValid"
	dspoDeploymentName              = "data-science-pipelines-operator-controller-manager"
)

type dspoDeploymentObservation struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

type platformConfigObservation struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
	Version string
}

// AIPipelinesReconciler reconciles the singleton AIPipelines module CR. It is
// the only controller that writes AIPipelines.status; subordinate controllers
// expose readiness through the resources this reconciler observes.
type AIPipelinesReconciler struct {
	client.Client
	APIReader client.Reader
	Scheme    *runtime.Scheme
	Log       logr.Logger
	Namespace string
}

// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=aipipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=aipipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *AIPipelinesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Name != aipipelinesv1alpha1.AIPipelinesInstanceName {
		return ctrl.Result{}, nil
	}

	module := &aipipelinesv1alpha1.AIPipelines{}
	if err := r.Get(ctx, req.NamespacedName, module); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	dspo := observeDSPODeployment(ctx, reader, r.Namespace)
	argo := observeArgoLifecycle(ctx, reader, r.Namespace, module.Spec.ArgoWorkflowsControllersManagementState(), module)
	platformConfig := observePlatformConfig(ctx, reader)
	desired := buildAIPipelinesStatus(module, dspo, argo, platformConfig)
	if apiequality.Semantic.DeepEqual(module.Status, desired) {
		return ctrl.Result{}, nil
	}

	updated := module.DeepCopy()
	updated.Status = desired
	if err := r.Status().Update(ctx, updated); err != nil {
		return ctrl.Result{}, fmt.Errorf("update AIPipelines status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *AIPipelinesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&aipipelinesv1alpha1.AIPipelines{}).
		Watches(
			&appsv1.Deployment{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, object client.Object) []reconcile.Request {
				if object.GetNamespace() != r.Namespace || object.GetName() != dspoDeploymentName {
					return nil
				}
				return []reconcile.Request{{NamespacedName: types.NamespacedName{
					Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
				}}}
			}),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, object client.Object) []reconcile.Request {
				if !isPlatformConfigMap(object) {
					return nil
				}
				return []reconcile.Request{{NamespacedName: types.NamespacedName{
					Name: aipipelinesv1alpha1.AIPipelinesInstanceName,
				}}}
			}),
		)
	if err := watchArgoAssets(b, r.Namespace); err != nil {
		return err
	}
	return b.Complete(r)
}

func buildAIPipelinesStatus(
	module *aipipelinesv1alpha1.AIPipelines,
	dspoObservation dspoDeploymentObservation,
	argoObservation argoLifecycleObservation,
	platformObservations ...platformConfigObservation,
) aipipelinesv1alpha1.AIPipelinesStatus {
	configurationStatus, configurationReason, configurationMessage := validateAIPipelinesConfiguration(module)
	dspoStatus, dspoReason, dspoMessage := dspoObservation.Status, dspoObservation.Reason, dspoObservation.Message
	argoStatus, argoReason, argoMessage := argoObservation.Status, argoObservation.Reason, argoObservation.Message
	platformConfig := platformConfigObservation{
		Status:  metav1.ConditionTrue,
		Reason:  "StandaloneConfiguration",
		Message: "Using standalone operator configuration",
		Version: config.ResolvedPlatformVersion(),
	}
	if len(platformObservations) > 0 {
		platformConfig = platformObservations[0]
	}

	readyStatus := aggregateConditionStatuses(configurationStatus, platformConfig.Status, dspoStatus, argoStatus)
	readyReason := "Ready"
	readyMessage := "AIPipelines is ready"
	if readyStatus != metav1.ConditionTrue {
		readyReason = "ComponentsNotReady"
		readyMessage = "AIPipelines is waiting for configuration, DSPO, or Argo readiness"
	}

	provisioningStatus := aggregateConditionStatuses(configurationStatus, platformConfig.Status, argoStatus)
	provisioningReason := "ManifestApplicationSucceeded"
	provisioningMessage := "All AIPipelines manifests were applied successfully"
	switch {
	case configurationStatus != metav1.ConditionTrue:
		provisioningReason = "InvalidConfiguration"
		provisioningMessage = configurationMessage
	case platformConfig.Status != metav1.ConditionTrue:
		provisioningReason = platformConfig.Reason
		provisioningMessage = platformConfig.Message
	case argoStatus != metav1.ConditionTrue:
		provisioningReason = argoReason
		provisioningMessage = argoMessage
	}

	status := aipipelinesv1alpha1.AIPipelinesStatus{}
	status.ObservedGeneration = module.Generation
	status.Phase = common.PhaseNotReady
	if readyStatus == metav1.ConditionTrue {
		status.Phase = common.PhaseReady
	}
	status.Conditions = []common.Condition{
		moduleCondition(module, common.ConditionTypeReady, readyStatus, readyReason, readyMessage),
		moduleCondition(module, common.ConditionTypeProvisioningSucceeded, provisioningStatus, provisioningReason, provisioningMessage),
		moduleCondition(module, common.ConditionType(conditionTypeConfigurationValid), configurationStatus, configurationReason, configurationMessage),
		moduleCondition(module, common.ConditionType("PlatformConfigurationValid"), platformConfig.Status, platformConfig.Reason, platformConfig.Message),
		moduleCondition(module, common.ConditionType(conditionTypeDSPOReady), dspoStatus, dspoReason, dspoMessage),
		moduleCondition(module, common.ConditionType(conditionTypeArgoReady), argoStatus, argoReason, argoMessage),
	}

	if provisioningStatus == metav1.ConditionTrue && platformConfig.Version != "" {
		status.SetPlatformRelease(platformConfig.Version)
	} else if previousRelease := module.Status.GetPlatformRelease(); previousRelease != "" {
		status.SetPlatformRelease(previousRelease)
	}

	return status
}

func observePlatformConfig(ctx context.Context, reader client.Reader) platformConfigObservation {
	version, err := resolvePlatformVersion(ctx, reader)
	if err != nil {
		return platformConfigObservation{
			Status:  metav1.ConditionFalse,
			Reason:  "PlatformConfigurationUnavailable",
			Message: err.Error(),
		}
	}
	return platformConfigObservation{
		Status:  metav1.ConditionTrue,
		Reason:  "PlatformConfigurationAvailable",
		Message: "Platform configuration is available",
		Version: version,
	}
}

func validateAIPipelinesConfiguration(module *aipipelinesv1alpha1.AIPipelines) (metav1.ConditionStatus, string, string) {
	switch state := module.Spec.ArgoWorkflowsControllersManagementState(); state {
	case common.Managed, common.Removed:
		return metav1.ConditionTrue, "ValidConfiguration", "AIPipelines configuration is valid"
	default:
		return metav1.ConditionFalse, "InvalidManagementState", fmt.Sprintf("unsupported Argo Workflows management state %q", state)
	}
}

func observeDSPODeployment(ctx context.Context, reader client.Reader, namespace string) dspoDeploymentObservation {
	deployment := &appsv1.Deployment{}
	err := reader.Get(ctx, types.NamespacedName{Name: dspoDeploymentName, Namespace: namespace}, deployment)
	if apierrors.IsNotFound(err) {
		return dspoDeploymentObservation{Status: metav1.ConditionFalse, Reason: "DSPONotFound", Message: "Waiting for the DSPO deployment"}
	}
	if err != nil {
		return dspoDeploymentObservation{Status: metav1.ConditionUnknown, Reason: "DSPOObservationFailed", Message: fmt.Sprintf("Unable to observe the DSPO deployment: %v", err)}
	}
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return dspoDeploymentObservation{Status: metav1.ConditionUnknown, Reason: "DSPOStatusPending", Message: "Waiting for the DSPO deployment to observe its latest generation"}
	}
	for i := range deployment.Status.Conditions {
		condition := deployment.Status.Conditions[i]
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return dspoDeploymentObservation{Status: metav1.ConditionTrue, Reason: "DSPOAvailable", Message: "The DSPO deployment is available"}
		}
	}
	return dspoDeploymentObservation{Status: metav1.ConditionFalse, Reason: "DSPOUnavailable", Message: "Waiting for the DSPO deployment to become available"}
}

func aggregateConditionStatuses(statuses ...metav1.ConditionStatus) metav1.ConditionStatus {
	result := metav1.ConditionTrue
	for _, status := range statuses {
		if status == metav1.ConditionFalse {
			return metav1.ConditionFalse
		}
		if status != metav1.ConditionTrue {
			result = metav1.ConditionUnknown
		}
	}
	return result
}

func moduleCondition(
	module *aipipelinesv1alpha1.AIPipelines,
	conditionType common.ConditionType,
	status metav1.ConditionStatus,
	reason, message string,
) common.Condition {
	transitionTime := metav1.Now()
	for i := range module.Status.Conditions {
		previous := module.Status.Conditions[i]
		if previous.Type == string(conditionType) && previous.Status == status {
			transitionTime = previous.LastTransitionTime
			break
		}
	}

	return common.Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: module.Generation,
		LastTransitionTime: transitionTime,
	}
}
