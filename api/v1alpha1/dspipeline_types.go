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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DSPASpec struct {
	// APIService specifies the Kubeflow Apiserver configurations
	// +kubebuilder:default:={deploy: true}
	*APIServer `json:"apiServer,omitempty"`
	// +kubebuilder:default:={deploy: true}
	*PersistenceAgent `json:"persistenceAgent,omitempty"`
	// +kubebuilder:default:={deploy: true}
	*ScheduledWorkflow `json:"scheduledWorkflow,omitempty"`
	// +kubebuilder:default:={mariaDB: {deploy: true}}
	*Database `json:"database,omitempty"`
	// +kubebuilder:validation:Optional
	*MlPipelineUI `json:"mlpipelineUI"`
	// +kubebuilder:validation:Required
	*ObjectStorage `json:"objectStorage"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:={deploy: false}
	*MLMD `json:"mlmd"`
}

type APIServer struct {
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool   `json:"deploy"`
	Image  string `json:"image,omitempty"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	ApplyTektonCustomResource bool `json:"applyTektonCustomResource"`
	// +kubebuilder:default:=false
	// +kubebuilder:validation:Optional
	ArchiveLogs              bool   `json:"archiveLogs"`
	ArtifactImage            string `json:"artifactImage,omitempty"`
	CacheImage               string `json:"cacheImage,omitempty"`
	MoveResultsImage         string `json:"moveResultsImage,omitempty"`
	*ArtifactScriptConfigMap `json:"artifactScriptConfigMap,omitempty"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	InjectDefaultScript bool `json:"injectDefaultScript"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	StripEOF bool `json:"stripEOF"`
	// +kubebuilder:validation:Enum=Cancelled;StoppedRunFinally;CancelledRunFinally
	// +kubebuilder:default:=Cancelled
	TerminateStatus string `json:"terminateStatus,omitempty"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	TrackArtifacts bool `json:"trackArtifacts"`
	// +kubebuilder:default:=120
	DBConfigConMaxLifetimeSec int `json:"dbConfigConMaxLifetimeSec,omitempty"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	CollectMetrics bool `json:"collectMetrics"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	EnableRoute bool `json:"enableOauth"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	EnableSamplePipeline bool `json:"enableSamplePipeline"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	AutoUpdatePipelineDefaultVersion bool                  `json:"autoUpdatePipelineDefaultVersion"`
	Resources                        *ResourceRequirements `json:"resources,omitempty"`
}

type ArtifactScriptConfigMap struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type PersistenceAgent struct {
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool   `json:"deploy"`
	Image  string `json:"image,omitempty"`
	// +kubebuilder:default:=2
	NumWorkers int                   `json:"numWorkers,omitempty"`
	Resources  *ResourceRequirements `json:"resources,omitempty"`
}

type ScheduledWorkflow struct {
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool   `json:"deploy"`
	Image  string `json:"image,omitempty"`
	// +kubebuilder:default:=UTC
	CronScheduleTimezone string                `json:"cronScheduleTimezone,omitempty"`
	Resources            *ResourceRequirements `json:"resources,omitempty"`
}

type MlPipelineUI struct {
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy        bool                  `json:"deploy"`
	ConfigMapName string                `json:"configMap,omitempty"`
	Resources     *ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Required
	Image string `json:"image"`
}

type Database struct {
	*MariaDB    `json:"mariaDB,omitempty"`
	*ExternalDB `json:"externalDB,omitempty"`
}

type MariaDB struct {
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool   `json:"deploy"`
	Image  string `json:"image,omitempty"`
	// +kubebuilder:default:=mlpipeline
	Username       string          `json:"username,omitempty"`
	PasswordSecret *SecretKeyValue `json:"passwordSecret,omitempty"`
	// +kubebuilder:default:=mlpipeline
	DBName string `json:"pipelineDBName,omitempty"`
	// +kubebuilder:default:="10Gi"
	PVCSize   resource.Quantity     `json:"pvcSize,omitempty"`
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

type ExternalDB struct {
	// +kubebuilder:validation:Required
	Host           string          `json:"host"`
	Port           string          `json:"port"`
	Username       string          `json:"username"`
	DBName         string          `json:"pipelineDBName"`
	PasswordSecret *SecretKeyValue `json:"passwordSecret"`
}

type ObjectStorage struct {
	*Minio           `json:"minio,omitempty"`
	*ExternalStorage `json:"externalStorage,omitempty"`
}

type Minio struct {
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool `json:"deploy"`
	// +kubebuilder:default:=mlpipeline
	Bucket              string `json:"bucket,omitempty"`
	*S3CredentialSecret `json:"s3CredentialsSecret,omitempty"`
	// +kubebuilder:default:="10Gi"
	PVCSize   resource.Quantity     `json:"pvcSize,omitempty"`
	Resources *ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Required
	Image string `json:"image"`
}

type MLMD struct {
	// +kubebuilder:default:=false
	// +kubebuilder:validation:Optional
	Deploy  bool `json:"deploy"`
	*Envoy  `json:"envoy,omitempty"`
	*GRPC   `json:"grpc,omitempty"`
	*Writer `json:"writer,omitempty"`
}

type Envoy struct {
	Resources *ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Required
	Image string `json:"image"`
}

type GRPC struct {
	Resources *ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Required
	Image string `json:"image"`
	// +kubebuilder:validation:Optional
	Port string `json:"port"`
}

type Writer struct {
	Resources *ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Required
	Image string `json:"image"`
}

// ResourceRequirements structures compute resource requirements.
// Replaces ResourceRequirements from corev1 which also includes optional storage field.
// We handle storage field separately, and should not include it as a subfield for Resources.
type ResourceRequirements struct {
	Limits   *Resources `json:"limits,omitempty"`
	Requests *Resources `json:"requests,omitempty"`
}

type Resources struct {
	CPU    resource.Quantity `json:"cpu,omitempty"`
	Memory resource.Quantity `json:"memory,omitempty"`
}

type ExternalStorage struct {
	// +kubebuilder:validation:Required
	Host                string `json:"host"`
	Bucket              string `json:"bucket"`
	Scheme              string `json:"scheme"`
	*S3CredentialSecret `json:"s3CredentialsSecret"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Secure bool `json:"secure"`
	// +kubebuilder:validation:Optional
	Port string `json:"port"`
}

type S3CredentialSecret struct {
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`
	// The "Keys" in the k8sSecret key/value pairs. Not to be confused with the values.
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type SecretKeyValue struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	Key  string `json:"key"`
}

type DSPAStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=dspa

type DataSciencePipelinesApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DSPASpec   `json:"spec,omitempty"`
	Status            DSPAStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type DataSciencePipelinesApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataSciencePipelinesApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataSciencePipelinesApplication{}, &DataSciencePipelinesApplicationList{})
}
