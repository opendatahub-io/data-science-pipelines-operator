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

	"github.com/go-logr/logr"
	mf "github.com/manifestival/manifestival"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const finalizerName = "datasciencepipelinesapplications.opendatahub.io/finalizer"

// DSPAReconciler reconciles a DSPAParams object
type DSPAReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Log           logr.Logger
	TemplatesPath string
}

func (r *DSPAReconciler) Apply(owner mf.Owner, params *DSPAParams, template string, fns ...mf.Transformer) error {
	tmplManifest, err := config.Manifest(r.Client, r.TemplatesPath+template, params)
	if err != nil {
		return fmt.Errorf("error loading template yaml: %w", err)
	}
	tmplManifest, err = tmplManifest.Transform(
		mf.InjectOwner(owner),
	)
	if err != nil {
		return err
	}

	tmplManifest, err = tmplManifest.Transform(fns...)
	if err != nil {
		return err
	}

	return tmplManifest.Apply()
}

func (r *DSPAReconciler) ApplyWithoutOwner(params *DSPAParams, template string, fns ...mf.Transformer) error {
	tmplManifest, err := config.Manifest(r.Client, r.TemplatesPath+template, params)
	if err != nil {
		return fmt.Errorf("error loading template yaml: %w", err)
	}

	tmplManifest, err = tmplManifest.Transform(fns...)
	if err != nil {
		return err
	}

	return tmplManifest.Apply()
}

func (r *DSPAReconciler) DeleteResource(params *DSPAParams, template string, fns ...mf.Transformer) error {
	tmplManifest, err := config.Manifest(r.Client, r.TemplatesPath+template, params)
	if err != nil {
		return fmt.Errorf("error loading template yaml: %w", err)
	}

	tmplManifest, err = tmplManifest.Transform(fns...)
	if err != nil {
		return err
	}

	return tmplManifest.Delete()
}

func (r *DSPAReconciler) DeleteResourceIfItExists(ctx context.Context, obj client.Object, nn types.NamespacedName) error {
	err := r.Get(ctx, nn, obj)
	if err == nil {
		err = r.Delete(ctx, obj)
	} else if apierrs.IsNotFound(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	return err
}

func (r *DSPAReconciler) buildCondition(conditionType string, dspa *dspav1alpha1.DataSciencePipelinesApplication, reason string) metav1.Condition {
	condition := metav1.Condition{}
	condition.Type = conditionType
	condition.ObservedGeneration = dspa.Generation
	condition.Status = metav1.ConditionFalse
	condition.Reason = reason

	if dspa.Status.Conditions == nil {
		condition.LastTransitionTime = metav1.Now()
	}

	return condition
}

func (r *DSPAReconciler) isDeploymentAvailable(ctx context.Context, dspa *dspav1alpha1.DataSciencePipelinesApplication, name string) bool {
	found := &appsv1.Deployment{}

	// Every Deployment in DSPA is the name followed by the DSPA CR name
	component := name + "-" + dspa.Name

	err := r.Get(ctx, types.NamespacedName{Name: component, Namespace: dspa.Namespace}, found)
	if err == nil {
		for _, s := range found.Status.Conditions {
			if s.Type == "Available" && s.Status == corev1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

//+kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=*,resources=deployments;services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets;configmaps;services;serviceaccounts;persistentvolumes;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumes;persistentvolumeclaims,verbs=*
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=create;delete;get
//+kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=*
//+kubebuilder:rbac:groups=core,resources=pods;pods/exec;pods/log;services,verbs=*
//+kubebuilder:rbac:groups=core;apps;extensions,resources=deployments;replicasets,verbs=*
//+kubebuilder:rbac:groups=kubeflow.org,resources=*,verbs=*
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=*
//+kubebuilder:rbac:groups=machinelearning.seldon.io,resources=seldondeployments,verbs=*
//+kubebuilder:rbac:groups=tekton.dev,resources=*,verbs=*
//+kubebuilder:rbac:groups=custom.tekton.dev,resources=pipelineloops,verbs=*
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=image.openshift.io,resources=imagestreamtags,verbs=get
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch;list
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

func (r *DSPAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace)

	log.V(1).Info("DataSciencePipelinesApplication Reconciler called.")

	params := &DSPAParams{}

	dspa := &dspav1alpha1.DataSciencePipelinesApplication{}
	err := r.Get(ctx, req.NamespacedName, dspa)
	if err != nil && apierrs.IsNotFound(err) {
		log.Info("Stop DSPAParams reconciliation")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Unable to fetch the DSPAParams")
		return ctrl.Result{}, err
	}

	// FixMe: Hack for stubbing gvk during tests as these are not populated by test suite
	// https://github.com/opendatahub-io/data-science-pipelines-operator/pull/7#discussion_r1102887037
	// In production we expect these to be populated
	if dspa.Kind == "" {
		dspa = dspa.DeepCopy()
		gvk := dspav1alpha1.GroupVersion.WithKind("DataSciencePipelinesApplication")
		dspa.APIVersion, dspa.Kind = gvk.Version, gvk.Kind
	}

	// Ensure that empty values do not overwrite desired state
	if dspa.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(dspa, finalizerName) {
			controllerutil.AddFinalizer(dspa, finalizerName)
			if err := r.Update(ctx, dspa); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(dspa, finalizerName) {
			params.Name = dspa.Name
			params.Namespace = dspa.Namespace
			if err := r.cleanUpResources(params); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(dspa, finalizerName)
			if err := r.Update(ctx, dspa); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	err = params.ExtractParams(ctx, dspa, r.Client, r.Log)
	if err != nil {
		log.Info(fmt.Sprintf("Encountered error when parsing CR: [%s]", err))
		return ctrl.Result{}, nil
	}

	// Initialize conditions
	var conditions []metav1.Condition

	databaseReady := r.buildCondition(config.DatabaseReady, dspa, config.DatabaseReady)
	err = r.ReconcileDatabase(ctx, dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	if r.isDeploymentAvailable(ctx, dspa, "mariadb") {
		databaseReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, databaseReady)

	objectStorageReady := r.buildCondition(config.ObjectStorageReady, dspa, config.ObjectStorageReady)
	err = r.ReconcileStorage(ctx, dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	if r.isDeploymentAvailable(ctx, dspa, "minio") {
		objectStorageReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, objectStorageReady)

	err = r.ReconcileCommon(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	apiServerReady := r.buildCondition(config.APIServerReady, dspa, config.MinimumReplicasAvailable)
	err = r.ReconcileAPIServer(ctx, dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	if r.isDeploymentAvailable(ctx, dspa, "ds-pipeline") {
		apiServerReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, apiServerReady)

	persistenceAgentReady := r.buildCondition(config.PersistenceAgentReady, dspa, config.MinimumReplicasAvailable)
	err = r.ReconcilePersistenceAgent(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	if r.isDeploymentAvailable(ctx, dspa, "ds-pipeline-persistenceagent") {
		persistenceAgentReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, persistenceAgentReady)

	scheduledWorkflowReady := r.buildCondition(config.ScheduledWorkflowReady, dspa, config.MinimumReplicasAvailable)
	err = r.ReconcileScheduledWorkflow(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	if r.isDeploymentAvailable(ctx, dspa, "ds-pipeline-scheduledworkflow") {
		scheduledWorkflowReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, scheduledWorkflowReady)

	userInterfaceReady := r.buildCondition(config.UserInterfaceReady, dspa, config.MinimumReplicasAvailable)
	err = r.ReconcileUI(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	if r.isDeploymentAvailable(ctx, dspa, "ds-pipeline-ui") {
		userInterfaceReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, userInterfaceReady)

	err = r.ReconcileViewerCRD(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Updating CR status")

	crReady := r.buildCondition(config.CrReady, dspa, config.MinimumReplicasAvailable)
	crReady.Type = config.CrReady

	// Compute Ready Logic for the CR
	if ((apiServerReady.Status == metav1.ConditionTrue) &&
		(persistenceAgentReady.Status == metav1.ConditionTrue) &&
		(scheduledWorkflowReady.Status == metav1.ConditionTrue) &&
		(databaseReady.Status == metav1.ConditionTrue) &&
		(objectStorageReady.Status == metav1.ConditionTrue)) &&
		(userInterfaceReady.Status == metav1.ConditionTrue || (userInterfaceReady.Status == metav1.ConditionFalse && !dspa.Spec.MlPipelineUI.Deploy)) {
		crReady.Status = metav1.ConditionTrue
	} else {
		crReady.Status = metav1.ConditionFalse
	}
	conditions = append(conditions, crReady)

	for i, condition := range dspa.Status.Conditions {
		if condition.Status != conditions[i].Status {
			conditions[i].LastTransitionTime = metav1.Now()
		}
	}
	dspa.Status.Conditions = conditions

	err = r.Status().Update(ctx, dspa)
	if err != nil {
		log.Info(err.Error())
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DSPAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dspav1alpha1.DataSciencePipelinesApplication{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&routev1.Route{}).
		// TODO: Add watcher for ui cluster rbac since it has no owner
		Complete(r)
}

// Clean Up any resources not handled by garbage collection, like Cluster ResourceRequirements
func (r *DSPAReconciler) cleanUpResources(params *DSPAParams) error {
	return r.CleanUpCommon(params)
}
