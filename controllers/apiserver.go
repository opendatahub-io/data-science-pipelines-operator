/*

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
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"math/rand"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

var apiServerTemplates = []string{
	"apiserver/artifact_script.yaml.tmpl",
	"apiserver/role_ds-pipeline.yaml.tmpl",
	"apiserver/role_pipeline-runner.yaml.tmpl",
	"apiserver/rolebinding_ds-pipeline.yaml.tmpl",
	"apiserver/rolebinding_pipeline-runner.yaml.tmpl",
	"apiserver/sa_ds-pipeline.yaml.tmpl",
	"apiserver/sa_pipeline-runner.yaml.tmpl",
	"apiserver/service.yaml.tmpl",
	"apiserver/deployment.yaml.tmpl",
}

const (
	defaultDBHostPrefix                = "mariadb"
	defaultDBHostPort                  = "3306"
	defaultDBUser                      = "mlpipeline"
	defaultDBName                      = "mlpipeline"
	defaultDBSecretKey                 = "password"
	defaultMinioHostPrefix             = "minio"
	defaultMinioPort                   = "9000"
	defaultObjectStorageAccessKey      = "accesskey"
	defaultObjectStorageSecretKey      = "secretkey"
	defaultMinioScheme                 = "http"
	DefaultObjectStoreConnectionSecure = false
	// This is hardcoded in kfp-tekton, apiserver will always use this hardcoded secret for tekton resources
	minioArtifactSecret     = "apiserver/mlpipeline-minio-artifact.yaml.tmpl"
	minioArtifactSecretName = "mlpipeline-minio-artifact"
)

func passwordGen(n int) string {
	rand.Seed(time.Now().UnixNano())
	var chars = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

//SetupDBConnection sets up an DB Connection
func (r *DSPipelineReconciler) SetupDBConnection(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline, params *DSPipelineParams) error {
	//Set up DB Connection
	usingExternalDB, err := params.UsingExternalDB(dsp)
	if err != nil {
		return err
	}

	if usingExternalDB {
		// Assume validation for CR ensures these values exist
		params.DBConnection.Host = dsp.Spec.Database.ExternalDB.Host
		params.DBConnection.Port = dsp.Spec.Database.ExternalDB.Port
		params.DBConnection.Username = dsp.Spec.Database.ExternalDB.Username
		params.DBConnection.DBName = dsp.Spec.Database.ExternalDB.DBName
		params.DBConnection.CredentialsSecret = dsp.Spec.Database.ExternalDB.PasswordSecret

	} else {
		params.DBConnection.Host = fmt.Sprintf(
			"%s.%s.svc.cluster.local", defaultDBHostPrefix+"-"+params.Name,
			params.Namespace,
		)
		params.DBConnection.Port = defaultDBHostPort
		params.DBConnection.Username = defaultDBUser
		params.DBConnection.DBName = defaultDBName

		if dsp.Spec.Database.ExternalDB.Username != "" {
			params.DBConnection.Username = dsp.Spec.Database.ExternalDB.Username
		}
		if dsp.Spec.Database.ExternalDB.DBName != "" {
			params.DBConnection.DBName = dsp.Spec.Database.ExternalDB.DBName
		}
		mariaDBSecretSpecified := !reflect.DeepEqual(dsp.Spec.MariaDB.PasswordSecret, dspipelinesiov1alpha1.SecretKeyValue{})
		if mariaDBSecretSpecified {
			params.DBConnection.CredentialsSecret = dsp.Spec.MariaDB.PasswordSecret
		}
	}

	DBCredentialsNotSpecified := reflect.DeepEqual(params.DBConnection.CredentialsSecret, dspipelinesiov1alpha1.SecretKeyValue{})
	if DBCredentialsNotSpecified {
		// We assume validation ensures DB Credentials are specified for External DB
		// So this case is only possible if MariaDB deployment is specified, but no secret is provided.
		// In this case a custom secret will be created.
		params.DBConnection.CredentialsSecret = dspipelinesiov1alpha1.SecretKeyValue{
			Name: defaultDBHostPrefix + params.Name,
			Key:  defaultDBSecretKey,
		}
		params.DBConnection.Password = passwordGen(12)
	} else {
		// Attempt to fetch the specified DB secret
		dbSecret := &v1.Secret{}
		namespacedName := types.NamespacedName{
			Name:      params.DBConnection.CredentialsSecret.Name,
			Namespace: params.Namespace,
		}
		err = r.Get(ctx, namespacedName, dbSecret)
		if err != nil && apierrs.IsNotFound(err) {
			r.Log.Error(err, fmt.Sprintf("Specified DB secret %s does not exist.", params.DBConnection.CredentialsSecret.Name))
			return err
		} else if err != nil {
			r.Log.Error(err, "Unable to fetch DB secret...")
			return err
		}
		params.DBConnection.Password, err = GetSecretDataDecoded(dbSecret, params.DBConnection.CredentialsSecret.Key)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Encountered error when trying to fetch key %s in secret %s",
				params.DBConnection.CredentialsSecret.Key, params.DBConnection.CredentialsSecret.Name))
			return err
		}
	}
	return nil
}

//SetupObjectStorageConnection sets up an Object Storage Connection
func (r *DSPipelineReconciler) SetupObjectStorageConnection(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline, params *DSPipelineParams) error {

	usingExternalObjectStorage, err := params.UsingExternalStorage(dsp)
	if err != nil {
		return err
	}

	params.ObjectStorageConnection.Secure = DefaultObjectStoreConnectionSecure
	if usingExternalObjectStorage {
		// Assume validation for CR ensures these values exist
		params.ObjectStorageConnection.Bucket = dsp.Spec.ObjectStorage.ExternalStorage.Bucket
		params.ObjectStorageConnection.CredentialsSecret = dsp.Spec.ObjectStorage.ExternalStorage.S3CredentialSecret
		params.ObjectStorageConnection.Host = dsp.Spec.ObjectStorage.ExternalStorage.Host
		params.ObjectStorageConnection.Port = dsp.Spec.ObjectStorage.ExternalStorage.Port
		params.ObjectStorageConnection.Scheme = dsp.Spec.ObjectStorage.ExternalStorage.Scheme
	} else {
		params.ObjectStorageConnection.Bucket = dsp.Spec.ObjectStorage.Minio.Bucket

		params.ObjectStorageConnection.Host = fmt.Sprintf(
			"%s.%s.svc.cluster.local", defaultMinioHostPrefix+"-"+params.Name,
			params.Namespace,
		)
		params.ObjectStorageConnection.Port = defaultMinioPort
		params.ObjectStorageConnection.Scheme = defaultMinioScheme
		minioSecretSpecified := !reflect.DeepEqual(dsp.Spec.Minio.S3CredentialSecret, dspipelinesiov1alpha1.S3CredentialSecret{})
		if minioSecretSpecified {
			params.ObjectStorageConnection.CredentialsSecret = dsp.Spec.ObjectStorage.Minio.S3CredentialSecret
		}
	}

	params.ObjectStorageConnection.Endpoint = fmt.Sprintf(
		"%s://%s:%s",
		params.ObjectStorageConnection.Scheme,
		params.ObjectStorageConnection.Host,
		params.ObjectStorageConnection.Port,
	)

	// TODO: there is an issue with how we're handling ObjectStorageConnection.CredentialsSecret
	// We use this info to create the secret, but also fetch the custom secret, we cannot do both with the same info
	// Use "CredentialsSecret" to create, and create a local variable populated with custom DSP values
	// example customSecretName = dsp.spec.objectstorage.externalobjectstorage.secretkey etc...
	DBCredentialsNotSpecified := reflect.DeepEqual(params.ObjectStorageConnection.CredentialsSecret, dspipelinesiov1alpha1.S3CredentialSecret{})
	if DBCredentialsNotSpecified {
		// We assume validation ensures Storage Credentials are specified for External Object Storage
		// So this case is only possible if Minio deployment is specified, but no secret is provided.
		// In this case a custom secret will be created.
		params.ObjectStorageConnection.CredentialsSecret = dspipelinesiov1alpha1.S3CredentialSecret{
			SecretName: minioArtifactSecret,
			AccessKey:  defaultObjectStorageAccessKey,
			SecretKey:  defaultObjectStorageSecretKey,
		}
	} else {
		// Attempt to fetch the specified DB secret
		dbSecret := &v1.Secret{}
		namespacedName := types.NamespacedName{
			Name:      params.ObjectStorageConnection.CredentialsSecret.Name,
			Namespace: params.Namespace,
		}
		err = r.Get(ctx, namespacedName, dbSecret)
		if err != nil && apierrs.IsNotFound(err) {
			r.Log.Error(err, fmt.Sprintf("Specified DB secret %s does not exist.", params.ObjectStorageConnection.CredentialsSecret.Name))
			return err
		} else if err != nil {
			r.Log.Error(err, "Unable to fetch DB secret...")
			return err
		}
		params.ObjectStorageConnection.Password, err = GetSecretDataDecoded(dbSecret, params.ObjectStorageConnection.CredentialsSecret.Key)
		if err != nil {
			r.Log.Error(err, fmt.Sprintf("Encountered error when trying to fetch key %s in secret %s",
				params.ObjectStorageConnection.CredentialsSecret.Key, params.ObjectStorageConnection.CredentialsSecret.Name))
			return err
		}
	}
	return nil
}

func (r *DSPipelineReconciler) ReconcileAPIServer(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline, req ctrl.Request, params *DSPipelineParams) error {
	r.Log.Info("Applying APIServer Resources")

	err := r.SetupDBConnection(ctx, dsp, params)
	if err != nil {
		return err
	}

	err = r.SetupObjectStorageConnection(ctx, dsp, params)
	if err != nil {
		return err
	}

	for _, template := range apiServerTemplates {
		err := r.Apply(dsp, params, template)
		if err != nil {
			return err
		}
	}

	// Todo: We should switch to only using this secret for now otherwise the 2 storage secrets will go
	// out of sync if one is deleted because the password will be re-generated.
	// can also just check if the specified secret exists and copy those values onto the hardcoded secret.
	secret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      minioArtifactSecretName,
		Namespace: req.Namespace,
	}
	err = r.Get(ctx, namespacedName, secret)
	if err != nil && apierrs.IsNotFound(err) {
		r.Log.Info("Specified minio artifact secret not found, creating...")
		err := r.Apply(dsp, params, minioArtifactSecret)
		if err != nil {
			return err
		}
	} else if err != nil {
		r.Log.Error(err, "Unable to fetch minio artifact secret...")
		return err
	}

	r.Log.Info("Finished applying APIServer Resources")
	return nil
}
