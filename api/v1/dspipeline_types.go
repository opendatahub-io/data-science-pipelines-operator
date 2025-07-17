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

package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DSPASpec struct {
	// DS Pipelines API Server configuration.
	// +kubebuilder:default:={deploy: true}
	*APIServer `json:"apiServer,omitempty"`
	// DS Pipelines PersistenceAgent configuration.
	// +kubebuilder:default:={deploy: true}
	*PersistenceAgent `json:"persistenceAgent,omitempty"`
	// DS Pipelines Scheduled Workflow configuration.
	// +kubebuilder:default:={deploy: true}
	*ScheduledWorkflow `json:"scheduledWorkflow,omitempty"`
	// Database specifies database configurations, used for DS Pipelines metadata tracking. Specify either the default MariaDB deployment, or configure your own External SQL DB.
	// +kubebuilder:default:={mariaDB: {deploy: true}}
	*Database `json:"database,omitempty"`
	// Deploy the KFP UI with DS Pipelines UI. This feature is unsupported, and primarily used for exploration, testing, and development purposes.
	// +kubebuilder:validation:Optional
	*MlPipelineUI `json:"mlpipelineUI"`
	// ObjectStorage specifies Object Store configurations, used for DS Pipelines artifact passing and storage. Specify either the your own External Storage (e.g. AWS S3), or use the default Minio deployment (unsupported, primarily for development, and testing) .
	// +kubebuilder:validation:Required
	*ObjectStorage `json:"objectStorage"`
	*MLMD          `json:"mlmd,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="v2"
	DSPVersion string `json:"dspVersion,omitempty"`

	// PodToPodTLS Set to "true" or "false" to enable or disable TLS communication between DSPA components (pods). Defaults to "true" to enable TLS between all pods. Only supported in DSP V2 on OpenShift.
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	PodToPodTLS *bool `json:"podToPodTLS"`

	// WorkflowController is an argo-specific component that manages a DSPA's Workflow objects and handles the orchestration of them with the central Argo server
	// +kubebuilder:validation:Optional
	*WorkflowController `json:"workflowController,omitempty"`
}

// +kubebuilder:validation:Pattern=`^(Managed|Removed)$`
type ManagedPipelineState string

type ManagedPipelineOptions struct {
	// Set to one of the following values:
	//
	// - "Managed" : This pipeline is automatically imported.
	// - "Removed" : This pipeline is not automatically imported. If previously set to "Managed", setting to "Removed" does not remove existing managed pipelines but does prevent future updates from being imported.
	//
	// +kubebuilder:validation:Enum=Managed;Removed
	// +kubebuilder:default=Removed
	// +kubebuilder:validation:Optional
	State ManagedPipelineState `json:"state,omitempty"`
}

type ManagedPipelinesSpec struct {
	// Configures whether to automatically import the technical preview of the InstructLab pipeline.
	// You must enable the trainingoperator component to run the InstructLab pipeline.
	// +kubebuilder:validation:Optional
	InstructLab *ManagedPipelineOptions `json:"instructLab,omitempty"`
}

type APIServer struct {
	// Enable DS Pipelines Operator management of DSP API Server. Setting Deploy to false disables operator reconciliation. Default: true
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool `json:"deploy"`
	// Specify a custom image for DSP API Server.
	Image string `json:"image,omitempty"`
	// Create an Openshift Route for this DSP API Server. Default: true
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	EnableRoute bool `json:"enableOauth"`
	// Include the Iris sample pipeline with the deployment of this DSP API Server. Default: true
	// +kubebuilder:default:=false
	// +kubebuilder:validation:Optional
	EnableSamplePipeline bool `json:"enableSamplePipeline"`
	// Launcher/Executor image used during pipeline execution.
	ArgoLauncherImage string `json:"argoLauncherImage,omitempty"`
	// Driver image used during pipeline execution.
	ArgoDriverImage string `json:"argoDriverImage,omitempty"`
	// Generic runtime image used for building managed pipelines during
	// api server init, and for basic runtime operations.
	RuntimeGenericImage string `json:"runtimeGenericImage,omitempty"`
	// Toolbox image used for basic container spec runtime operations
	// in managed pipelines.
	ToolboxImage string `json:"toolboxImage,omitempty"`
	// RhelAI image used for ilab tasks in managed pipelines.
	RHELAIImage string `json:"rhelAIImage,omitempty"`
	// Enable various managed pipelines on this DSP API server.
	ManagedPipelines *ManagedPipelinesSpec `json:"managedPipelines,omitempty"`
	// Specify custom Pod resource requirements for this component.
	Resources *ResourceRequirements `json:"resources,omitempty"`
	// Specify init container resource requirements. The init container
	// is used to build managed-pipelines and store them in a shared volume.
	InitResources *ResourceRequirements `json:"initResources,omitempty"`

	// If the Object store/DB is behind a TLS secured connection that is
	// unrecognized by the host OpenShift/K8s cluster, then you can
	// provide a PEM formatted CA bundle to be injected into the DSP
	// server pod to trust this connection. CA Bundle should be provided
	// as values within configmaps, mapped to keys.
	CABundle *CABundle `json:"cABundle,omitempty"`

	// CustomServerConfig is a custom config file that you can provide
	// for the api server to use instead.
	CustomServerConfig *ScriptConfigMap `json:"customServerConfigMap,omitempty"`

	// When specified, the `data` contents of the `kfp-launcher` ConfigMap that DSPO writes
	// will be fully replaced with the `data` contents of the ConfigMap specified here.
	// This allows the user to fully replace the `data` contents of the kfp-launcher ConfigMap.
	// The `kfp-launcher` component requires a ConfigMap to exist in the namespace
	// where it runs (i.e. the namespace where pipelines run). This ConfigMap contains
	// object storage configuration, as well as pipeline root (object store root path
	// where artifacts will be uploaded) configuration. Currently this ConfigMap *must*
	// be named "kfp-launcher". We currently deploy a default copy of the kfp-launcher
	// ConfigMap via DSPO, but a user may want to provide their own ConfigMap configuration,
	// so that they can specify multiple object storage sources and paths.
	// +kubebuilder:validation:Optional
	CustomKfpLauncherConfigMap string `json:"customKfpLauncherConfigMap,omitempty"`

	// This is the path where the ca bundle will be mounted in the
	// pipeline server and user executor pods
	// +kubebuilder:validation:Optional
	CABundleFileMountPath string `json:"caBundleFileMountPath"`
	// This is the filename of the ca bundle that will be created in the
	// pipeline server and user executor pods
	// +kubebuilder:validation:Optional
	CABundleFileName string `json:"caBundleFileName"`

	// The expiry time (seconds) for artifact download links when
	// querying the dsp server via /apis/v2beta1/artifacts/{id}?share_url=true
	// Default: 60
	// +kubebuilder:default:=60
	// +kubebuilder:validation:Optional
	ArtifactSignedURLExpirySeconds *int `json:"artifactSignedURLExpirySeconds"`

	// The storage for pipeline definitions (pipelines and pipeline versions). It can be
	// either 'database' or 'kubernetes' (Pipeline and PipelineVersion kinds). Defaults to 'database'.
	// +kubebuilder:default:=database
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=database;kubernetes
	PipelineStore string `json:"pipelineStore,omitempty"`

	// Allows/disallows caching in the DSP API server.
	// When true, cache options are permitted for Pipelines and Task configurations made via the KFP SDK at compile time.
	// When false, caching is always disabled and overrides the Pipelines and Task configurations.
	// Default: true
	// +kubebuilder:default:=true
	CacheEnabled *bool `json:"cacheEnabled,omitempty"`
}

type CABundle struct {
	// +kubebuilder:validation:Required
	ConfigMapName string `json:"configMapName"`
	// Key should map to a CA bundle. The key is also used to name
	// the CA bundle file (e.g. ca-bundle.crt)
	// +kubebuilder:validation:Required
	ConfigMapKey string `json:"configMapKey"`
}

type ScriptConfigMap struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type PersistenceAgent struct {
	// Enable DS Pipelines Operator management of Persisence Agent. Setting Deploy to false disables operator reconciliation. Default: true
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool `json:"deploy"`
	// Specify a custom image for DSP PersistenceAgent.
	Image string `json:"image,omitempty"`
	// Number of worker for Persistence Agent sync job. Default: 2
	// +kubebuilder:default:=2
	NumWorkers int `json:"numWorkers,omitempty"`
	// Specify custom Pod resource requirements for this component.
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

type ScheduledWorkflow struct {
	// Enable DS Pipelines Operator management of ScheduledWorkflow. Setting Deploy to false disables operator reconciliation. Default: true
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool `json:"deploy"`
	// Specify a custom image for DSP ScheduledWorkflow controller.
	Image string `json:"image,omitempty"`
	// Specify the Cron timezone used for ScheduledWorkflow PipelineRuns. Default: UTC
	// +kubebuilder:default:=UTC
	CronScheduleTimezone string `json:"cronScheduleTimezone,omitempty"`
	// Specify custom Pod resource requirements for this component.
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

type MlPipelineUI struct {
	// Enable DS Pipelines Operator management of KFP UI. Setting Deploy to false disables operator reconciliation. Default: true
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy        bool   `json:"deploy"`
	ConfigMapName string `json:"configMap,omitempty"`
	// Specify custom Pod resource requirements for this component.
	Resources *ResourceRequirements `json:"resources,omitempty"`
	// Specify a custom image for KFP UI pod.
	// +kubebuilder:validation:Required
	Image string `json:"image"`
}

type Database struct {
	*MariaDB    `json:"mariaDB,omitempty"`
	*ExternalDB `json:"externalDB,omitempty"`

	// +kubebuilder:validation:Optional
	// CustomExtraParams allow users to further customize the sql dsn parameters used by the Pipeline Server
	// when opening a connection with the Database.
	// ref: https://github.com/go-sql-driver/mysql?tab=readme-ov-file#dsn-data-source-name
	//
	// Value must be a JSON string. For example, to disable tls for Pipeline Server DB connection
	// the user can provide a string: {"tls":"true"}
	//
	// If updating post DSPA deployment, then a manual restart of the pipeline server pod will be required
	// so the new configmap may be consumed.
	CustomExtraParams *string `json:"customExtraParams,omitempty"`

	// Default: false
	// +kubebuilder:default:=false
	// +kubebuilder:validation:Optional
	DisableHealthCheck bool `json:"disableHealthCheck"`
}

type MariaDB struct {
	// Enable DS Pipelines Operator management of MariaDB. Setting Deploy to false disables operator reconciliation. Default: true
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool `json:"deploy"`
	// Specify a custom image for DSP MariaDB pod.
	Image string `json:"image,omitempty"`
	// The MariadB username that will be created. Should match `^[a-zA-Z0-9_]+`. Default: mlpipeline
	// +kubebuilder:default:=mlpipeline
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9_]+$`
	Username       string          `json:"username,omitempty"`
	PasswordSecret *SecretKeyValue `json:"passwordSecret,omitempty"`
	// +kubebuilder:default:=mlpipeline
	// The database name that will be created. Should match `^[a-zA-Z0-9_]+`. // Default: mlpipeline
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9_]+$`
	DBName string `json:"pipelineDBName,omitempty"`
	// Customize the size of the PVC created for the default MariaDB instance. Default: 10Gi
	// +kubebuilder:default:="10Gi"
	PVCSize resource.Quantity `json:"pvcSize,omitempty"`
	// Volume Mode Filesystem storageClass to use for PVC creation
	// +kubebuilder:validation:Optional
	StorageClassName string `json:"storageClassName,omitempty"`
	// Specify custom Pod resource requirements for this component.
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
	// Enable DS Pipelines Operator management of Minio. Setting Deploy to false disables operator reconciliation.
	*Minio           `json:"minio,omitempty"`
	*ExternalStorage `json:"externalStorage,omitempty"`
	// Default: false
	// +kubebuilder:default:=false
	// +kubebuilder:validation:Optional
	DisableHealthCheck bool `json:"disableHealthCheck"`
	// Enable an external route so the object storage is reachable from outside the cluster. Default: false
	// +kubebuilder:default:=false
	// +kubebuilder:validation:Optional
	EnableExternalRoute bool `json:"enableExternalRoute"`
}

type Minio struct {
	// Enable DS Pipelines Operator management of Minio. Setting Deploy to false disables operator reconciliation. Default: true
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy bool `json:"deploy"`
	// Provide the Bucket name that will be used to store artifacts in S3. If provided bucket does not exist, DSP Apiserver will attempt to create it. As such the credentials provided should have sufficient permissions to do create buckets. Default: mlpipeline
	// +kubebuilder:default:=mlpipeline
	Bucket string `json:"bucket,omitempty"`
	// Credentials for the S3 user (e.g. IAM user cred stored in a k8s secret.). Note that the S3 user should have the permissions to create a bucket if the provided bucket does not exist.
	*S3CredentialSecret `json:"s3CredentialsSecret,omitempty"`
	// Customize the size of the PVC created for the Minio instance. Default: 10Gi
	// +kubebuilder:default:="10Gi"
	PVCSize resource.Quantity `json:"pvcSize,omitempty"`
	// Volume Mode Filesystem storageClass to use for PVC creation
	// +kubebuilder:validation:Optional
	StorageClassName string `json:"storageClassName,omitempty"`
	// Specify custom Pod resource requirements for this component.
	Resources *ResourceRequirements `json:"resources,omitempty"`
	// Specify a custom image for Minio pod.
	// +kubebuilder:validation:Required
	Image string `json:"image"`
}

type MLMD struct {
	// Enable DS Pipelines Operator management of MLMD. Setting Deploy to false disables operator reconciliation. Default: true
	// +kubebuilder:default:=false
	// +kubebuilder:validation:Optional
	Deploy bool `json:"deploy"`
	*Envoy `json:"envoy,omitempty"`
	*GRPC  `json:"grpc,omitempty"`
}

type Envoy struct {
	Resources *ResourceRequirements `json:"resources,omitempty"`
	Image     string                `json:"image,omitempty"`
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	DeployRoute bool `json:"deployRoute"`
}

type GRPC struct {
	Resources *ResourceRequirements `json:"resources,omitempty"`
	Image     string                `json:"image,omitempty"`
	// +kubebuilder:validation:Optional
	Port string `json:"port"`
}

type Writer struct {
	Resources *ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:validation:Required
	Image string `json:"image"`
}

type WorkflowController struct {
	// +kubebuilder:default:=true
	// +kubebuilder:validation:Optional
	Deploy        bool   `json:"deploy"`
	Image         string `json:"image,omitempty"`
	ArgoExecImage string `json:"argoExecImage,omitempty"`
	CustomConfig  string `json:"customConfig,omitempty"`
	// Specify custom Pod resource requirements for this component.
	Resources *ResourceRequirements `json:"resources,omitempty"`
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
	Host   string `json:"host"`
	Bucket string `json:"bucket"`
	Scheme string `json:"scheme"`
	// +kubebuilder:validation:Optional
	Region string `json:"region"`
	// Subpath where objects should be stored for this DSPA
	// +kubebuilder:validation:Optional
	BasePath            string `json:"basePath"`
	*S3CredentialSecret `json:"s3CredentialsSecret"`
	// +kubebuilder:validation:Optional
	Secure *bool `json:"secure"`
	// +kubebuilder:validation:Optional
	Port string `json:"port"`
}

type S3CredentialSecret struct {
	// +kubebuilder:validation:Required
	// The name of the Secret where the AccessKey and SecretKey are defined.
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
	// +kubebuilder:validation:Optional
	Components ComponentStatus    `json:"components,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type ComponentStatus struct {
	// +kubebuilder:validation:Optional
	MLMDProxy ComponentDetailStatus `json:"mlmdProxy,omitempty"`
	APIServer ComponentDetailStatus `json:"apiServer,omitempty"`
}

type ComponentDetailStatus struct {
	Url string `json:"url,omitempty"`
	// +kubebuilder:validation:Optional
	ExternalUrl string `json:"externalUrl,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=dspa
// +kubebuilder:storageversion

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
