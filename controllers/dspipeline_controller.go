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
	"github.com/opendatahub-io/ds-pipelines-controller/controllers/config"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const finalizerName = "dspipelines.opendatahub.io/finalizer"

// DSPipelineReconciler reconciles a DSPipelineParams object
type DSPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func (r *DSPipelineReconciler) Apply(owner mf.Owner, params *DSPipelineParams, template string, fns ...mf.Transformer) error {
	tmplManifest, err := config.Manifest(r.Client, template, params)
	if err != nil {
		return fmt.Errorf("error loading template yaml: %w", err)
	}
	tmplManifest, err = tmplManifest.Transform(
		mf.InjectOwner(owner),
	)

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
	tmplManifest, err := config.Manifest(r.Client, template, params)
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
	tmplManifest, err := config.Manifest(r.Client, template, params)
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

	if dspipeline.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(dspipeline, finalizerName) {
			controllerutil.AddFinalizer(dspipeline, finalizerName)
			if err := r.Update(ctx, dspipeline); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(dspipeline, finalizerName) {
			if err := r.cleanUpResources(dspipeline, params); err != nil {
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

	usingCustomDB, err := params.UsingCustomDB(dspipeline)
	if err != nil {
		return ctrl.Result{}, err
	}

	usingCustomStorage, err := params.UsingCustomStorage(dspipeline)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !usingCustomStorage {
		err := r.ReconcileStorage(dspipeline, ctx, req, params)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if !usingCustomDB {
		err = r.ReconcileDatabase(dspipeline, ctx, req, params)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// TODO: Ensure db/storage (if deploying custom) are running before
	// Use status fields to conditionally deploy

	err = r.ReconcileAPIServer(dspipeline, ctx, params)
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

	err = r.ReconcileVisualizationServer(dspipeline, params)
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
		Owns(&v1.Secret{}).
		Owns(&v1.Service{}).
		Owns(&v1.ConfigMap{}).
		Owns(&v1.ServiceAccount{}).
		Complete(r)
}

// Clean Up any resources not handled by garbage collection like Cluster Resources
func (r *DSPipelineReconciler) cleanUpResources(dsp *dspipelinesiov1alpha1.DSPipeline, params *DSPipelineParams) error {
	err := r.CleanUpPersistenceAgent(dsp, params)
	if err != nil {
		return err
	}

	err = r.CleanUpScheduledWorkflow(dsp, params)
	if err != nil {
		return err
	}

	return nil
}
