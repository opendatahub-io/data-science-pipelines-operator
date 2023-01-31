package controllers

import (
	"context"
	"fmt"
	mf "github.com/manifestival/manifestival"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
	"github.com/opendatahub-io/ds-pipelines-controller/controllers/config"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	deploymentTemplate = "config/internal/apiserver/deployment.yaml.tmpl"
	defaultDBHost      = "mysql"
	defaultDBHostPort  = "3306"
	defaultMinioHost   = "minio-service.redhat-ods-applications.svc.cluster.local"
	defaultMinioPort   = "9000"
)

type Deployment struct {
	ServiceName                      string
	Name                             string
	Namespace                        string
	Owner                            mf.Owner
	ApiServerImage                   string
	ServiceAccountName               string
	PipelineRuntime                  string
	Client                           client.Client
	StripEOF                         string
	ArtifactScript                   string
	ArtifactImage                    string
	InjectDefaultScript              string
	ApplyTektonCustomResource        string
	TerminateStatus                  string
	AutoUpdatePipelineDefaultVersion string
	DBConfigCONMAXLifetimeSec        string
	VisualizationServerServiceHost   string
	VisualizaionServerServicePort    string

	ObjectStoreConfigBucketName string
	S3CredentialsSecretName     string
	AccessKeySecretKey          string
	SecretKey                   string
	ObjectStoreConfigSecure     string
	MinioServiceServiceHost     string
	MinioServiceServicePort     string

	ArtifactBucket              string
	ArtifactEndpoint            string
	ArtifactEndpointScheme      string
	ArtifactLogs                string
	ArtifactTracking            string
	ArtifactScriptConfigMapName string
	ArtifactScriptConfigMapKey  string

	DBUser              string
	DBPasswordSecretKey string
	DBPasswordSecret    string
	DBName              string
	DBHost              string
	DBPort              string
}

func (r *DSPipelineReconciler) ReconcileAPIServer(dsp *dspipelinesiov1alpha1.DSPipeline, req context.Context) error {
	r.Log.Info("Applying APIServer deployment")

	deployment := Deployment{
		ServiceName:                      dsp.Name,
		Name:                             dsp.Name,
		Namespace:                        dsp.Namespace,
		Owner:                            dsp,
		ApiServerImage:                   dsp.Spec.APIServer.Image,
		ServiceAccountName:               "ds-pipeline",
		PipelineRuntime:                  "tekton",
		StripEOF:                         "true",
		ObjectStoreConfigSecure:          "false",
		ArtifactEndpointScheme:           "http:/",
		ArtifactLogs:                     "false",
		ArtifactTracking:                 "true",
		ArtifactScriptConfigMapName:      dsp.Spec.APIServer.ArtifactScriptConfigMap.Name,
		ArtifactScriptConfigMapKey:       dsp.Spec.APIServer.ArtifactScriptConfigMap.Key,
		ArtifactImage:                    dsp.Spec.APIServer.ArtifactImage,
		InjectDefaultScript:              "true",
		ApplyTektonCustomResource:        "true",
		TerminateStatus:                  "Cancelled",
		AutoUpdatePipelineDefaultVersion: "true",
		DBConfigCONMAXLifetimeSec:        "120",
		VisualizationServerServiceHost:   "ds-pipeline-visualizationserver",
		VisualizaionServerServicePort:    "8888",
		Client:                           r.Client,
	}

	if dsp.Spec.Database.CustomDB != (dspipelinesiov1alpha1.CustomDB{}) {
		customDB := dsp.Spec.Database.CustomDB
		deployment.DBUser = customDB.Username
		deployment.DBName = customDB.DBName
		deployment.DBPasswordSecretKey = customDB.PasswordSecret.Key
		deployment.DBPasswordSecret = customDB.PasswordSecret.Name
		deployment.DBHost = customDB.Host
		deployment.DBPort = customDB.Port
	} else if dsp.Spec.Database.MariaDB != (dspipelinesiov1alpha1.MariaDB{}) {
		mariaDB := dsp.Spec.Database.MariaDB
		deployment.DBUser = mariaDB.Username
		deployment.DBName = mariaDB.DBName
		deployment.DBPasswordSecretKey = mariaDB.PasswordSecret.Key
		deployment.DBPasswordSecret = mariaDB.PasswordSecret.Name
		deployment.DBHost = defaultDBHost
		deployment.DBPort = defaultDBHostPort
	} else {
		return fmt.Errorf("no Database specified for DS-Pipeline resource")
	}

	if dsp.Spec.Storage.CustomStorage != (dspipelinesiov1alpha1.CustomStorage{}) {
		storage := dsp.Spec.Storage.CustomStorage
		deployment.ObjectStoreConfigBucketName = storage.Bucket
		deployment.S3CredentialsSecretName = storage.SecretName
		deployment.AccessKeySecretKey = storage.AccessKey
		deployment.SecretKey = storage.SecretKey
		deployment.MinioServiceServiceHost = storage.Host
		deployment.MinioServiceServicePort = storage.Port
		deployment.ArtifactBucket = storage.Bucket
		deployment.ArtifactEndpoint = storage.Host + ":" + storage.Port
	} else if dsp.Spec.Storage.Minio != (dspipelinesiov1alpha1.Minio{}) {
		storage := dsp.Spec.Storage.Minio
		deployment.ObjectStoreConfigBucketName = storage.Bucket
		deployment.S3CredentialsSecretName = storage.SecretName
		deployment.AccessKeySecretKey = storage.AccessKey
		deployment.SecretKey = storage.SecretKey
		deployment.MinioServiceServiceHost = defaultMinioHost
		deployment.MinioServiceServicePort = defaultMinioPort
		deployment.ArtifactBucket = storage.Bucket
		deployment.ArtifactEndpoint = defaultMinioHost + ":" + defaultMinioPort
	} else {
		return fmt.Errorf("no S3 storage specified for DS-Pipeline resource")
	}

	manifest, err := config.Manifest(deployment.Client, deploymentTemplate, deployment)
	if err != nil {
		return fmt.Errorf("error loading APIServer deployment yaml: %w", err)
	}

	manifest, err = manifest.Transform(
		mf.InjectOwner(dsp),
		func(resource *unstructured.Unstructured) error {
			var deployment = &appsv1.Deployment{}
			if tErr := scheme.Scheme.Convert(resource, deployment, nil); tErr != nil {
				return tErr
			}

			return scheme.Scheme.Convert(resource, deployment, nil)
		},
	)

	if err = manifest.Apply(); err != nil {
		return err
	}

	return nil
}
