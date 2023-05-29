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
		return fmt.Errorf("error loading template (%s) yaml: %w", template, err)
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
		return fmt.Errorf("error loading template (%s) yaml: %w", template, err)
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
		return fmt.Errorf("error loading template (%s) yaml: %w", template, err)
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
	condition.LastTransitionTime = metav1.Now()

	return condition
}

// isDeploymentInCondition evaluates if condition with "name" is in condition of type "conditionType".
// this procedure is valid only for conditions with bool status type, for conditions of non bool type
// results are undefined.
func (r *DSPAReconciler) isDeploymentInCondition(ctx context.Context,
	dspa *dspav1alpha1.DataSciencePipelinesApplication, name string,
	conditionType appsv1.DeploymentConditionType) (bool, appsv1.DeploymentCondition) {
	found := &appsv1.Deployment{}

	// Every Deployment in DSPA is the name followed by the DSPA CR name
	component := name + "-" + dspa.Name

	err := r.Get(ctx, types.NamespacedName{Name: component, Namespace: dspa.Namespace}, found)
	if err == nil {
		if found.Spec.Replicas != nil && *found.Spec.Replicas == 0 {
			return false, appsv1.DeploymentCondition{}
		}
		for _, s := range found.Status.Conditions {
			if s.Type == conditionType && s.Status == corev1.ConditionTrue {
				return true, s
			}
		}
	}
	return false, appsv1.DeploymentCondition{}
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
	log := r.Log.WithValues("namespace", req.Namespace).WithValues("dspa_name", req.Name)

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

	err = r.ReconcileDatabase(ctx, dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileStorage(ctx, dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileCommon(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileAPIServer(ctx, dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcilePersistenceAgent(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileScheduledWorkflow(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileUI(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileMLMD(dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Updating CR status")
	// Refresh DSPA before updating
	err = r.Get(ctx, req.NamespacedName, dspa)
	if err != nil {
		log.Info(err.Error())
		return ctrl.Result{}, err
	}

	conditions := r.GenerateStatus(ctx, dspa)
	dspa.Status.Conditions = conditions

	// Update Status
	err = r.Status().Update(ctx, dspa)
	if err != nil {
		log.Info(err.Error())
		return ctrl.Result{}, err
	}

	r.PublishMetrics(
		dspa,
		GetConditionByType(config.APIServerReady, conditions),
		GetConditionByType(config.PersistenceAgentReady, conditions),
		GetConditionByType(config.ScheduledWorkflowReady, conditions),
		GetConditionByType(config.CrReady, conditions),
	)

	return ctrl.Result{}, nil
}

// GetConditionByType returns condition of type T if it exists in conditions, otherwise
// return empty condition struct.
func GetConditionByType(t string, conditions []metav1.Condition) metav1.Condition {
	for _, c := range conditions {
		if c.Type == t {
			return c
		}
	}
	return metav1.Condition{}
}

func (r *DSPAReconciler) GenerateStatus(ctx context.Context, dspa *dspav1alpha1.DataSciencePipelinesApplication) []metav1.Condition {
	var conditions []metav1.Condition

	apiServerReady := r.buildCondition(config.APIServerReady, dspa, config.MinimumReplicasAvailable)
	deploymentAvailable, _ := r.isDeploymentInCondition(ctx, dspa, "ds-pipeline", appsv1.DeploymentAvailable)
	if deploymentAvailable {
		apiServerReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, apiServerReady)

	persistenceAgentReady := r.buildCondition(config.PersistenceAgentReady, dspa, config.MinimumReplicasAvailable)
	deploymentAvailable, _ = r.isDeploymentInCondition(ctx, dspa, "ds-pipeline-persistenceagent", appsv1.DeploymentAvailable)
	if deploymentAvailable {
		persistenceAgentReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, persistenceAgentReady)

	scheduledWorkflowReady := r.buildCondition(config.ScheduledWorkflowReady, dspa, config.MinimumReplicasAvailable)
	deploymentAvailable, _ = r.isDeploymentInCondition(ctx, dspa, "ds-pipeline-scheduledworkflow", appsv1.DeploymentAvailable)
	if deploymentAvailable {
		scheduledWorkflowReady.Status = metav1.ConditionTrue
	}
	conditions = append(conditions, scheduledWorkflowReady)

	crReady := r.buildCondition(config.CrReady, dspa, config.MinimumReplicasAvailable)
	crReady.Type = config.CrReady

	// Compute Ready Logic for the CR
	if (apiServerReady.Status == metav1.ConditionTrue) &&
		(persistenceAgentReady.Status == metav1.ConditionTrue) &&
		(scheduledWorkflowReady.Status == metav1.ConditionTrue) {
		crReady.Status = metav1.ConditionTrue
	} else {
		crReady.Status = metav1.ConditionFalse
	}
	conditions = append(conditions, crReady)

	for i, condition := range dspa.Status.Conditions {
		if condition.Status == conditions[i].Status {
			conditions[i].LastTransitionTime = condition.LastTransitionTime
		}
	}

	return conditions
}

func (r *DSPAReconciler) PublishMetrics(dspa *dspav1alpha1.DataSciencePipelinesApplication,
	apiServerReady, persistenceAgentReady, scheduledWorkflowReady,
	crReady metav1.Condition) {
	log := r.Log.WithValues("namespace", dspa.Namespace).WithValues("dspa_name", dspa.Name)
	log.Info("Publishing Ready Metrics")
	if apiServerReady.Status == metav1.ConditionTrue {
		log.Info("APIServer Ready")
		APIServerReadyMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(1)
	} else {
		log.Info("APIServer Not Ready")
		APIServerReadyMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(0)
	}

	if persistenceAgentReady.Status == metav1.ConditionTrue {
		log.Info("PersistanceAgent Ready")
		PersistenceAgentReadyMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(1)
	} else {
		log.Info("PersistanceAgent Not Ready")
		PersistenceAgentReadyMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(0)
	}

	if scheduledWorkflowReady.Status == metav1.ConditionTrue {
		log.Info("ScheduledWorkflow Ready")
		ScheduledWorkflowReadyMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(1)
	} else {
		log.Info("ScheduledWorkflow Not Ready")
		ScheduledWorkflowReadyMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(0)
	}

	if crReady.Status == metav1.ConditionTrue {
		log.Info("CR Fully Ready")
		CrReadyMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(1)
	} else {
		log.Info("CR Not Ready")
		CrReadyMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(0)
	}
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
