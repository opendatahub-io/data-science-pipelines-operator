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
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/go-logr/logr"
	mf "github.com/manifestival/manifestival"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/util"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const finalizerName = "datasciencepipelinesapplications.opendatahub.io/finalizer"

// DSPAReconciler reconciles a DSPAParams object
type DSPAReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Log           logr.Logger
	TemplatesPath string
}

func (r *DSPAReconciler) ApplyDir(owner mf.Owner, params *DSPAParams, directory string, fns ...mf.Transformer) error {
	templates, err := util.GetTemplatesInDir(r.TemplatesPath, directory)
	if err != nil {
		return err
	}
	return r.ApplyAll(owner, params, templates)
}

func (r *DSPAReconciler) ApplyAll(owner mf.Owner, params *DSPAParams, templates []string, fns ...mf.Transformer) error {
	for _, template := range templates {
		err := r.Apply(owner, params, template)
		if err != nil {
			return err
		}
	}
	return nil
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
//+kubebuilder:rbac:groups=mcad.ibm.com,resources=appwrappers,verbs=create;get;list;patch;delete
//+kubebuilder:rbac:groups=ray.io,resources=rayclusters;rayjobs;rayservices,verbs=create;get;list;patch;delete
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=image.openshift.io,resources=imagestreamtags,verbs=get
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch;list
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workload.codeflare.dev,resources=appwrappers;appwrappers/finalizers;appwrappers/status,verbs=create;delete;deletecollection;get;list;patch;update;watch

func (r *DSPAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace).WithValues("dspa_name", req.Name)

	log.V(1).Info("DataSciencePipelinesApplication Reconciler called.")

	params := &DSPAParams{}

	dspa := &dspav1alpha1.DataSciencePipelinesApplication{}
	err := r.Get(ctx, req.NamespacedName, dspa)
	if err != nil && apierrs.IsNotFound(err) {
		log.Info("DSPA resource was not found")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Encountered error when fetching DSPA")
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
		return ctrl.Result{Requeue: true, RequeueAfter: 2 * time.Minute}, nil
	}

	err = r.ReconcileDatabase(ctx, dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileStorage(ctx, dspa, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get Prereq Status (DB and ObjStore Ready)
	dbAvailable := r.isDatabaseAccessible(ctx, dspa, params)
	objStoreAvailable := r.isObjectStorageAccessible(ctx, dspa, params)
	dspaPrereqsReady := (dbAvailable && objStoreAvailable)

	if dspaPrereqsReady {
		// Manage Common Manifests
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

		err = r.ReconcileCRDViewer(dspa, params)
		if err != nil {
			return ctrl.Result{}, err
		}

		err = r.ReconcileVisualizationServer(dspa, params)
		if err != nil {
			return ctrl.Result{}, err
		}

		err = r.ReconcileWorkflowController(dspa, params)
		if err != nil {
			return ctrl.Result{}, err
		}

	}

	log.Info("Updating CR status")
	// Refresh DSPA before updating
	err = r.Get(ctx, req.NamespacedName, dspa)
	if err != nil {
		log.Info(err.Error())
		return ctrl.Result{}, err
	}

	conditions, err := r.GenerateStatus(ctx, dspa, dbAvailable, objStoreAvailable)
	if err != nil {
		log.Info(err.Error())
		return ctrl.Result{}, err
	}
	dspa.Status.Conditions = conditions

	// Update Status
	err = r.Status().Update(ctx, dspa)
	if err != nil {
		log.Info(err.Error())
		return ctrl.Result{}, err
	}

	r.PublishMetrics(
		dspa,
		util.GetConditionByType(config.DatabaseAvailable, conditions),
		util.GetConditionByType(config.ObjectStoreAvailable, conditions),
		util.GetConditionByType(config.APIServerReady, conditions),
		util.GetConditionByType(config.PersistenceAgentReady, conditions),
		util.GetConditionByType(config.ScheduledWorkflowReady, conditions),
		util.GetConditionByType(config.CrReady, conditions),
	)
	return ctrl.Result{}, nil
}

// handleReadyCondition evaluates if condition with "name" is in condition of type "conditionType".
// this procedure is valid only for conditions with bool status type, for conditions of non bool type
// results are undefined.
func (r *DSPAReconciler) handleReadyCondition(ctx context.Context, dspa *dspav1alpha1.DataSciencePipelinesApplication, name string, condition string) (metav1.Condition, error) {
	readyCondition := r.buildCondition(condition, dspa, config.MinimumReplicasAvailable)
	deployment := &appsv1.Deployment{}

	// Every Deployment in DSPA is the name followed by the DSPA CR name
	component := name + "-" + dspa.Name

	err := r.Get(ctx, types.NamespacedName{Name: component, Namespace: dspa.Namespace}, deployment)
	if err != nil {
		if apierrs.IsNotFound(err) {
			readyCondition.Reason = config.ComponentDeploymentNotFound
			readyCondition.Status = metav1.ConditionFalse
			readyCondition.Message = fmt.Sprintf("Deployment for component \"%s\" is missing - pre-requisite component may not yet be available.", component)
			return readyCondition, nil
		} else {
			return metav1.Condition{}, err
		}
	}

	// First check if deployment is scaled down, if it is, component is deemed not ready
	if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 0 {
		readyCondition.Reason = config.MinimumReplicasAvailable
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Message = fmt.Sprintf("Deployment for component \"%s\" is scaled down.", component)
		return readyCondition, nil
	}

	// At this point component is not minimally available, possible scenarios:
	// 1. Component deployment has encountered errors
	// 2. Component is still deploying
	// We check for (1), and if no errors are found we presume (2)

	progressingCond := util.GetDeploymentCondition(deployment.Status, appsv1.DeploymentProgressing)
	availableCond := util.GetDeploymentCondition(deployment.Status, appsv1.DeploymentAvailable)
	replicaFailureCond := util.GetDeploymentCondition(deployment.Status, appsv1.DeploymentReplicaFailure)

	if availableCond != nil && availableCond.Status == corev1.ConditionTrue {
		// If this DSPA component is minimally available, we are done.
		readyCondition.Reason = config.MinimumReplicasAvailable
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Message = fmt.Sprintf("Component [%s] is minimally available.", component)
		return readyCondition, nil
	}

	// There are two possible reasons for progress failing, deadline and replica create error:
	// https://github.com/kubernetes/kubernetes/blob/release-1.27/pkg/controller/deployment/util/deployment_util.go#L69
	// We check for both to investigate potential issues during deployment
	if progressingCond != nil && progressingCond.Status == corev1.ConditionFalse &&
		(progressingCond.Reason == "ProgressDeadlineExceeded" || progressingCond.Reason == "ReplicaSetCreateError") {
		readyCondition.Reason = config.FailingToDeploy
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Message = fmt.Sprintf("Component [%s] has failed to progress. Reason: [%s]. "+
			"Message: [%s]", component, progressingCond.Reason, progressingCond.Message)
		return readyCondition, nil
	}

	if replicaFailureCond != nil && replicaFailureCond.Status == corev1.ConditionTrue {
		readyCondition.Reason = config.FailingToDeploy
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Message = fmt.Sprintf("Component's replica [%s] has failed to create. Reason: [%s]. "+
			"Message: [%s]", component, replicaFailureCond.Reason, replicaFailureCond.Message)
		return readyCondition, nil
	}

	// Search through the pods associated with this deployment
	// if a failed pod is encountered, report Ready=false with failure
	// message
	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.MatchingLabels(deployment.Spec.Selector.MatchLabels),
	}
	err = r.Client.List(ctx, podList, opts...)
	if err != nil {
		return metav1.Condition{}, err
	}

	hasPodFailures := false
	podFailureMessage := ""
	// We loop through all pods within this deployment and inspect their statuses for failures
	// Any failure detected in any pod results in FailingToDeploy status
	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodFailed {
			hasPodFailures = true
			podFailureMessage += fmt.Sprintf("Pod named [%s] that is associated with this component [%s] "+
				"is in failed phase.", p.Name, component)
		}
		// We loop through the containers in each pod, as in some cases the Pod can be in pending state
		// but an individual container may be failing due to runtime errors.
		for _, c := range p.Status.ContainerStatuses {
			if c.State.Waiting != nil && c.State.Waiting.Reason == "CrashLoopBackOff" {
				readyCondition.Reason = config.FailingToDeploy
				readyCondition.Status = metav1.ConditionFalse
				// We concatenate messages from all failing containers.
				readyCondition.Message = fmt.Sprintf("Component [%s] is in CrashLoopBackOff. "+
					"Message from pod: [%s]", component, c.State.Waiting.Message)
				return readyCondition, nil
			}
		}
	}

	if hasPodFailures {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = config.FailingToDeploy
		readyCondition.Message = podFailureMessage
		return readyCondition, nil
	}

	// No errors encountered, assume deployment is progressing successfully
	// If this DSPA component is minimally available, we are done.
	readyCondition.Reason = config.Deploying
	readyCondition.Status = metav1.ConditionFalse
	readyCondition.Message = fmt.Sprintf("Component [%s] is deploying.", component)
	return readyCondition, nil

}

func (r *DSPAReconciler) GenerateStatus(ctx context.Context, dspa *dspav1alpha1.DataSciencePipelinesApplication, dbAvailableStatus, objStoreAvailableStatus bool) ([]metav1.Condition, error) {
	// Create Database Availability Condition
	databaseAvailable := r.buildCondition(config.DatabaseAvailable, dspa, config.DatabaseAvailable)
	if dbAvailableStatus {
		databaseAvailable.Status = metav1.ConditionTrue
		databaseAvailable.Message = "Database connectivity successfully verified"
	} else {
		databaseAvailable.Message = "Could not connect to database"
	}

	// Create Object Storage Availability Condition
	objStoreAvailable := r.buildCondition(config.ObjectStoreAvailable, dspa, config.ObjectStoreAvailable)
	if objStoreAvailableStatus {
		objStoreAvailable.Status = metav1.ConditionTrue
		objStoreAvailable.Message = "Object Store connectivity successfully verified"
	} else {
		objStoreAvailable.Message = "Could not connect to Object Store"
	}

	// Create APIServer Readiness Condition
	apiServerReady, err := r.handleReadyCondition(ctx, dspa, "ds-pipeline", config.APIServerReady)
	if err != nil {
		return []metav1.Condition{}, err
	}

	// Create PersistenceAgent Readiness Condition
	persistenceAgentReady, err := r.handleReadyCondition(ctx, dspa, "ds-pipeline-persistenceagent", config.PersistenceAgentReady)
	if err != nil {
		return []metav1.Condition{}, err
	}

	// Create ScheduledWorkflow Readiness Condition
	scheduledWorkflowReady, err := r.handleReadyCondition(ctx, dspa, "ds-pipeline-scheduledworkflow", config.ScheduledWorkflowReady)
	if err != nil {
		return []metav1.Condition{}, err
	}

	var conditions []metav1.Condition
	conditions = append(conditions, databaseAvailable)
	conditions = append(conditions, objStoreAvailable)
	conditions = append(conditions, apiServerReady)
	conditions = append(conditions, persistenceAgentReady)
	conditions = append(conditions, scheduledWorkflowReady)

	// Compute Ready Logic for the CR
	crReady := r.buildCondition(config.CrReady, dspa, config.MinimumReplicasAvailable)
	crReady.Type = config.CrReady

	componentConditions := []metav1.Condition{databaseAvailable, objStoreAvailable, apiServerReady, persistenceAgentReady, scheduledWorkflowReady}
	allReady := true
	failureMessages := ""
	for _, c := range componentConditions {
		if c.Status == metav1.ConditionFalse {
			allReady = false
			failureMessages += fmt.Sprintf("%s \n", c.Message)
		}
	}

	if allReady {
		crReady.Status = metav1.ConditionTrue
		crReady.Message = "All components are ready."
	} else {
		crReady.Status = metav1.ConditionFalse
		crReady.Message = failureMessages
	}
	conditions = append(conditions, crReady)

	for i, condition := range dspa.Status.Conditions {
		if condition.Status == conditions[i].Status {
			conditions[i].LastTransitionTime = condition.LastTransitionTime
		}
	}

	return conditions, nil
}

func (r *DSPAReconciler) PublishMetrics(dspa *dspav1alpha1.DataSciencePipelinesApplication,
	dbAvailable, objStoreAvailable, apiServerReady, persistenceAgentReady, scheduledWorkflowReady,
	crReady metav1.Condition) {
	log := r.Log.WithValues("namespace", dspa.Namespace).WithValues("dspa_name", dspa.Name)
	log.Info("Publishing Ready Metrics")
	if dbAvailable.Status == metav1.ConditionTrue {
		log.Info("Database Accessible")
		DBAvailableMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(1)
	} else {
		log.Info("Database Not Yet Accessible")
		DBAvailableMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(0)
	}

	if objStoreAvailable.Status == metav1.ConditionTrue {
		log.Info("Object Store Accessible")
		ObjectStoreAvailableMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(1)
	} else {
		log.Info("Object Store Not Yet Accessible")
		ObjectStoreAvailableMetric.WithLabelValues(dspa.Name, dspa.Namespace).Set(0)
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
		// Watch for Pods belonging to DSPA
		Watches(&source.Kind{Type: &corev1.Pod{}},
			handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
				log := r.Log.WithValues("namespace", o.GetNamespace())

				component, hasComponentLabel := o.GetLabels()["component"]

				if !hasComponentLabel || (component != "data-science-pipelines") {
					return []reconcile.Request{}
				}

				dspaName, hasDSPALabel := o.GetLabels()["dspa"]
				if !hasDSPALabel {
					msg := fmt.Sprintf("Pod with data-science-pipelines label encountered, but is missing dspa "+
						"label, could not reconcile on [Pod: %s] ", o.GetName())
					log.V(1).Info(msg)
					return []reconcile.Request{}
				}

				log.V(1).Info(fmt.Sprintf("Reconcile event triggered by [Pod: %s] ", o.GetName()))
				namespacedName := types.NamespacedName{
					Name:      dspaName,
					Namespace: o.GetNamespace(),
				}
				reconcileRequests := append([]reconcile.Request{}, reconcile.Request{NamespacedName: namespacedName})
				return reconcileRequests
			})).
		// TODO: Add watcher for ui cluster rbac since it has no owner
		WithOptions(controller.Options{
			MaxConcurrentReconciles: config.DefaultMaxConcurrentReconciles,
		}).
		Complete(r)
}

// Clean Up any resources not handled by garbage collection, like Cluster ResourceRequirements
func (r *DSPAReconciler) cleanUpResources(params *DSPAParams) error {
	return r.CleanUpCommon(params)
}
