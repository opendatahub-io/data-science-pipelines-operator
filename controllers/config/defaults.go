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
)

const (
	DefaultImageValue = "MustSetInConfig"

	MLPipelineUIConfigMapPrefix       = "ds-pipeline-ui-configmap-"
	ArtifactScriptConfigMapNamePrefix = "ds-pipeline-artifact-script-"
	ArtifactScriptConfigMapKey        = "artifact_script"
	DSPServicePrefix                  = "ds-pipeline"

	MariaDBName             = "mlpipeline"
	MariaDBHostPrefix       = "mariadb"
	MariaDBHostPort         = "3306"
	MariaDBUser             = "mlpipeline"
	MariaDBSecretNamePrefix = "mariadb-"
	MariaDBSecretKey        = "password"
	MariaDBNamePVCSize      = "10Gi"

	MinioHostPrefix    = "minio"
	MinioPort          = "9000"
	MinioScheme        = "http"
	MinioDefaultBucket = "mlpipeline"
	MinioPVCSize       = "10Gi"

	ObjectStoreConnectionSecure = false
	ObjectStorageSecretName     = "mlpipeline-minio-artifact" // hardcoded in kfp-tekton
	ObjectStorageAccessKey      = "accesskey"
	ObjectStorageSecretKey      = "secretkey"
)

// DSPO Config File Paths
const (
	APIServerImagePath            = "Images.ApiServer"
	APIServerArtifactImagePath    = "Images.Artifact"
	PersistenceAgentImagePath     = "Images.PersistentAgent"
	ScheduledWorkflowImagePath    = "Images.ScheduledWorkflow"
	ViewerCRDImagePath            = "Images.ViewerCRD"
	APIServerCacheImagePath       = "Images.Cache"
	APIServerMoveResultsImagePath = "Images.MoveResultsImage"
	MariaDBImagePath              = "Images.MariaDB"
)

// Any required Configmap paths can be added here,
// they will be automatically included for required
// validation check
var requiredFields = []string{
	APIServerImagePath,
	APIServerArtifactImagePath,
	PersistenceAgentImagePath,
	ScheduledWorkflowImagePath,
	ViewerCRDImagePath,
	APIServerCacheImagePath,
	APIServerMoveResultsImagePath,
	MariaDBImagePath,
}

func GetConfigRequiredFields() []string {
	return requiredFields
}

// Default ResourceRequirements
var (
	APIServerResourceRequirements         = createResourceRequirement("250m", "500Mi", "500m", "1Gi")
	PersistenceAgentResourceRequirements  = createResourceRequirement("120m", "500Mi", "250m", "1Gi")
	ScheduledWorkflowResourceRequirements = createResourceRequirement("120m", "100Mi", "250m", "250Mi")
	ViewerCRDResourceRequirements         = createResourceRequirement("120m", "100Mi", "250m", "500Mi")
	MariaDBResourceRequirements           = createResourceRequirement("300m", "800Mi", "1", "1Gi")
	MinioResourceRequirements             = createResourceRequirement("200m", "100Mi", "250m", "1Gi")
	MlPipelineUIResourceRequirements      = createResourceRequirement("100m", "256Mi", "100m", "256Mi")
)

func createResourceRequirement(RequestsCPU string, RequestsMemory string, LimitsCPU string, LimitsMemory string) dspav1alpha1.ResourceRequirements {
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
