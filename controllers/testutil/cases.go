/*

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

package testutil

const SecretKind = "Secret"

type TestCase struct {
	Description         string
	Path                string
	AdditionalResources map[string][]string
}

type CaseComponentResources map[string]ResourcePath
type ResourcePath map[string]string

var Cases = map[string]TestCase{
	"case_0": {
		Description: "empty CR Spec",
		Path:        "./testdata/deploy/case_0/cr.yaml",
	},
	"case_1": {
		Description: "all Deploy fields are set to false",
		Path:        "./testdata/deploy/case_1/cr.yaml",
	},
	"case_2": {
		Description: "standard CR Spec with components specified",
		Path:        "./testdata/deploy/case_2/cr.yaml",
	},
	"case_3": {
		Description: "custom Artifact configmap is provided, custom images override defaults",
		Path:        "./testdata/deploy/case_3/cr.yaml",
		AdditionalResources: map[string][]string{
			SecretKind: {
				"./testdata/deploy/case_3/secret1.yaml",
				"./testdata/deploy/case_3/secret2.yaml",
			},
		},
	},
}
var DeploymentsCreated = CaseComponentResources{
	"case_0": {
		"apiserver":                   "./testdata/results/case_0/apiserver/deployment.yaml",
		"mariadb":                     "./testdata/results/case_0/mariadb/deployment.yaml",
		"persistenceAgentDeployment":  "./testdata/results/case_0/persistence-agent/deployment.yaml",
		"scheduledWorkflowDeployment": "./testdata/results/case_0/scheduled-workflow/deployment.yaml",
	},
	"case_2": {
		"apiserver":                   "./testdata/results/case_2/apiserver/deployment.yaml",
		"mariadb":                     "./testdata/results/case_2/mariadb/deployment.yaml",
		"minioDeployment":             "./testdata/results/case_2/minio/deployment.yaml",
		"mlpipelinesUIDeployment":     "./testdata/results/case_2/mlpipelines-ui/deployment.yaml",
		"persistenceAgentDeployment":  "./testdata/results/case_2/persistence-agent/deployment.yaml",
		"scheduledWorkflowDeployment": "./testdata/results/case_2/scheduled-workflow/deployment.yaml",
		"viewerCrdDeployment":         "./testdata/results/case_2/viewer-crd/deployment.yaml",
	},
	"case_3": {
		"apiserver": "./testdata/results/case_3/apiserver/deployment.yaml",
	},
}

var DeploymentsNotCreated = CaseComponentResources{
	"case_0": {
		"viewerCrdDeployment": "./testdata/results/case_0/viewer-crd/deployment.yaml",
	},
	"case_1": {
		"apiserver":                   "./testdata/results/case_1/apiserver/deployment.yaml",
		"mariadb":                     "./testdata/results/case_1/mariadb/deployment.yaml",
		"minioDeployment":             "./testdata/results/case_1/minio/deployment.yaml",
		"mlpipelinesUIDeployment":     "./testdata/results/case_1/mlpipelines-ui/deployment.yaml",
		"persistenceAgentDeployment":  "./testdata/results/case_1/persistence-agent/deployment.yaml",
		"scheduledWorkflowDeployment": "./testdata/results/case_1/scheduled-workflow/deployment.yaml",
	},
}

var ConfigMapsCreated = CaseComponentResources{
	"case_0": {
		"apiserver": "./testdata/results/case_0/apiserver/configmap_artifact_script.yaml",
	},
	"case_2": {
		"apiserver": "./testdata/results/case_2/apiserver/configmap_artifact_script.yaml",
	},
}

var SecretsCreated = CaseComponentResources{
	"case_3": {
		"database": "./testdata/results/case_3/database/secret.yaml",
		"storage":  "./testdata/results/case_3/storage/secret.yaml",
	},
}

var ConfigMapsNotCreated = CaseComponentResources{
	"case_3": {
		"apiserver": "./testdata/results/case_3/apiserver/configmap_artifact_script.yaml",
	},
}
