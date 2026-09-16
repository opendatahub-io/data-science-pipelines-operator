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
	argoassets "github.com/opendatahub-io/data-science-pipelines-operator/config/argo"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	argoLifecycleFinalizer        = "aipipelines.components.platform.opendatahub.io/argo-cleanup"
	argoManagedAnnotation         = "aipipelines.components.platform.opendatahub.io/argo-managed"
	legacyPipelinesComponentLabel = "app.opendatahub.io/data-science-pipelines-operator"
	argoWorkflowCRDName           = "workflows.argoproj.io"
)

var argoManagedPaths = map[string][][]string{
	"CustomResourceDefinition": {{"spec"}},
	"ClusterRole":              {{"rules"}, {"aggregationRule"}},
	"Role":                     {{"rules"}},
	"ClusterRoleBinding":       {{"roleRef"}, {"subjects"}},
	"RoleBinding":              {{"roleRef"}, {"subjects"}},
	"ConfigMap":                {{"data"}, {"binaryData"}, {"immutable"}},
	"ServiceAccount":           {{"automountServiceAccountToken"}, {"imagePullSecrets"}},
}

type argoLifecycleObservation struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// AIPipelinesArgoReconciler manages only shared Argo installation assets.
// DSPAs and their namespace-scoped operands remain owned by DSPAReconciler.
type AIPipelinesArgoReconciler struct {
	client.Client
	APIReader client.Reader
	Scheme    *runtime.Scheme
	Log       logr.Logger
	Namespace string
}

// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=aipipelines,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=aipipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=create;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,resourceNames=applications.app.k8s.io;clusterworkflowtemplates.argoproj.io;cronworkflows.argoproj.io;viewers.kubeflow.org;workflowartifactgctasks.argoproj.io;workfloweventbindings.argoproj.io;workflows.argoproj.io;workflowtaskresults.argoproj.io;workflowtasksets.argoproj.io;workflowtemplates.argoproj.io,verbs=get;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=create;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,resourceNames=argo-aggregate-to-admin;argo-aggregate-to-edit;argo-aggregate-to-view;argo-binding;argo-cluster-role;argo-role;ds-pipeline-argo-binding,verbs=get;update;delete
// +kubebuilder:rbac:groups="",resources=configmaps;serviceaccounts,verbs=get;list;watch;create;update;patch;delete

func (r *AIPipelinesArgoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	module := &aipipelinesv1alpha1.AIPipelines{}
	if err := r.Get(ctx, req.NamespacedName, module); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if req.Name != aipipelinesv1alpha1.AIPipelinesInstanceName {
		if !module.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(module, argoLifecycleFinalizer) {
			updated := module.DeepCopy()
			controllerutil.RemoveFinalizer(updated, argoLifecycleFinalizer)
			if err := r.Update(ctx, updated); err != nil {
				return ctrl.Result{}, fmt.Errorf("remove Argo finalizer from non-singleton AIPipelines: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	assets, err := r.desiredAssets(module)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !module.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(module, argoLifecycleFinalizer) {
			return ctrl.Result{}, nil
		}
		pending, err := r.removeControllerAssets(ctx, assets)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("clean shared Argo assets during AIPipelines finalization: %w", err)
		}
		if pending {
			return ctrl.Result{Requeue: true}, nil
		}
		updated := module.DeepCopy()
		controllerutil.RemoveFinalizer(updated, argoLifecycleFinalizer)
		if err := r.Update(ctx, updated); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove AIPipelines Argo finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}
	if !controllerutil.ContainsFinalizer(module, argoLifecycleFinalizer) {
		updated := module.DeepCopy()
		controllerutil.AddFinalizer(updated, argoLifecycleFinalizer)
		if err := r.Update(ctx, updated); err != nil {
			return ctrl.Result{}, fmt.Errorf("add AIPipelines Argo finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	switch module.Spec.ArgoWorkflowsControllersManagementState() {
	case common.Managed:
		if err := r.validateExistingAssetOwnership(ctx, assets); err != nil {
			return ctrl.Result{}, err
		}
		for _, asset := range assets {
			if err := r.reconcileAsset(ctx, asset); err != nil {
				return ctrl.Result{}, fmt.Errorf("reconcile shared Argo %s %s: %w", asset.GetKind(), asset.GetName(), err)
			}
		}
	case common.Removed:
		if err := r.requireWorkflowCRD(ctx); err != nil {
			return ctrl.Result{}, err
		}
		pending, err := r.removeControllerAssets(ctx, assets)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("remove shared Argo controller assets: %w", err)
		}
		if pending {
			return ctrl.Result{Requeue: true}, nil
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unsupported Argo Workflows management state %q", module.Spec.ArgoWorkflowsControllersManagementState())
	}

	return ctrl.Result{}, nil
}

func (r *AIPipelinesArgoReconciler) validateExistingAssetOwnership(ctx context.Context, assets []*unstructured.Unstructured) error {
	for _, desired := range assets {
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(desired.GroupVersionKind())
		err := r.reader().Get(ctx, client.ObjectKeyFromObject(desired), current)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("check existing shared Argo %s %s: %w", desired.GetKind(), desired.GetName(), err)
		}
		if !isArgoManaged(current) {
			return fmt.Errorf("shared Argo %s %s already exists and is not owned by AI Pipelines", desired.GetKind(), desired.GetName())
		}
	}
	return nil
}

func (r *AIPipelinesArgoReconciler) desiredAssets(module *aipipelinesv1alpha1.AIPipelines) ([]*unstructured.Unstructured, error) {
	assets, err := argoassets.Objects(r.Namespace)
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		labels := asset.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[legacyPipelinesComponentLabel] = "true"
		labels[config.DSPVersionk8sLabel] = config.DSPV2VersionString
		asset.SetLabels(labels)

		annotations := asset.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[argoManagedAnnotation] = "true"
		asset.SetAnnotations(annotations)

		// CRDs are deliberately finalized rather than owner-referenced. Deleting
		// the module must not garbage-collect DSPA-owned Workflow objects.
		if asset.GetKind() != "CustomResourceDefinition" {
			if err := controllerutil.SetControllerReference(module, asset, r.Scheme); err != nil {
				return nil, fmt.Errorf("set owner for Argo %s %s: %w", asset.GetKind(), asset.GetName(), err)
			}
		}
	}
	return assets, nil
}

func (r *AIPipelinesArgoReconciler) reconcileAsset(ctx context.Context, desired *unstructured.Unstructured) error {
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(desired.GroupVersionKind())
	err := r.reader().Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	updated := current.DeepCopy()
	mergeStringMap := func(existing, wanted map[string]string) map[string]string {
		if existing == nil {
			existing = map[string]string{}
		}
		for key, value := range wanted {
			existing[key] = value
		}
		return existing
	}
	updated.SetLabels(mergeStringMap(updated.GetLabels(), desired.GetLabels()))
	updated.SetAnnotations(mergeStringMap(updated.GetAnnotations(), desired.GetAnnotations()))
	if desired.GetKind() != "CustomResourceDefinition" {
		updated.SetOwnerReferences(desired.GetOwnerReferences())
	}

	for _, path := range argoManagedPaths[desired.GetKind()] {
		value, found, err := unstructured.NestedFieldNoCopy(desired.Object, path...)
		if err != nil {
			return err
		}
		if !found {
			unstructured.RemoveNestedField(updated.Object, path...)
			continue
		}
		if err := unstructured.SetNestedField(updated.Object, runtime.DeepCopyJSONValue(value), path...); err != nil {
			return err
		}
	}

	if apiequality.Semantic.DeepEqual(current.Object, updated.Object) {
		return nil
	}
	return r.Update(ctx, updated)
}

func (r *AIPipelinesArgoReconciler) removeControllerAssets(ctx context.Context, assets []*unstructured.Unstructured) (bool, error) {
	var failures []string
	pending := false
	for _, asset := range assets {
		if asset.GetKind() == "CustomResourceDefinition" {
			continue
		}
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(asset.GroupVersionKind())
		if err := r.reader().Get(ctx, client.ObjectKeyFromObject(asset), current); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			failures = append(failures, fmt.Sprintf("get %s %s: %v", asset.GetKind(), asset.GetName(), err))
			continue
		}
		if !isArgoManaged(current) {
			continue
		}
		if err := r.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			failures = append(failures, fmt.Sprintf("delete %s %s: %v", asset.GetKind(), asset.GetName(), err))
		} else {
			pending = true
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return pending, fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return pending, nil
}

func isArgoManaged(object client.Object) bool {
	return object.GetLabels()[legacyPipelinesComponentLabel] == "true" || object.GetAnnotations()[argoManagedAnnotation] == "true"
}

func (r *AIPipelinesArgoReconciler) requireWorkflowCRD(ctx context.Context) error {
	_, err := r.getWorkflowCRD(ctx)
	if apierrors.IsNotFound(err) {
		return fmt.Errorf("Argo Workflows controllers are Removed but required CRD %s is missing", argoWorkflowCRDName)
	}
	return err
}

func (r *AIPipelinesArgoReconciler) getWorkflowCRD(ctx context.Context) (*unstructured.Unstructured, error) {
	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1")
	crd.SetKind("CustomResourceDefinition")
	err := r.reader().Get(ctx, types.NamespacedName{Name: argoWorkflowCRDName}, crd)
	return crd, err
}

func (r *AIPipelinesArgoReconciler) reader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}
	return r.Client
}

func (r *AIPipelinesArgoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		Named("aipipelines-argo-lifecycle").
		For(&aipipelinesv1alpha1.AIPipelines{})
	if err := watchArgoAssets(b, r.Namespace); err != nil {
		return err
	}
	return b.Complete(r)
}

func watchArgoAssets(b *builder.Builder, namespace string) error {
	assets, err := argoassets.Objects(namespace)
	if err != nil {
		return fmt.Errorf("load shared Argo assets for watches: %w", err)
	}
	keys := map[string]struct{}{}
	for _, asset := range assets {
		keys[argoAssetKey(asset)] = struct{}{}
	}
	filter := predicate.NewPredicateFuncs(func(object client.Object) bool {
		_, ok := keys[argoAssetKey(object)]
		return ok
	})
	enqueueModule := handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: aipipelinesv1alpha1.AIPipelinesInstanceName}}}
	})

	b.Watches(&corev1.ConfigMap{}, enqueueModule, builder.WithPredicates(filter)).
		Watches(&corev1.ServiceAccount{}, enqueueModule, builder.WithPredicates(filter)).
		Watches(&rbacv1.Role{}, enqueueModule, builder.WithPredicates(filter)).
		Watches(&rbacv1.RoleBinding{}, enqueueModule, builder.WithPredicates(filter)).
		Watches(&rbacv1.ClusterRole{}, enqueueModule, builder.WithPredicates(filter)).
		Watches(&rbacv1.ClusterRoleBinding{}, enqueueModule, builder.WithPredicates(filter))
	// CRDs are watched as unstructured resources so the main scheme does not
	// need to own the Argo API types themselves.
	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1")
	crd.SetKind("CustomResourceDefinition")
	b.Watches(crd, enqueueModule, builder.WithPredicates(filter))
	return nil
}

func argoAssetKey(object client.Object) string {
	// Typed objects delivered by controller-runtime watches do not always have
	// TypeMeta populated, so matching must not depend on the observed GVK.
	return object.GetNamespace() + "/" + object.GetName()
}

func observeArgoLifecycle(ctx context.Context, reader client.Reader, namespace string, state common.ManagementState, module *aipipelinesv1alpha1.AIPipelines) argoLifecycleObservation {
	assets, err := argoassets.Objects(namespace)
	if err != nil {
		return argoLifecycleObservation{Status: metav1.ConditionFalse, Reason: "ArgoAssetsInvalid", Message: err.Error()}
	}

	var missing, drifted, remaining []string
	for _, desired := range assets {
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(desired.GroupVersionKind())
		err := reader.Get(ctx, client.ObjectKeyFromObject(desired), current)
		if apierrors.IsNotFound(err) {
			if state == common.Managed {
				missing = append(missing, desired.GetKind()+"/"+desired.GetName())
			}
			if state == common.Removed && desired.GetName() == argoWorkflowCRDName {
				return argoLifecycleObservation{Status: metav1.ConditionFalse, Reason: "ArgoWorkflowCRDMissing", Message: "Required Argo Workflow CRD is missing"}
			}
			continue
		}
		if err != nil {
			return argoLifecycleObservation{Status: metav1.ConditionUnknown, Reason: "ArgoObservationFailed", Message: err.Error()}
		}

		if state == common.Removed {
			if desired.GetKind() != "CustomResourceDefinition" && (current.GetAnnotations()[argoManagedAnnotation] == "true" || current.GetLabels()[legacyPipelinesComponentLabel] == "true") {
				remaining = append(remaining, desired.GetKind()+"/"+desired.GetName())
			}
			continue
		}
		if !isArgoManaged(current) {
			return argoLifecycleObservation{Status: metav1.ConditionFalse, Reason: "ArgoResourcesNotOwned", Message: fmt.Sprintf("Shared Argo %s %s exists but is not owned by AI Pipelines", desired.GetKind(), desired.GetName())}
		}
		if desired.GetKind() == "CustomResourceDefinition" {
			established, err := argoCRDEstablished(current)
			if err != nil {
				return argoLifecycleObservation{Status: metav1.ConditionUnknown, Reason: "ArgoObservationFailed", Message: err.Error()}
			}
			if !established {
				drifted = append(drifted, desired.GetKind()+"/"+desired.GetName())
				continue
			}
		}
		if current.GetAnnotations()[argoManagedAnnotation] != "true" ||
			current.GetLabels()[legacyPipelinesComponentLabel] != "true" ||
			current.GetLabels()[config.DSPVersionk8sLabel] != config.DSPV2VersionString ||
			(desired.GetKind() != "CustomResourceDefinition" && !metav1.IsControlledBy(current, module)) ||
			!argoManagedFieldsMatch(current, desired) {
			drifted = append(drifted, desired.GetKind()+"/"+desired.GetName())
		}
	}

	if len(missing)+len(drifted)+len(remaining) > 0 {
		items := append(append(missing, drifted...), remaining...)
		sort.Strings(items)
		return argoLifecycleObservation{Status: metav1.ConditionFalse, Reason: "ArgoResourcesProgressing", Message: "Waiting for shared Argo resources: " + strings.Join(items, ", ")}
	}
	if state == common.Removed {
		return argoLifecycleObservation{Status: metav1.ConditionTrue, Reason: "Removed", Message: "Shared Argo controller resources are removed; CRDs are preserved"}
	}
	return argoLifecycleObservation{Status: metav1.ConditionTrue, Reason: "ArgoResourcesReady", Message: "Shared Argo resources are ready"}
}

func argoCRDEstablished(crd *unstructured.Unstructured) (bool, error) {
	value, found, err := unstructured.NestedFieldNoCopy(crd.Object, "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("read Established condition for CRD %s: %w", crd.GetName(), err)
	}
	if !found || value == nil {
		return false, nil
	}
	conditions, ok := value.([]interface{})
	if !ok {
		return false, fmt.Errorf("read Established condition for CRD %s: expected a condition list", crd.GetName())
	}
	for _, item := range conditions {
		condition, ok := item.(map[string]interface{})
		if ok && condition["type"] == string(apiextensionsv1.Established) && condition["status"] == string(metav1.ConditionTrue) {
			return true, nil
		}
	}
	return false, nil
}

func argoManagedFieldsMatch(current, desired *unstructured.Unstructured) bool {
	for _, path := range argoManagedPaths[desired.GetKind()] {
		wanted, wantedFound, err := unstructured.NestedFieldNoCopy(desired.Object, path...)
		if err != nil {
			return false
		}
		actual, actualFound, err := unstructured.NestedFieldNoCopy(current.Object, path...)
		if err != nil || wantedFound != actualFound {
			return false
		}
		if wantedFound && !containsJSONValue(actual, wanted) {
			return false
		}
	}
	return true
}

func containsJSONValue(actual, wanted interface{}) bool {
	switch wanted := wanted.(type) {
	case map[string]interface{}:
		actual, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}
		for key, value := range wanted {
			actualValue, found := actual[key]
			if !found || !containsJSONValue(actualValue, value) {
				return false
			}
		}
		return true
	case []interface{}:
		actual, ok := actual.([]interface{})
		if !ok || len(actual) != len(wanted) {
			return false
		}
		for i := range wanted {
			if !containsJSONValue(actual[i], wanted[i]) {
				return false
			}
		}
		return true
	default:
		return apiequality.Semantic.DeepEqual(actual, wanted)
	}
}
