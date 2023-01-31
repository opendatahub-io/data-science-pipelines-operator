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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Todo: Add validation

type DSPipelineSpec struct {
	// APIService specifies the Kubeflow Apiserver configurations
	APIServer           `json:"apiServer,omitempty" validation:"Required"`
	PersistentAgent     `json:"persistentAgent,omitempty"`
	ScheduledWorkflow   `json:"scheduledWorkflow,omitempty"`
	ViewerCRD           `json:"viewerCRD,omitempty"`
	VisualizationServer `json:"visualizationServer,omitempty"`
	Database            `json:"database,omitempty"`
	Storage             `json:"storage,omitempty"`
	MlPipelineUI        `json:"mlpipelineUI,omitempty"`
}

type APIServer struct {
	Image                   string `json:"apiServerImage,omitempty"`
	ArtifactImage           string `json:"artifactImage,omitempty"`
	ArtifactScriptConfigMap `json:"artifactScriptConfigMap,omitempty"`
}

type ArtifactScriptConfigMap struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type PersistentAgent struct {
	Image                 string `json:"image,omitempty"`
	PipelineAPIServerName string `json:"pipelineAPIServerName,omitempty"`
}

type ScheduledWorkflow struct {
	Image string `json:"image,omitempty"`
}

type ViewerCRD struct {
	Image string `json:"image,omitempty"`
}

type VisualizationServer struct {
	Image string `json:"image,omitempty"`
}

type Database struct {
	Image    string `json:"image,omitempty"`
	MariaDB  `json:"mariaDB,omitempty"`
	CustomDB `json:"customDB,omitempty"`
}

type MariaDB struct {
	Image          string         `json:"image,omitempty"`
	Username       string         `json:"username,omitempty"`
	PasswordSecret SecretKeyValue `json:"passwordSecret,omitempty"`
	DBName         string         `json:"pipelineDBName,omitempty"`
}

type CustomDB struct {
	Host           string         `json:"host,omitempty"`
	Port           string         `json:"port,omitempty"`
	Username       string         `json:"username,omitempty"`
	DBName         string         `json:"pipelineDBName,omitempty"`
	PasswordSecret SecretKeyValue `json:"passwordSecret,omitempty"`
}

type Storage struct {
	Minio         `json:"minio,omitempty"`
	CustomStorage `json:"customStorage,omitempty"`
}

type Minio struct {
	Image              string `json:"image,omitempty"`
	Bucket             string `json:"bucket,omitempty"`
	s3CredentialSecret `json:"s3CredentialsSecret,omitempty"`
}

type CustomStorage struct {
	Host               string `json:"host,omitempty"`
	Port               string `json:"port,omitempty"`
	Bucket             string `json:"bucket,omitempty"`
	s3CredentialSecret `json:"s3CredentialsSecret,omitempty"`
}

type MlPipelineUI struct {
	Image         string `json:"image,omitempty"`
	ConfigMapName string `json:"configMap,omitempty"`
}

type DSPipelineStatus struct {
}

type s3CredentialSecret struct {
	SecretName string `json:"secretName,omitempty"`
	AccessKey  string `json:"accessKey,omitempty"`
	SecretKey  string `json:"secretKey,omitempty"`
}

type SecretKeyValue struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

type DSPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DSPipelineSpec   `json:"spec,omitempty"`
	Status DSPipelineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type DSPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DSPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DSPipeline{}, &DSPipelineList{})
}
