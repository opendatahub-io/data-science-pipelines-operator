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
)

const (
	DefaultImageValue = "MustSetInConfig"

	MLPipelineUIConfigMapPrefix       = "ds-pipeline-ui-configmap-"
	ArtifactScriptConfigMapNamePrefix = "ds-pipeline-artifact-script-"
	ArtifactScriptConfigMapKey        = "artifact_script"
	DSPServicePrefix                  = "ds-pipeline"

	DBSecretNamePrefix = "ds-pipeline-db-"
	DBSecretKey        = "password"

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

const (
	DatabaseReady               = "DatabaseReady"
	DataBaseFailedToDeploy      = "DataBaseFailedToDeploy"
	ObjectStorageReady          = "ObjectStorageReady"
	ObjectStorageFailedToDeploy = "ObjectStorageFailedToDeploy"
	APIServerReady              = "APIServerReady"
	PersistenceAgentReady       = "PersistenceAgentReady"
	ScheduledWorkflowReady      = "ScheduledWorkflowReady"
	UserInterfaceReady          = "UserInterfaceReady"
	CrReady                     = "Ready"
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
	APIServerResourceRequirements         = createResourceRequirement(resource.MustParse("250m"), resource.MustParse("500Mi"), resource.MustParse("500m"), resource.MustParse("1Gi"))
	PersistenceAgentResourceRequirements  = createResourceRequirement(resource.MustParse("120m"), resource.MustParse("500Mi"), resource.MustParse("250m"), resource.MustParse("1Gi"))
	ScheduledWorkflowResourceRequirements = createResourceRequirement(resource.MustParse("120m"), resource.MustParse("100Mi"), resource.MustParse("250m"), resource.MustParse("250Mi"))
	ViewerCRDResourceRequirements         = createResourceRequirement(resource.MustParse("120m"), resource.MustParse("100Mi"), resource.MustParse("250m"), resource.MustParse("500Mi"))
	MariaDBResourceRequirements           = createResourceRequirement(resource.MustParse("300m"), resource.MustParse("800Mi"), resource.MustParse("1"), resource.MustParse("1Gi"))
	MinioResourceRequirements             = createResourceRequirement(resource.MustParse("200m"), resource.MustParse("100Mi"), resource.MustParse("250m"), resource.MustParse("1Gi"))
	MlPipelineUIResourceRequirements      = createResourceRequirement(resource.MustParse("100m"), resource.MustParse("256Mi"), resource.MustParse("100m"), resource.MustParse("256Mi"))
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
