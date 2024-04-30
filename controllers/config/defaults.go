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

package config

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	DefaultImageValue = "MustSetInConfig"

	CustomCABundleRootMountPath = "/dsp-custom-certs"

	// GlobalODHCaBundleConfigMapName key and label values  are a contract with
	// ODH Platform https://github.com/opendatahub-io/architecture-decision-records/pull/28
	GlobalODHCaBundleConfigMapName = "odh-trusted-ca-bundle"
	// GlobalODHCaBundleConfigMapSystemBundleKey is the key that is added by network operator
	// https://docs.openshift.com/container-platform/4.15/networking/configuring-a-custom-pki.html#certificate-injection-using-operators_configuring-a-custom-pki
	GlobalODHCaBundleConfigMapSystemBundleKey = "ca-bundle.crt"

	CustomDSPTrustedCAConfigMapNamePrefix = "dsp-trusted-ca"
	CustomDSPTrustedCAConfigMapKey        = "dsp-ca.crt"

	DefaultSystemSSLCertFile     = "SSL_CERT_FILE"
	DefaultSystemSSLCertFilePath = "/etc/pki/tls/certs/ca-bundle.crt" // Fedora/RHEL 6

	MLPipelineUIConfigMapPrefix       = "ds-pipeline-ui-configmap-"
	ArtifactScriptConfigMapNamePrefix = "ds-pipeline-artifact-script-"
	ArtifactScriptConfigMapKey        = "artifact_script"

	CustomServerConfigMapNamePrefix = "ds-pipeline-server-config-"
	CustomServerConfigMapNameKey    = "config.json"
	DSPServicePrefix                = "ds-pipeline"

	DefaultDBSecretNamePrefix = "ds-pipeline-db-"
	DefaultDBSecretKey        = "password"
	GeneratedDBPasswordLength = 12

	MariaDBName        = "mlpipeline"
	MariaDBHostPrefix  = "mariadb"
	MariaDBHostPort    = "3306"
	MariaDBUser        = "mlpipeline"
	MariaDBNamePVCSize = "10Gi"

	MinioHostPrefix    = "minio"
	MinioPort          = "9000"
	MinioScheme        = "http"
	MinioDefaultBucket = "mlpipeline"
	MinioPVCSize       = "10Gi"

	DefaultObjectStorageSecretNamePrefix  = "ds-pipeline-s3-"
	DefaultObjectStorageAccessKey         = "accesskey"
	DefaultObjectStorageSecretKey         = "secretkey"
	GeneratedObjectStorageAccessKeyLength = 16
	GeneratedObjectStorageSecretKeyLength = 24

	MlmdGrpcPort = "8080"
)

// DSPO Config File Paths
const (
	APIServerImagePath                       = "Images.ApiServer"
	APIServerArtifactImagePath               = "Images.Artifact"
	PersistenceAgentImagePath                = "Images.PersistentAgent"
	ScheduledWorkflowImagePath               = "Images.ScheduledWorkflow"
	APIServerCacheImagePath                  = "Images.Cache"
	APIServerMoveResultsImagePath            = "Images.MoveResultsImage"
	MariaDBImagePath                         = "Images.MariaDB"
	OAuthProxyImagePath                      = "Images.OAuthProxy"
	MlmdEnvoyImagePath                       = "Images.MlmdEnvoy"
	MlmdGRPCImagePath                        = "Images.MlmdGRPC"
	MlmdWriterImagePath                      = "Images.MlmdWriter"
	ObjStoreConnectionTimeoutConfigName      = "DSPO.HealthCheck.ObjectStore.ConnectionTimeout"
	DBConnectionTimeoutConfigName            = "DSPO.HealthCheck.Database.ConnectionTimeout"
	RequeueTimeConfigName                    = "DSPO.RequeueTime"
	ApiServerIncludeOwnerReferenceConfigName = "DSPO.ApiServer.IncludeOwnerReference"
)

// DSPV2-Argo Image Paths
const (
	APIServerImagePathV2Argo             = "ImagesV2.Argo.ApiServer"
	APIServerArtifactImagePathV2Argo     = "ImagesV2.Argo.Artifact"
	APIServerCacheImagePathV2Argo        = "ImagesV2.Argo.Cache"
	APIServerMoveResultsImagePathV2Argo  = "ImagesV2.Argo.MoveResultsImage"
	APIServerArgoLauncherImagePathV2Argo = "ImagesV2.Argo.ArgoLauncherImage"
	APIServerArgoDriverImagePathV2Argo   = "ImagesV2.Argo.ArgoDriverImage"
	PersistenceAgentImagePathV2Argo      = "ImagesV2.Argo.PersistentAgent"
	ScheduledWorkflowImagePathV2Argo     = "ImagesV2.Argo.ScheduledWorkflow"
	MlmdEnvoyImagePathV2Argo             = "ImagesV2.Argo.MlmdEnvoy"
	MlmdGRPCImagePathV2Argo              = "ImagesV2.Argo.MlmdGRPC"
	ArgoWorkflowControllerImagePath      = "ImagesV2.Argo.WorkflowController"
	ArgoExecImagePath                    = "ImagesV2.Argo.ArgoExecImage"
)

// DSPV2-Tekton Image Paths
// Note: These won't exist in config but aren't used, adding in case of future support
// TODO: remove
const (
	APIServerImagePathV2Tekton            = "ImagesV2.Tekton.ApiServer"
	APIServerArtifactImagePathV2Tekton    = "ImagesV2.Tekton.Artifact"
	APIServerCacheImagePathV2Tekton       = "ImagesV2.Tekton.Cache"
	APIServerMoveResultsImagePathV2Tekton = "ImagesV2.Tekton.MoveResultsImage"
	PersistenceAgentImagePathV2Tekton     = "ImagesV2.Tekton.PersistentAgent"
	ScheduledWorkflowImagePathV2Tekton    = "ImagesV2.Tekton.ScheduledWorkflow"
	MlmdEnvoyImagePathV2Tekton            = "ImagesV2.Tekton.MlmdEnvoy"
	MlmdGRPCImagePathV2Tekton             = "ImagesV2.Tekton.MlmdGRPC"
)

// DSPA Status Condition Types
const (
	DatabaseAvailable      = "DatabaseAvailable"
	ObjectStoreAvailable   = "ObjectStoreAvailable"
	APIServerReady         = "APIServerReady"
	PersistenceAgentReady  = "PersistenceAgentReady"
	ScheduledWorkflowReady = "ScheduledWorkflowReady"
	CrReady                = "Ready"
)

// DSPA Ready Status Condition Reasons
// As per k8s api convention: Reason is intended
// to be used in concise output, such as one-line
// kubectl get output, and in summarizing
// occurrences of causes
const (
	MinimumReplicasAvailable    = "MinimumReplicasAvailable"
	FailingToDeploy             = "FailingToDeploy"
	Deploying                   = "Deploying"
	ComponentDeploymentNotFound = "ComponentDeploymentNotFound"
)

// Any required Configmap paths can be added here,
// they will be automatically included for required
// validation check
var requiredFields = []string{
	APIServerImagePath,
	APIServerArtifactImagePath,
	PersistenceAgentImagePath,
	ScheduledWorkflowImagePath,
	APIServerCacheImagePath,
	APIServerMoveResultsImagePath,
	MariaDBImagePath,
	OAuthProxyImagePath,
}

// DefaultDBConnectionTimeout is the default DB storage healthcheck timeout
const DefaultDBConnectionTimeout = time.Second * 15

// DefaultObjStoreConnectionTimeout is the default Object storage healthcheck timeout
const DefaultObjStoreConnectionTimeout = time.Second * 15

const DefaultMaxConcurrentReconciles = 10

const DefaultRequeueTime = time.Second * 20

const DefaultApiServerIncludeOwnerReferenceConfigName = true

func GetConfigRequiredFields() []string {
	return requiredFields
}

// Default ResourceRequirements
var (
	APIServerResourceRequirements         = createResourceRequirement(resource.MustParse("250m"), resource.MustParse("500Mi"), resource.MustParse("500m"), resource.MustParse("1Gi"))
	PersistenceAgentResourceRequirements  = createResourceRequirement(resource.MustParse("120m"), resource.MustParse("500Mi"), resource.MustParse("250m"), resource.MustParse("1Gi"))
	ScheduledWorkflowResourceRequirements = createResourceRequirement(resource.MustParse("120m"), resource.MustParse("100Mi"), resource.MustParse("250m"), resource.MustParse("250Mi"))
	MariaDBResourceRequirements           = createResourceRequirement(resource.MustParse("300m"), resource.MustParse("800Mi"), resource.MustParse("1"), resource.MustParse("1Gi"))
	MinioResourceRequirements             = createResourceRequirement(resource.MustParse("200m"), resource.MustParse("100Mi"), resource.MustParse("250m"), resource.MustParse("1Gi"))
	MlPipelineUIResourceRequirements      = createResourceRequirement(resource.MustParse("100m"), resource.MustParse("256Mi"), resource.MustParse("100m"), resource.MustParse("256Mi"))
	MlmdEnvoyResourceRequirements         = createResourceRequirement(resource.MustParse("100m"), resource.MustParse("256Mi"), resource.MustParse("100m"), resource.MustParse("256Mi"))
	MlmdGRPCResourceRequirements          = createResourceRequirement(resource.MustParse("100m"), resource.MustParse("256Mi"), resource.MustParse("100m"), resource.MustParse("256Mi"))
	MlmdWriterResourceRequirements        = createResourceRequirement(resource.MustParse("100m"), resource.MustParse("256Mi"), resource.MustParse("100m"), resource.MustParse("256Mi"))
)

type DBExtraParams map[string]string

func createResourceRequirement(RequestsCPU resource.Quantity, RequestsMemory resource.Quantity, LimitsCPU resource.Quantity, LimitsMemory resource.Quantity) dspav1alpha1.ResourceRequirements {
	return dspav1alpha1.ResourceRequirements{
		Requests: &dspav1alpha1.Resources{
			CPU:    RequestsCPU,
			Memory: RequestsMemory,
		},
		Limits: &dspav1alpha1.Resources{
			CPU:    LimitsCPU,
			Memory: LimitsMemory,
		},
	}
}

func GetStringConfigWithDefault(configName, value string) string {
	if !viper.IsSet(configName) {
		return value
	}
	return viper.GetString(configName)
}

func GetDurationConfigWithDefault(configName string, value time.Duration) time.Duration {
	if !viper.IsSet(configName) {
		return value
	}
	return viper.GetDuration(configName)
}

func GetBoolConfigWithDefault(configName string, value bool) bool {
	if !viper.IsSet(configName) {
		return value
	}
	return viper.GetBool(configName)
}

// GetCABundleFileMountPath provides the location in pipeline step-copy-artifact step where the
// ca bundle is mounted for aws cli to connect to s3 store.
// Since pipeline step-copy-artifact step uses aws cli, and there are issues surrounding
// passing a path to aws cli (see: https://github.com/aws/aws-cli/issues/3425#issuecomment-402289636)
// as such for pipelines, we concatenate the certs into a single cert bundle and use a separate configmap for this
func GetCABundleFileMountPath() string {
	return fmt.Sprintf("%s/%s", CustomCABundleRootMountPath, CustomDSPTrustedCAConfigMapKey)
}

func GetDefaultDBExtraParams(params DBExtraParams, log logr.Logger) (string, error) {
	extraParamsJson, err := json.Marshal(params)
	if err != nil {
		log.Info(fmt.Sprintf("Error marshaling TLS configuration to JSON: %v", err))
		return "", err
	}
	return string(extraParamsJson), nil
}
