package controllers

import (
	"context"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
	buildv1 "github.com/openshift/api/build/v1"
	imagev1 "github.com/openshift/api/image/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var visualizationServerTemplates = []string{
	"config/internal/visualization-server/deployment.yaml.tmpl",
	"config/internal/visualization-server/rolebinding.yaml.tmpl",
	"config/internal/visualization-server/sa.yaml.tmpl",
	"config/internal/visualization-server/service.yaml.tmpl",
}

const (
	visualizationServerBuildConfigTemplate = "config/internal/visualization-server/buildconfig.yaml.tmpl"
	visualizationServerImageStreamTemplate = "config/internal/visualization-server/imagestream.yaml.tmpl"
	visualizationBuildConfigName           = "visualization-server"
	visualizationImageName                 = "visualization-server"
	tensorFlowImageStreamName              = "tensorflow"
	tensorFlowImageStreamTemplate          = "config/internal/visualization-server/imagestream-tensorflow.yaml.tmpl"
)

func (r *DSPipelineReconciler) ReconcileVisualizationServer(dsp *dspipelinesiov1alpha1.DSPipeline,
	ctx context.Context, req ctrl.Request, params *DSPipelineParams) error {
	r.Log.Info("Applying VisualizationServer Resources")

	// Do not want to create multiple builds for the same image in the same namespace
	buildConfig := &buildv1.BuildConfig{}
	namespacedName := types.NamespacedName{
		Name:      visualizationBuildConfigName,
		Namespace: req.Namespace,
	}
	err := r.Get(ctx, namespacedName, buildConfig)
	if err != nil && apierrs.IsNotFound(err) {
		r.Log.Info("Specified VisualizationServer buildconfig not found, creating...")
		err := r.ApplyWithoutOwner(params, visualizationServerBuildConfigTemplate)
		if err != nil {
			return err
		}
	} else if err != nil {
		r.Log.Error(err, "Unable to fetch VisualizationServer buildconfig...")
		return err
	}

	// Do not want to create multiple imagestreams for the same image in the same namespace
	imageStream := &imagev1.ImageStream{}
	namespacedName = types.NamespacedName{
		Name:      visualizationImageName,
		Namespace: req.Namespace,
	}
	err = r.Get(ctx, namespacedName, imageStream)
	if err != nil && apierrs.IsNotFound(err) {
		r.Log.Info("Specified VisualizationServer ImageStream not found, creating...")
		err := r.ApplyWithoutOwner(params, visualizationServerImageStreamTemplate)
		if err != nil {
			return err
		}
	} else if err != nil {
		r.Log.Error(err, "Unable to fetch VisualizationServer ImageStream...")
		return err
	}

	// If DSP Is being installed standalone
	imageStream = &imagev1.ImageStream{}
	namespacedName = types.NamespacedName{
		Name:      tensorFlowImageStreamName,
		Namespace: req.Namespace,
	}
	err = r.Get(ctx, namespacedName, imageStream)
	if err != nil && apierrs.IsNotFound(err) {
		r.Log.Info("Specified TensorFlow ImageStream not found, creating...")
		err := r.ApplyWithoutOwner(params, tensorFlowImageStreamTemplate)
		if err != nil {
			return err
		}
	} else if err != nil {
		r.Log.Error(err, "Unable to fetch TensorFlow ImageStream...")
		return err
	}

	// Deploy remaining resources
	for _, template := range visualizationServerTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Finished applying VisualizationServer Resources")
	return nil
}

func (r *DSPipelineReconciler) CleanUpVisualizationServer(dsp *dspipelinesiov1alpha1.DSPipeline,
	ctx context.Context, req ctrl.Request, params *DSPipelineParams) error {

	// If there are no other DSP's in this namespace, cleanup the build artifacts
	dsPipelines := &dspipelinesiov1alpha1.DSPipelineList{}
	listOptions := client.ListOptions{
		Namespace: req.Namespace,
	}

	err := r.List(ctx, dsPipelines, &listOptions)
	if err != nil {
		return err
	}

	// The dsp found is being deleted during cleanup
	noDSPipelines := len(dsPipelines.Items) == 1

	if noDSPipelines {
		err := r.DeleteResource(params, visualizationServerImageStreamTemplate)
		if err != nil {
			return err
		}
		err = r.DeleteResource(params, visualizationServerBuildConfigTemplate)
		if err != nil {
			return err
		}
	}

	return nil
}
