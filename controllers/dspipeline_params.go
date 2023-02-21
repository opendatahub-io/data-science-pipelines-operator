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
	"encoding/base64"
	"fmt"
	"github.com/go-logr/logr"
	mf "github.com/manifestival/manifestival"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"math/rand"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	defaultArtifactScriptConfigMap    = "ds-pipeline-artifact-script"
	defaultArtifactScriptConfigMapKey = "artifact_script"
	defaultDSPServicePrefix           = "ds-pipeline"
)

type DSPipelineParams struct {
	Name                 string
	Namespace            string
	Owner                mf.Owner
	APIServer            dspipelinesiov1alpha1.APIServer
	APIServerServiceName string
	ScheduledWorkflow    dspipelinesiov1alpha1.ScheduledWorkflow
	ViewerCRD            dspipelinesiov1alpha1.ViewerCRD
	PersistenceAgent     dspipelinesiov1alpha1.PersistenceAgent
	MlPipelineUI         dspipelinesiov1alpha1.MlPipelineUI
	MariaDB              dspipelinesiov1alpha1.MariaDB
	Minio                dspipelinesiov1alpha1.Minio
	DBConnection
	ObjectStorageConnection
}

type DBConnection struct {
	Host              string
	Port              string
	Username          string
	DBName            string
	CredentialsSecret dspipelinesiov1alpha1.SecretKeyValue
	Password          string
}

type ObjectStorageConnection struct {
	Bucket            string
	CredentialsSecret dspipelinesiov1alpha1.S3CredentialSecret
	Secure            bool
	Host              string
	Port              string
	Scheme            string
	Endpoint          string // scheme://host:port
	AccessKeyID       string
	SecretAccessKey   string
}

func (p *DSPipelineParams) UsingExternalDB(dsp *dspipelinesiov1alpha1.DSPipeline) (bool, error) {
	ExternalDBIsNotEmpty := !reflect.DeepEqual(dsp.Spec.Database.ExternalDB, dspipelinesiov1alpha1.ExternalDB{})
	MariaDBIsNotEmpty := !reflect.DeepEqual(dsp.Spec.Database.MariaDB, dspipelinesiov1alpha1.MariaDB{})
	if ExternalDBIsNotEmpty {
		return true, nil
	} else if MariaDBIsNotEmpty {
		return false, nil
	}
	return false, fmt.Errorf("no Database specified for DS-Pipeline resource")
}

func (p *DSPipelineParams) UsingExternalStorage(dsp *dspipelinesiov1alpha1.DSPipeline) (bool, error) {
	ExternalStorageIsNotEmpty := !reflect.DeepEqual(dsp.Spec.ObjectStorage.ExternalStorage, dspipelinesiov1alpha1.ExternalStorage{})
	MinioIsNotEmpty := !reflect.DeepEqual(dsp.Spec.ObjectStorage.Minio, dspipelinesiov1alpha1.Minio{})
	if ExternalStorageIsNotEmpty {
		return true, nil
	} else if MinioIsNotEmpty {
		return false, nil
	}
	return false, fmt.Errorf("no Database specified for DS-Pipeline resource")
}

func passwordGen(n int) string {
	rand.Seed(time.Now().UnixNano())
	var chars = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// SetupDBParams Populates the DB connection Parameters.
// If an external secret is specified, SetupDBParams will retrieve DB credentials from it.
// If DSPO is managing a dynamically created secret, then SetupDBParams generates the creds.
func (p *DSPipelineParams) SetupDBParams(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline, client client.Client, log logr.Logger) error {
	//Set up DB Connection
	usingExternalDB, err := p.UsingExternalDB(dsp)
	if err != nil {
		return err
	}

	if usingExternalDB {
		// Assume validation for CR ensures these values exist
		p.DBConnection.Host = dsp.Spec.Database.ExternalDB.Host
		p.DBConnection.Port = dsp.Spec.Database.ExternalDB.Port
		p.DBConnection.Username = dsp.Spec.Database.ExternalDB.Username
		p.DBConnection.DBName = dsp.Spec.Database.ExternalDB.DBName
		p.DBConnection.CredentialsSecret = dsp.Spec.Database.ExternalDB.PasswordSecret
	} else {
		p.DBConnection.Host = fmt.Sprintf(
			"%s.%s.svc.cluster.local",
			defaultDBHostPrefix+"-"+p.Name,
			p.Namespace,
		)
		p.DBConnection.Port = defaultDBHostPort
		p.DBConnection.Username = defaultDBUser
		p.DBConnection.DBName = defaultDBName

		if dsp.Spec.Database.ExternalDB.Username != "" {
			p.DBConnection.Username = dsp.Spec.Database.ExternalDB.Username
		}
		if dsp.Spec.Database.ExternalDB.DBName != "" {
			p.DBConnection.DBName = dsp.Spec.Database.ExternalDB.DBName
		}
		mariaDBSecretSpecified := !reflect.DeepEqual(dsp.Spec.MariaDB.PasswordSecret, dspipelinesiov1alpha1.SecretKeyValue{})
		if mariaDBSecretSpecified {
			p.DBConnection.CredentialsSecret = dsp.Spec.MariaDB.PasswordSecret
		}
	}

	DBCredentialsNotSpecified := reflect.DeepEqual(p.DBConnection.CredentialsSecret, dspipelinesiov1alpha1.SecretKeyValue{})
	if DBCredentialsNotSpecified {
		// We assume validation ensures DB Credentials are specified for External DB
		// So this case is only possible if MariaDB deployment is specified, but no secret is provided.
		// In this case a custom secret will be created.
		p.DBConnection.CredentialsSecret = dspipelinesiov1alpha1.SecretKeyValue{
			Name: defaultDBHostPrefix + p.Name,
			Key:  defaultDBSecretKey,
		}
	}

	dbSecret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      p.DBConnection.CredentialsSecret.Name,
		Namespace: p.Namespace,
	}

	createNewSecret := false

	// Attempt to fetch the specified DB secret
	err = client.Get(ctx, namespacedName, dbSecret)
	if err != nil && apierrs.IsNotFound(err) {
		if DBCredentialsNotSpecified {
			generatedPass := passwordGen(12)
			p.DBConnection.Password = base64.StdEncoding.EncodeToString([]byte(generatedPass))
			createNewSecret = true
		} else {
			log.Error(err, fmt.Sprintf("DB secret %s was specified in CR but does not exist.",
				p.DBConnection.CredentialsSecret.Name))
			return err
		}
	} else if err != nil {
		log.Error(err, "Unable to fetch DB secret...")
		return err
	}

	// Password was dynamically generated, no need to retrieve it from fetched secret
	if createNewSecret {
		return nil
	}

	cs := p.DBConnection.CredentialsSecret
	p.DBConnection.Password = base64.StdEncoding.EncodeToString(dbSecret.Data[cs.Key])

	return nil
}

// SetupObjectParams Populates the Object Storage connection Parameters.
// If an external secret is specified, SetupObjectParams will retrieve storage credentials from it.
// If DSPO is managing a dynamically created secret, then SetupObjectParams generates the creds.
func (p *DSPipelineParams) SetupObjectParams(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline, client client.Client, log logr.Logger) error {

	usingExternalObjectStorage, err := p.UsingExternalStorage(dsp)
	if err != nil {
		return err
	}

	p.ObjectStorageConnection.Secure = DefaultObjectStoreConnectionSecure
	if usingExternalObjectStorage {
		// Assume validation for CR ensures these values exist
		p.ObjectStorageConnection.Bucket = dsp.Spec.ObjectStorage.ExternalStorage.Bucket
		p.ObjectStorageConnection.CredentialsSecret = dsp.Spec.ObjectStorage.ExternalStorage.S3CredentialSecret
		p.ObjectStorageConnection.Host = dsp.Spec.ObjectStorage.ExternalStorage.Host
		p.ObjectStorageConnection.Port = dsp.Spec.ObjectStorage.ExternalStorage.Port
		p.ObjectStorageConnection.Scheme = dsp.Spec.ObjectStorage.ExternalStorage.Scheme
	} else {
		p.ObjectStorageConnection.Bucket = dsp.Spec.ObjectStorage.Minio.Bucket
		p.ObjectStorageConnection.Host = fmt.Sprintf(
			"%s.%s.svc.cluster.local", defaultMinioHostPrefix+"-"+p.Name,
			p.Namespace,
		)
		p.ObjectStorageConnection.Port = defaultMinioPort
		p.ObjectStorageConnection.Scheme = defaultMinioScheme
		minioSecretSpecified := !reflect.DeepEqual(dsp.Spec.Minio.S3CredentialSecret, dspipelinesiov1alpha1.S3CredentialSecret{})
		if minioSecretSpecified {
			p.ObjectStorageConnection.CredentialsSecret = dsp.Spec.ObjectStorage.Minio.S3CredentialSecret
		}
	}

	p.ObjectStorageConnection.Endpoint = fmt.Sprintf(
		"%s://%s:%s",
		p.ObjectStorageConnection.Scheme,
		p.ObjectStorageConnection.Host,
		p.ObjectStorageConnection.Port,
	)

	storageCredentialsNotSpecified := reflect.DeepEqual(p.ObjectStorageConnection.CredentialsSecret, dspipelinesiov1alpha1.S3CredentialSecret{})
	if storageCredentialsNotSpecified {
		// We assume validation ensures Storage Credentials are specified for External Object Storage
		// So this case is only possible if Custom Object Storage deployment is enabled, but no secret is provided.
		// In this case a custom secret will be created.
		p.ObjectStorageConnection.CredentialsSecret = dspipelinesiov1alpha1.S3CredentialSecret{
			SecretName: defaultObjectStorageSecretName,
			AccessKey:  defaultObjectStorageAccessKey,
			SecretKey:  defaultObjectStorageSecretKey,
		}
	}

	storageSecret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      p.ObjectStorageConnection.CredentialsSecret.SecretName,
		Namespace: p.Namespace,
	}

	createNewSecret := false

	// Attempt to fetch the specified storage secret
	err = client.Get(ctx, namespacedName, storageSecret)
	if err != nil && apierrs.IsNotFound(err) {
		if storageCredentialsNotSpecified {
			generatedPass := passwordGen(16)
			p.ObjectStorageConnection.AccessKeyID = base64.StdEncoding.EncodeToString([]byte(generatedPass))
			generatedPass = passwordGen(24)
			p.ObjectStorageConnection.SecretAccessKey = base64.StdEncoding.EncodeToString([]byte(generatedPass))
			createNewSecret = true
		} else {
			log.Error(err, fmt.Sprintf("Storage secret %s was specified in CR but does not exist.",
				p.ObjectStorageConnection.CredentialsSecret.SecretName))
			return err
		}
	} else if err != nil {
		log.Error(err, "Unable to fetch Storage secret...")
		return err
	}

	// Password was dynamically generated, no need to retrieve it from fetched secret
	if createNewSecret {
		return nil
	}

	cs := p.ObjectStorageConnection.CredentialsSecret
	p.ObjectStorageConnection.AccessKeyID = base64.StdEncoding.EncodeToString(storageSecret.Data[cs.AccessKey])
	p.ObjectStorageConnection.SecretAccessKey = base64.StdEncoding.EncodeToString(storageSecret.Data[cs.SecretKey])

	return nil
}

func (p *DSPipelineParams) ExtractParams(ctx context.Context, dsp *dspipelinesiov1alpha1.DSPipeline, client client.Client, log logr.Logger) error {
	p.Name = dsp.Name
	p.Namespace = dsp.Namespace
	p.Owner = dsp
	p.APIServer = dsp.Spec.APIServer
	p.APIServerServiceName = fmt.Sprintf("%s-%s", defaultDSPServicePrefix, p.Name)
	p.ScheduledWorkflow = dsp.Spec.ScheduledWorkflow
	p.ViewerCRD = dsp.Spec.ViewerCRD
	p.PersistenceAgent = dsp.Spec.PersistenceAgent
	p.MlPipelineUI = dsp.Spec.MlPipelineUI
	p.MariaDB = dsp.Spec.MariaDB
	p.Minio = dsp.Spec.Minio

	err := p.SetupDBParams(ctx, dsp, client, log)
	if err != nil {
		return err
	}

	err = p.SetupObjectParams(ctx, dsp, client, log)
	if err != nil {
		return err
	}

	if dsp.Spec.APIServer.ArtifactScriptConfigMap == (dspipelinesiov1alpha1.ArtifactScriptConfigMap{}) {
		p.APIServer.ArtifactScriptConfigMap.Name = defaultArtifactScriptConfigMap
		p.APIServer.ArtifactScriptConfigMap.Key = defaultArtifactScriptConfigMapKey
	}

	return nil
}
