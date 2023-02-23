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
	dspipelinesiov1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	ODHMLPipelinesVersionTag    = "v1.18.0-8" // ToDO: Make this a flag
	APIServerImage              = "quay.io/modh/odh-ml-pipelines-api-server-container:" + ODHMLPipelinesVersionTag
	APIServerArtifactImage      = "quay.io/modh/odh-ml-pipelines-artifact-manager-container:" + ODHMLPipelinesVersionTag
	APIServerCacheImage         = "registry.access.redhat.com/ubi8/ubi-minimal"
	APIServerMoveResultsImage   = "busybox"
	PersistenceAgentImage       = "quay.io/modh/odh-ml-pipelines-persistenceagent-container:" + ODHMLPipelinesVersionTag
	ScheduledWorkflowImage      = "quay.io/modh/odh-ml-pipelines-scheduledworkflow-container:" + ODHMLPipelinesVersionTag
	ViewerCRDImage              = "quay.io/modh/odh-ml-pipelines-viewercontroller-container:" + ODHMLPipelinesVersionTag
	MlPipelineUIImage           = "quay.io/opendatahub/odh-ml-pipelines-frontend-container:beta-ui"
	MariaDBImage                = "registry.redhat.io/rhel8/mariadb-103:1-188"
	MinioImage                  = "quay.io/opendatahub/minio:RELEASE.2019-08-14T20-37-41Z-license-compliance"
	ArtifactScriptConfigMap     = "ds-pipeline-artifact-script"
	ArtifactScriptConfigMapKey  = "artifact_script"
	DSPServicePrefix            = "ds-pipeline"
	MariaDBName                 = "mlpipeline"
	MariaDBHostPrefix           = "mariadb"
	MariaDBHostPort             = "3306"
	MariaDBUser                 = "mlpipeline"
	MariaDBSecretNamePrefix     = "mariadb-"
	MariaDBSecretKey            = "password"
	MinioHostPrefix             = "minio"
	MinioPort                   = "9000"
	MinioScheme                 = "http"
	MinioDefaultBucket          = "mlpipeline"
	ObjectStoreConnectionSecure = false
	ObjectStorageSecretName     = "mlpipeline-minio-artifact" // hardcoded in kfp-tekton
	ObjectStorageAccessKey      = "accesskey"
	ObjectStorageSecretKey      = "secretkey"
)

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

func createResourceRequirement(RequestsCPU string, RequestsMemory string, LimitsCPU string, LimitsMemory string) dspipelinesiov1alpha1.ResourceRequirements {
	return dspipelinesiov1alpha1.ResourceRequirements{
		Requests: dspipelinesiov1alpha1.Resources{
			CPU:    resource.MustParse(RequestsCPU),
			Memory: resource.MustParse(RequestsMemory),
		},
		Limits: dspipelinesiov1alpha1.Resources{
			CPU:    resource.MustParse(LimitsCPU),
			Memory: resource.MustParse(LimitsMemory),
		},
	}
}
