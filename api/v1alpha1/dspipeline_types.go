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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Todo: Add validation

type DSPipelineSpec struct {
	// APIService specifies the Kubeflow Apiserver configurations
	APIServer         `json:"apiServer,omitempty" validation:"Required"`
	PersistenceAgent  `json:"persistenceAgent,omitempty"`
	ScheduledWorkflow `json:"scheduledWorkflow,omitempty"`
	ViewerCRD         `json:"viewerCRD,omitempty"`
	Database          `json:"database,omitempty"`
	ObjectStorage     `json:"objectStorage,omitempty"`
	MlPipelineUI      `json:"mlpipelineUI,omitempty"`
}

type APIServer struct {
	Deploy                           bool   `json:"deploy,omitempty"`
	Image                            string `json:"image,omitempty"`
	ApplyTektonCustomResource        bool   `json:"applyTektonCustomResource,omitempty"`
	ArchiveLogs                      bool   `json:"archiveLogs,omitempty"`
	ArtifactImage                    string `json:"artifactImage,omitempty"`
	CacheImage                       string `json:"cacheImage,omitempty"`
	MoveResultsImage                 string `json:"moveResultsImage,omitempty"`
	ArtifactScriptConfigMap          `json:"artifactScriptConfigMap,omitempty"`
	InjectDefaultScript              bool   `json:"injectDefaultScript,omitempty"`
	StripEOF                         bool   `json:"stripEOF,omitempty"`
	TerminateStatus                  string `json:"terminateStatus,omitempty"`
	TrackArtifacts                   bool   `json:"trackArtifacts,omitempty"`
	DBConfigConMaxLifetimeSec        int    `json:"dbConfigConMaxLifetimeSec,omitempty"`
	CollectMetrics                   bool   `json:"collectMetrics,omitempty"`
	AutoUpdatePipelineDefaultVersion bool   `json:"autoUpdatePipelineDefaultVersion,omitempty"`
	corev1.ResourceRequirements      `json:"resources,omitempty"`
}

type ArtifactScriptConfigMap struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type PersistenceAgent struct {
	Deploy                      bool   `json:"deploy,omitempty"`
	Image                       string `json:"image,omitempty"`
	NumWorkers                  int    `json:"numWorkers,omitempty"`
	corev1.ResourceRequirements `json:"resources,omitempty"`
}

type ScheduledWorkflow struct {
	Deploy                      bool   `json:"deploy,omitempty"`
	Image                       string `json:"image,omitempty"`
	CronScheduleTimezone        string `json:"cronScheduleTimezone,omitempty"`
	corev1.ResourceRequirements `json:"resources,omitempty"`
}

type ViewerCRD struct {
	Deploy                      bool   `json:"deploy,omitempty"`
	Image                       string `json:"image,omitempty"`
	MaxNumViewer                int    `json:"maxNumViewer,omitempty"`
	corev1.ResourceRequirements `json:"resources,omitempty"`
}

type MlPipelineUI struct {
	Deploy                      bool   `json:"deploy,omitempty"`
	Image                       string `json:"image,omitempty"`
	ConfigMapName               string `json:"configMap,omitempty"`
	corev1.ResourceRequirements `json:"resources,omitempty"`
}

type Database struct {
	MariaDB    `json:"mariaDB,omitempty"`
	ExternalDB `json:"externalDB,omitempty"`
}

type MariaDB struct {
	Deploy         bool                        `json:"deploy,omitempty"`
	Image          string                      `json:"image,omitempty"`
	Username       string                      `json:"username,omitempty"`
	PasswordSecret SecretKeyValue              `json:"passwordSecret,omitempty"`
	DBName         string                      `json:"pipelineDBName,omitempty"`
	PVCSize        resource.Quantity           `json:"pvcSize,omitempty"`
	Resources      corev1.ResourceRequirements `json:"resources,omitempty"`
}

type ExternalDB struct {
	Host           string         `json:"host,omitempty"`
	Port           string         `json:"port,omitempty"`
	Username       string         `json:"username,omitempty"`
	DBName         string         `json:"pipelineDBName,omitempty"`
	PasswordSecret SecretKeyValue `json:"passwordSecret,omitempty"`
}

type ObjectStorage struct {
	Minio           `json:"minio,omitempty"`
	ExternalStorage `json:"externalStorage,omitempty"`
}

type Minio struct {
	Deploy             bool   `json:"deploy,omitempty"`
	Image              string `json:"image,omitempty"`
	Bucket             string `json:"bucket,omitempty"`
	S3CredentialSecret `json:"s3CredentialsSecret,omitempty"`
	PVCSize            resource.Quantity           `json:"pvcSize,omitempty"`
	Resources          corev1.ResourceRequirements `json:"resources,omitempty"`
}

type ExternalStorage struct {
	Host               string `json:"host,omitempty"`
	Port               string `json:"port,omitempty"`
	Bucket             string `json:"bucket,omitempty"`
	Scheme             string `json:"scheme,omitempty"`
	S3CredentialSecret `json:"s3CredentialsSecret,omitempty"`
}

type S3CredentialSecret struct {
	SecretName string `json:"secretName,omitempty"`
	// The "Keys" in the k8sSecret key/value pairs. Not to be confused with the values.
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

type SecretKeyValue struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type DSPipelineStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

type DSPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DSPipelineSpec   `json:"spec,omitempty"`
	Status            DSPipelineStatus `json:"status,omitempty"`
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
