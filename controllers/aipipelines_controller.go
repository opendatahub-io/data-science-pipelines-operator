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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
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
)

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
// +kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications,verbs=get;list;watch

func (r *AIPipelinesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Name != aipipelinesv1alpha1.AIPipelinesInstanceName {
		return ctrl.Result{}, nil
	}

	module := &aipipelinesv1alpha1.AIPipelines{}
	if err := r.Get(ctx, req.NamespacedName, module); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	dspaList := &dspav1.DataSciencePipelinesApplicationList{}
	if err := r.List(ctx, dspaList); err != nil {
		return ctrl.Result{}, fmt.Errorf("list DSPA instances: %w", err)
	}

	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	argo := observeArgoLifecycle(ctx, reader, r.Namespace, module.Spec.ArgoWorkflowsControllersManagementState())
	desired := buildAIPipelinesStatus(module, dspaList.Items, argo)
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
			&dspav1.DataSciencePipelinesApplication{},
			handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
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
	dspas []dspav1.DataSciencePipelinesApplication,
	argoObservation argoLifecycleObservation,
) aipipelinesv1alpha1.AIPipelinesStatus {
	configurationStatus, configurationReason, configurationMessage := validateAIPipelinesConfiguration(module)
	dspoStatus, dspoReason, dspoMessage := aggregateDSPACondition(dspas, config.CrReady, false)

	argoStatus, argoReason, argoMessage := argoObservation.Status, argoObservation.Reason, argoObservation.Message
	if module.Spec.ArgoWorkflowsControllersManagementState() == common.Managed {
		dspaArgoStatus, dspaArgoReason, dspaArgoMessage := aggregateDSPACondition(dspas, config.WorkflowControllerReady, true)
		argoStatus = aggregateConditionStatuses(argoStatus, dspaArgoStatus)
		if argoObservation.Status == metav1.ConditionTrue && dspaArgoStatus != metav1.ConditionTrue {
			argoReason, argoMessage = dspaArgoReason, dspaArgoMessage
		}
	}

	readyStatus := aggregateConditionStatuses(configurationStatus, dspoStatus, argoStatus)
	readyReason := "Ready"
	readyMessage := "AIPipelines is ready"
	if readyStatus != metav1.ConditionTrue {
		readyReason = "ComponentsNotReady"
		readyMessage = "AIPipelines is waiting for configuration, DSPO, or Argo readiness"
	}

	provisioningStatus := configurationStatus
	provisioningReason := "ConfigurationAccepted"
	provisioningMessage := "AIPipelines configuration was accepted"
	if provisioningStatus != metav1.ConditionTrue {
		provisioningReason = "InvalidConfiguration"
		provisioningMessage = configurationMessage
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
		moduleCondition(module, common.ConditionType(conditionTypeDSPOReady), dspoStatus, dspoReason, dspoMessage),
		moduleCondition(module, common.ConditionType(conditionTypeArgoReady), argoStatus, argoReason, argoMessage),
	}

	if platformVersion := config.ResolvedPlatformVersion(); platformVersion != "" {
		status.SetPlatformRelease(platformVersion)
	}

	return status
}

func validateAIPipelinesConfiguration(module *aipipelinesv1alpha1.AIPipelines) (metav1.ConditionStatus, string, string) {
	switch state := module.Spec.ArgoWorkflowsControllersManagementState(); state {
	case common.Managed, common.Removed:
		return metav1.ConditionTrue, "ValidConfiguration", "AIPipelines configuration is valid"
	default:
		return metav1.ConditionFalse, "InvalidManagementState", fmt.Sprintf("unsupported Argo Workflows management state %q", state)
	}
}

func aggregateDSPACondition(
	dspas []dspav1.DataSciencePipelinesApplication,
	conditionType string,
	ignoreNotApplicable bool,
) (metav1.ConditionStatus, string, string) {
	if len(dspas) == 0 {
		return metav1.ConditionTrue, "NoDSPAInstances", "No DSPA instances are present"
	}

	var failed, pending []string
	applicable := len(dspas)
	for i := range dspas {
		condition := findDSPACondition(dspas[i].Status.Conditions, conditionType)
		name := dspas[i].Namespace + "/" + dspas[i].Name
		switch {
		case condition == nil || condition.Status == metav1.ConditionUnknown:
			pending = append(pending, name)
		case condition.Status == metav1.ConditionFalse && ignoreNotApplicable && condition.Reason == "NotApplicable":
			applicable--
			continue
		case condition.Status != metav1.ConditionTrue:
			failed = append(failed, name)
		}
	}

	sort.Strings(failed)
	sort.Strings(pending)
	if len(failed) > 0 {
		return metav1.ConditionFalse, "DSPAInstancesNotReady", "Not ready: " + strings.Join(failed, ", ")
	}
	if len(pending) > 0 {
		return metav1.ConditionUnknown, "DSPAStatusPending", "Waiting for status: " + strings.Join(pending, ", ")
	}
	if ignoreNotApplicable {
		return metav1.ConditionTrue, "AllDSPAInstancesReady",
			fmt.Sprintf("All %d applicable DSPA instance(s) report ready", applicable)
	}
	return metav1.ConditionTrue, "AllDSPAInstancesReady", fmt.Sprintf("All %d DSPA instance(s) report ready", len(dspas))
}

func findDSPACondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
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
