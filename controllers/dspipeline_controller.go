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
	"github.com/go-logr/logr"

	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DSPipelineReconciler reconciles a DSPipelineParams object
type DSPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=dspipelines.opendatahub.io,resources=dspipelines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dspipelines.opendatahub.io,resources=dspipelines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dspipelines.opendatahub.io,resources=dspipelines/finalizers,verbs=update

func (r *DSPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace)

	log.V(1).Info("DSPipelineParams Reconciler called.")

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

	err = params.ExtractParams(dspipeline)
	if err != nil {
		log.Error(err, "Unable to parse CR spec, "+
			"failed to reconcile, ensure CR is well formed")
		return ctrl.Result{}, err
	}

	usingCustomDB, err := params.UsingCustomDB(dspipeline)
	if err != nil {
		return ctrl.Result{}, err
	}

	usingCustomStorage, err := params.UsingCustomStorage(dspipeline)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !usingCustomStorage {
		err := r.ReconcileStorage(dspipeline, ctx, params)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if !usingCustomDB {
		err = r.ReconcileDatabase(dspipeline, ctx, params)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// TODO: Ensure db/storage (if deploying custom) are running before
	// deploying dsp stack

	err = r.ReconcileAPIServer(dspipeline, ctx, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileUI(dspipeline, ctx, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcilePersistenceAgent(dspipeline, ctx, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileScheduledWorkflow(dspipeline, ctx, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileViewerCRD(dspipeline, ctx, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.ReconcileVisualizationServer(dspipeline, ctx, params)
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
		Complete(r)
}
