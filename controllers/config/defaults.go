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
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"
	"time"
)

const (
	DefaultImageValue                  = "MustSetInConfig"
	APIServerPiplinesCABundleMountPath = "/etc/pki/tls/certs"
	PiplinesCABundleMountPath          = "/etc/pki/tls/certs"

	// GlobalCaBundleConfigMapName key and label values  are a contract with
	// ODH Platform https://github.com/opendatahub-io/architecture-decision-records/pull/28
	GlobalCaBundleConfigMapName = "odh-trusted-ca-bundle"

	// GlobalCaBundleConfigMapKey is the key provided by the configmap created via OCP Cluster Network Operator
	// https://docs.openshift.com/container-platform/4.14/networking/configuring-a-custom-pki.html#certificate-injection-using-operators_configuring-a-custom-pki
	GlobalCaBundleConfigMapKey = "ca-bundle.crt"

	MLPipelineUIConfigMapPrefix       = "ds-pipeline-ui-configmap-"
	ArtifactScriptConfigMapNamePrefix = "ds-pipeline-artifact-script-"
	ArtifactScriptConfigMapKey        = "artifact_script"
	DSPServicePrefix                  = "ds-pipeline"

	DBSecretNamePrefix   = "ds-pipeline-db-"
	DBSecretKey          = "password"
	DBDefaultExtraParams = "{\"tls\":\"%t\"}"

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

	ObjectStorageSecretName = "mlpipeline-minio-artifact" // hardcoded in kfp-tekton
	ObjectStorageAccessKey  = "accesskey"
	ObjectStorageSecretKey  = "secretkey"

	MlmdGrpcPort = "8080"
)

// DSPO Config File Paths
const (
	APIServerImagePath            = "Images.ApiServer"
	APIServerArtifactImagePath    = "Images.Artifact"
	PersistenceAgentImagePath     = "Images.PersistentAgent"
	ScheduledWorkflowImagePath    = "Images.ScheduledWorkflow"
	APIServerCacheImagePath       = "Images.Cache"
	APIServerMoveResultsImagePath = "Images.MoveResultsImage"
	MariaDBImagePath              = "Images.MariaDB"
	OAuthProxyImagePath           = "Images.OAuthProxy"
	MlmdEnvoyImagePath            = "Images.MlmdEnvoy"
	MlmdGRPCImagePath             = "Images.MlmdGRPC"
	MlmdWriterImagePath           = "Images.MlmdWriter"
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
