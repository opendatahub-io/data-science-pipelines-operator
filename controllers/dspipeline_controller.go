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

// DSPipelineReconciler reconciles a DSPipeline object
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

	log.V(1).Info("DSPipeline Reconciler called.")

	dspipeline := &dspipelinesiov1alpha1.DSPipeline{}
	err := r.Get(ctx, req.NamespacedName, dspipeline)
	if err != nil && apierrs.IsNotFound(err) {
		log.Info("Stop DSPipeline reconciliation")
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Unable to fetch the DSPipeline")
		return ctrl.Result{}, err
	}

	err = r.ReconcileAPIServer(dspipeline, ctx)
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
