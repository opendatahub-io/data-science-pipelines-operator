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
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const finalizerName = "dspipelines.opendatahub.io/finalizer"

// DSPipelineReconciler reconciles a DSPipelineParams object
type DSPipelineReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Log           logr.Logger
	TemplatesPath string
}

func (r *DSPipelineReconciler) Apply(owner mf.Owner, params *DSPipelineParams, template string, fns ...mf.Transformer) error {
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

	if err = tmplManifest.Apply(); err != nil {
		return err
	}
	return nil
}

func (r *DSPipelineReconciler) ApplyWithoutOwner(params *DSPipelineParams, template string, fns ...mf.Transformer) error {
	tmplManifest, err := config.Manifest(r.Client, r.TemplatesPath+template, params)
	if err != nil {
		return fmt.Errorf("error loading template yaml: %w", err)
	}

	tmplManifest, err = tmplManifest.Transform(fns...)
	if err != nil {
		return err
	}

	if err = tmplManifest.Apply(); err != nil {
		return err
	}
	return nil
}

func (r *DSPipelineReconciler) DeleteResource(params *DSPipelineParams, template string, fns ...mf.Transformer) error {
	tmplManifest, err := config.Manifest(r.Client, r.TemplatesPath+template, params)
	if err != nil {
		return fmt.Errorf("error loading template yaml: %w", err)
	}

	tmplManifest, err = tmplManifest.Transform(fns...)
	if err != nil {
		return err
	}

	if err = tmplManifest.Delete(); err != nil {
		return err
	}
	return nil
}

//+kubebuilder:rbac:groups=dspipelines.opendatahub.io,resources=dspipelines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dspipelines.opendatahub.io,resources=dspipelines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dspipelines.opendatahub.io,resources=dspipelines/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
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

func (r *DSPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace)

	log.V(1).Info("DSPipeline Reconciler called.")

	params := &DSPipelineParams{}

	dspipeline := &dspipelinesiov1alpha1.DSPipeline{}
	err := r.Get(ctx, req.NamespacedName, dspipeline)
	if err != nil && apierrs.IsNotFound(err) {
		log.Info("Stop DSPipelineParams reconciliation")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Unable to fetch the DSPipelineParams")
		return ctrl.Result{}, err
	}

	// FixMe: Hack for stubbing gvk during tests as these are not populated by test suite
	// https://github.com/opendatahub-io/data-science-pipelines-operator/pull/7#discussion_r1102887037
	// In production we expect these to be populated
	if dspipeline.Kind == "" {
		dspipeline = dspipeline.DeepCopy()
		gvk := dspipelinesiov1alpha1.GroupVersion.WithKind("DSPipeline")
		dspipeline.APIVersion, dspipeline.Kind = gvk.Version, gvk.Kind
	}

	err = params.ExtractParams(dspipeline)
	if err != nil {
		log.Error(err, "Unable to parse CR spec, "+
			"failed to reconcile, ensure CR is well formed")
		return ctrl.Result{}, err
	}

	if dspipeline.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(dspipeline, finalizerName) {
			controllerutil.AddFinalizer(dspipeline, finalizerName)
			if err := r.Update(ctx, dspipeline); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(dspipeline, finalizerName) {
			if err := r.cleanUpResources(ctx, req, dspipeline, params); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(dspipeline, finalizerName)
			if err := r.Update(ctx, dspipeline); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	usingCustomDB, err := params.UsingExternalDB(dspipeline)
	if err != nil {
		return ctrl.Result{}, err
	}

	usingCustomStorage, err := params.UsingExternalStorage(dspipeline)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !usingCustomStorage {
		err := r.ReconcileStorage(ctx, dspipeline, req, params)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if !usingCustomDB {
		err = r.ReconcileDatabase(ctx, dspipeline, req, params)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// TODO: Ensure db/storage (if deploying custom) are running before
	// Use status fields to conditionally deploy

	err = r.ReconcileAPIServer(ctx, dspipeline, req, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcilePersistenceAgent(dspipeline, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileScheduledWorkflow(dspipeline, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileUI(dspipeline, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileViewerCRD(dspipeline, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DSPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dspipelinesiov1alpha1.DSPipeline{}).
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

// Clean Up any resources not handled by garbage collection, like Cluster Resources
func (r *DSPipelineReconciler) cleanUpResources(ctx context.Context, req ctrl.Request, dsp *dspipelinesiov1alpha1.DSPipeline, params *DSPipelineParams) error {
	err := r.CleanUpUI(params)
	if err != nil {
		return err
	}

	return nil
}
