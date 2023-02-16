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
	"encoding/base64"
	"fmt"
	mf "github.com/manifestival/manifestival"
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"reflect"
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

func (r *DSPipelineParams) UsingExternalDB(dsp *dspipelinesiov1alpha1.DSPipeline) (bool, error) {
	ExternalDBIsNotEmpty := !reflect.DeepEqual(dsp.Spec.Database.ExternalDB, dspipelinesiov1alpha1.ExternalDB{})
	MariaDBIsNotEmpty := !reflect.DeepEqual(dsp.Spec.Database.MariaDB, dspipelinesiov1alpha1.MariaDB{})
	if ExternalDBIsNotEmpty {
		return true, nil
	} else if MariaDBIsNotEmpty {
		return false, nil
	}
	return false, fmt.Errorf("no Database specified for DS-Pipeline resource")
}

func (r *DSPipelineParams) UsingExternalStorage(dsp *dspipelinesiov1alpha1.DSPipeline) (bool, error) {
	ExternalStorageIsNotEmpty := !reflect.DeepEqual(dsp.Spec.ObjectStorage.ExternalStorage, dspipelinesiov1alpha1.ExternalStorage{})
	MinioIsNotEmpty := !reflect.DeepEqual(dsp.Spec.ObjectStorage.Minio, dspipelinesiov1alpha1.Minio{})
	if ExternalStorageIsNotEmpty {
		return true, nil
	} else if MinioIsNotEmpty {
		return false, nil
	}
	return false, fmt.Errorf("no Database specified for DS-Pipeline resource")
}

func GetSecretDataDecoded(s *v1.Secret, key string) (string, error) {
	var secretData []byte
	_, err := base64.StdEncoding.Decode(secretData, s.Data[key])
	if err != nil {
		return "", err
	}
	return string(secretData), nil
}

func (r *DSPipelineParams) ExtractParams(dsp *dspipelinesiov1alpha1.DSPipeline) error {
	r.Name = dsp.Name
	r.Namespace = dsp.Namespace
	r.Owner = dsp
	r.APIServer = dsp.Spec.APIServer
	r.APIServerServiceName = fmt.Sprintf("%s-%s", defaultDSPServicePrefix, r.Name)
	r.ScheduledWorkflow = dsp.Spec.ScheduledWorkflow
	r.ViewerCRD = dsp.Spec.ViewerCRD
	r.PersistenceAgent = dsp.Spec.PersistenceAgent
	r.MlPipelineUI = dsp.Spec.MlPipelineUI
	r.MariaDB = dsp.Spec.MariaDB
	r.Minio = dsp.Spec.Minio

	if dsp.Spec.APIServer.ArtifactScriptConfigMap == (dspipelinesiov1alpha1.ArtifactScriptConfigMap{}) {
		r.APIServer.ArtifactScriptConfigMap.Name = defaultArtifactScriptConfigMap
		r.APIServer.ArtifactScriptConfigMap.Key = defaultArtifactScriptConfigMapKey
	}

	return nil
}
