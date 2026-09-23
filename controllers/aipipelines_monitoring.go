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

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	monitoringassets "github.com/opendatahub-io/data-science-pipelines-operator/config/prometheus"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const prometheusRuleCRDName = "prometheusrules.monitoring.rhobs"

type monitoringObservation struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,resourceNames=prometheusrules.monitoring.rhobs,verbs=list;watch
// +kubebuilder:rbac:groups=monitoring.rhobs,resources=prometheusrules,resourceNames=data-science-pipelines-operator-datasciencepipelines-prometheusrules,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.rhobs,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:urls=/metrics,verbs=get

func (r *AIPipelinesReconciler) reconcileMonitoringResources(
	ctx context.Context,
	module *aipipelinesv1alpha1.AIPipelines,
) (monitoringObservation, error) {
	applicationsNamespace := r.ApplicationsNamespace
	if applicationsNamespace == "" {
		applicationsNamespace = r.Namespace
	}
	coreServiceMonitor, err := monitoringassets.CoreServiceMonitor(r.Namespace)
	if err != nil {
		return monitoringObservation{}, err
	}
	if _, err := r.reconcileMonitoringObject(ctx, module, coreServiceMonitor, "spec"); err != nil {
		return monitoringObservation{}, fmt.Errorf("reconcile core ServiceMonitor: %w", err)
	}

	if r.MonitoringNamespace == "" {
		return monitoringObservation{
			Status:  metav1.ConditionTrue,
			Reason:  "MonitoringNotConfigured",
			Message: "RHOAI monitoring is not configured",
		}, nil
	}
	if err := r.reconcilePrometheusRule(ctx, module); err != nil {
		return monitoringObservation{}, err
	}

	rhoaiServiceMonitor, err := monitoringassets.RHOAIServiceMonitor(applicationsNamespace, r.MonitoringNamespace)
	if err != nil {
		return monitoringObservation{}, err
	}
	supported, err := r.reconcileMonitoringObject(ctx, module, rhoaiServiceMonitor, "spec")
	if err != nil {
		return monitoringObservation{}, fmt.Errorf("reconcile RHOAI ServiceMonitor: %w", err)
	}
	if !supported {
		return monitoringObservation{
			Status:  metav1.ConditionTrue,
			Reason:  "MonitoringAPINotAvailable",
			Message: "RHOAI monitoring API is not available; optional monitoring resources were skipped",
		}, nil
	}
	binding, err := monitoringassets.RHOAIMetricsReaderRoleBinding(r.MonitoringNamespace)
	if err != nil {
		return monitoringObservation{}, err
	}
	if _, err := r.reconcileMonitoringObject(ctx, module, binding, "roleRef", "subjects"); err != nil {
		return monitoringObservation{}, fmt.Errorf("reconcile RHOAI metrics reader binding: %w", err)
	}
	return monitoringObservation{
		Status:  metav1.ConditionTrue,
		Reason:  "MonitoringResourcesReady",
		Message: "RHOAI monitoring resources are ready",
	}, nil
}

// reconcilePrometheusRule keeps monitoring ownership inside DSPO. The rule is
// optional when the monitoring.rhobs API is not installed and does not gate
// AIPipelines readiness.
func (r *AIPipelinesReconciler) reconcilePrometheusRule(
	ctx context.Context,
	module *aipipelinesv1alpha1.AIPipelines,
) error {
	applicationsNamespace := r.ApplicationsNamespace
	if applicationsNamespace == "" {
		applicationsNamespace = r.Namespace
	}
	desired, err := monitoringassets.Rule(applicationsNamespace)
	if err != nil {
		return err
	}
	_, err = r.reconcileMonitoringObject(ctx, module, desired, "spec")
	if err != nil {
		return fmt.Errorf("reconcile PrometheusRule: %w", err)
	}
	return nil
}

// reconcileMonitoringObject applies an optional monitoring object without
// making module readiness depend on the corresponding API being installed.
func (r *AIPipelinesReconciler) reconcileMonitoringObject(
	ctx context.Context,
	module *aipipelinesv1alpha1.AIPipelines,
	desired *unstructured.Unstructured,
	managedFields ...string,
) (bool, error) {
	if err := controllerutil.SetControllerReference(module, desired, r.Scheme); err != nil {
		return false, fmt.Errorf("set AIPipelines owner on %s: %w", desired.GetKind(), err)
	}

	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(desired.GroupVersionKind())
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	err := reader.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		createErr := r.Create(ctx, desired)
		if meta.IsNoMatchError(createErr) || runtime.IsNotRegisteredError(createErr) || apierrors.IsNotFound(createErr) {
			return false, nil
		}
		if createErr != nil {
			return false, fmt.Errorf("create AIPipelines %s: %w", desired.GetKind(), createErr)
		}
		return true, nil
	}
	if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get AIPipelines %s: %w", desired.GetKind(), err)
	}

	controller := metav1.GetControllerOf(current)
	if controller != nil && controller.UID != module.UID {
		return false, fmt.Errorf(
			"%s %q is controlled by %s %q with UID %q",
			desired.GetKind(), current.GetName(), controller.Kind, controller.Name, controller.UID,
		)
	}

	updated := current.DeepCopy()
	updated.SetOwnerReferences(desired.GetOwnerReferences())
	for _, field := range managedFields {
		value, found := desired.Object[field]
		if !found {
			delete(updated.Object, field)
			continue
		}
		updated.Object[field] = runtime.DeepCopyJSONValue(value)
	}
	if apiequality.Semantic.DeepEqual(current.Object, updated.Object) {
		return true, nil
	}
	if err := r.Update(ctx, updated); err != nil {
		if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
			return false, nil
		}
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("update AIPipelines %s: %w", desired.GetKind(), err)
	}
	return true, nil
}
