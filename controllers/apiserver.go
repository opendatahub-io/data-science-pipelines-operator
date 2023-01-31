package controllers

import (
	"context"
	"fmt"
	mf "github.com/manifestival/manifestival"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/ds-pipelines-controller/api/v1alpha1"
	"github.com/opendatahub-io/ds-pipelines-controller/controllers/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultDBHost                     = "mysql"
	defaultDBHostPort                 = "3306"
	defaultMinioHost                  = "minio-service.redhat-ods-applications.svc.cluster.local"
	defaultMinioPort                  = "9000"
	defaultArtifactScriptConfigMap    = "ds-pipeline-artifact-script-sample"
	defaultArtifactScriptConfigMapKey = "artifact_script"
)

var templates = []string{
	"config/internal/apiserver/deployment.yaml.tmpl",
	"config/internal/apiserver/service.yaml.tmpl",
	"config/internal/apiserver/sa.yaml.tmpl",
	"config/internal/apiserver/artifact_script.yaml.tmpl",
}

type ApiServerFields struct {
	ServiceName                      string
	Name                             string
	Namespace                        string
	Owner                            mf.Owner
	ApiServerImage                   string
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
	ObjectStoreConfigBucketName      string
	S3CredentialsSecretName          string
	AccessKeySecretKey               string
	SecretKey                        string
	ObjectStoreConfigSecure          string
	MinioServiceServiceHost          string
	MinioServiceServicePort          string
	ArtifactBucket                   string
	ArtifactEndpoint                 string
	ArtifactEndpointScheme           string
	ArtifactLogs                     string
	ArtifactTracking                 string
	ArtifactScriptConfigMapName      string
	ArtifactScriptConfigMapKey       string
	DBUser                           string
	DBPasswordSecretKey              string
	DBPasswordSecret                 string
	DBName                           string
	DBHost                           string
	DBPort                           string
}

func (r *DSPipelineReconciler) ReconcileAPIServer(dsp *dspipelinesiov1alpha1.DSPipeline, req context.Context) error {
	r.Log.Info("Applying APIServer apiServerFields")

	apiServerFields := ApiServerFields{
		ServiceName:             dsp.Name,
		Name:                    dsp.Name,
		Namespace:               dsp.Namespace,
		Owner:                   dsp,
		ApiServerImage:          dsp.Spec.APIServer.Image,
		PipelineRuntime:         "tekton",
		StripEOF:                "true",
		ObjectStoreConfigSecure: "false",
		ArtifactEndpointScheme:  "http:/",
		ArtifactLogs:            "false",
		ArtifactTracking:        "true",

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
	if dsp.Spec.APIServer.ArtifactScriptConfigMap != (dspipelinesiov1alpha1.ArtifactScriptConfigMap{}) {
		apiServerFields.ArtifactScriptConfigMapName = dsp.Spec.APIServer.ArtifactScriptConfigMap.Name
		apiServerFields.ArtifactScriptConfigMapKey = dsp.Spec.APIServer.ArtifactScriptConfigMap.Key
	} else {
		apiServerFields.ArtifactScriptConfigMapName = defaultArtifactScriptConfigMap
		apiServerFields.ArtifactScriptConfigMapKey = defaultArtifactScriptConfigMapKey
	}

	if dsp.Spec.Database.CustomDB != (dspipelinesiov1alpha1.CustomDB{}) {
		customDB := dsp.Spec.Database.CustomDB
		apiServerFields.DBUser = customDB.Username
		apiServerFields.DBName = customDB.DBName
		apiServerFields.DBPasswordSecretKey = customDB.PasswordSecret.Key
		apiServerFields.DBPasswordSecret = customDB.PasswordSecret.Name
		apiServerFields.DBHost = customDB.Host
		apiServerFields.DBPort = customDB.Port
	} else if dsp.Spec.Database.MariaDB != (dspipelinesiov1alpha1.MariaDB{}) {
		mariaDB := dsp.Spec.Database.MariaDB
		apiServerFields.DBUser = mariaDB.Username
		apiServerFields.DBName = mariaDB.DBName
		apiServerFields.DBPasswordSecretKey = mariaDB.PasswordSecret.Key
		apiServerFields.DBPasswordSecret = mariaDB.PasswordSecret.Name
		apiServerFields.DBHost = defaultDBHost
		apiServerFields.DBPort = defaultDBHostPort
	} else {
		return fmt.Errorf("no Database specified for DS-Pipeline resource")
	}

	if dsp.Spec.Storage.CustomStorage != (dspipelinesiov1alpha1.CustomStorage{}) {
		storage := dsp.Spec.Storage.CustomStorage
		apiServerFields.ObjectStoreConfigBucketName = storage.Bucket
		apiServerFields.S3CredentialsSecretName = storage.SecretName
		apiServerFields.AccessKeySecretKey = storage.AccessKey
		apiServerFields.SecretKey = storage.SecretKey
		apiServerFields.MinioServiceServiceHost = storage.Host
		apiServerFields.MinioServiceServicePort = storage.Port
		apiServerFields.ArtifactBucket = storage.Bucket
		apiServerFields.ArtifactEndpoint = storage.Host + ":" + storage.Port
	} else if dsp.Spec.Storage.Minio != (dspipelinesiov1alpha1.Minio{}) {
		storage := dsp.Spec.Storage.Minio
		apiServerFields.ObjectStoreConfigBucketName = storage.Bucket
		apiServerFields.S3CredentialsSecretName = storage.SecretName
		apiServerFields.AccessKeySecretKey = storage.AccessKey
		apiServerFields.SecretKey = storage.SecretKey
		apiServerFields.MinioServiceServiceHost = defaultMinioHost
		apiServerFields.MinioServiceServicePort = defaultMinioPort
		apiServerFields.ArtifactBucket = storage.Bucket
		apiServerFields.ArtifactEndpoint = defaultMinioHost + ":" + defaultMinioPort
	} else {
		return fmt.Errorf("no S3 storage specified for DS-Pipeline resource")
	}

	for _, template := range templates {
		tmplManifest, err := config.Manifest(apiServerFields.Client, template, apiServerFields)
		if err != nil {
			return fmt.Errorf("error loading template yaml: %w", err)
		}
		tmplManifest, err = tmplManifest.Transform(
			mf.InjectOwner(dsp),
		)
		if err = tmplManifest.Apply(); err != nil {
			return err
		}
	}

	return nil
}
