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

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,resourceNames=prometheusrules.monitoring.rhobs,verbs=list;watch
// +kubebuilder:rbac:groups=monitoring.rhobs,resources=prometheusrules,verbs=create
// +kubebuilder:rbac:groups=monitoring.rhobs,resources=prometheusrules,resourceNames=data-science-pipelines-operator-datasciencepipelines-prometheusrules,verbs=get;update;patch

// reconcilePrometheusRule keeps monitoring ownership inside DSPO. The rule is
// optional when the monitoring.rhobs API is not installed and does not gate
// AIPipelines readiness.
func (r *AIPipelinesReconciler) reconcilePrometheusRule(
	ctx context.Context,
	module *aipipelinesv1alpha1.AIPipelines,
) error {
	desired, err := monitoringassets.Rule(r.Namespace)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(module, desired, r.Scheme); err != nil {
		return fmt.Errorf("set AIPipelines owner on PrometheusRule: %w", err)
	}

	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(desired.GroupVersionKind())
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	err = reader.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil &&
			!meta.IsNoMatchError(err) && !apierrors.IsNotFound(err) {
			return fmt.Errorf("create AIPipelines PrometheusRule: %w", err)
		}
		return nil
	}
	if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get AIPipelines PrometheusRule: %w", err)
	}

	controller := metav1.GetControllerOf(current)
	if controller != nil && controller.UID != module.UID {
		return fmt.Errorf(
			"PrometheusRule %q is controlled by %s %q with UID %q",
			current.GetName(), controller.Kind, controller.Name, controller.UID,
		)
	}

	updated := current.DeepCopy()
	updated.SetOwnerReferences(desired.GetOwnerReferences())
	for _, field := range []string{"groups"} {
		value, found, err := unstructured.NestedFieldNoCopy(desired.Object, "spec", field)
		if err != nil {
			return fmt.Errorf("read desired PrometheusRule %s: %w", field, err)
		}
		if !found {
			unstructured.RemoveNestedField(updated.Object, "spec", field)
			continue
		}
		if err := unstructured.SetNestedField(updated.Object, runtime.DeepCopyJSONValue(value), "spec", field); err != nil {
			return fmt.Errorf("set PrometheusRule %s: %w", field, err)
		}
	}
	if apiequality.Semantic.DeepEqual(current.Object, updated.Object) {
		return nil
	}
	if err := r.Update(ctx, updated); err != nil && !meta.IsNoMatchError(err) {
		return fmt.Errorf("update AIPipelines PrometheusRule: %w", err)
	}
	return nil
}
