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

package controllers

import (
	"fmt"
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	util "github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

type TestCase struct {
	Description         string
	Path                string
	AdditionalResources map[string][]string
}

type CaseComponentResources map[string]ResourcePath
type ResourcePath map[string]string

type DSPA = dspav1alpha1.DataSciencePipelinesApplication

const SecretKind = "Secret"

var cases = map[string]TestCase{
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
		Description: "custom Artifact configmap is provided, custom images override defaults, no sample pipeline",
		Path:        "./testdata/deploy/case_3/cr.yaml",
		AdditionalResources: map[string][]string{
			SecretKind: {
				"./testdata/deploy/case_3/secret1.yaml",
				"./testdata/deploy/case_3/secret2.yaml",
			},
		},
	},
}
var deploymentsCreated = CaseComponentResources{
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

var deploymentsNotCreated = CaseComponentResources{
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

var configMapsCreated = CaseComponentResources{
	"case_0": {
		"apiserver": "./testdata/results/case_0/apiserver/configmap_artifact_script.yaml",
	},
	"case_2": {
		"apiserver": "./testdata/results/case_2/apiserver/configmap_artifact_script.yaml",
	},
}

var secretsCreated = CaseComponentResources{
	"case_3": {
		"database": "./testdata/results/case_3/database/secret.yaml",
		"storage":  "./testdata/results/case_3/storage/secret.yaml",
	},
}

var configMapsNotCreated = CaseComponentResources{
	"case_3": {
		"apiserver":                "./testdata/results/case_3/apiserver/configmap_artifact_script.yaml",
		"apiserver-sampleconfig":   "./testdata/results/case_3/apiserver/sample-config.yaml",
		"apiserver-samplepipeline": "./testdata/results/case_3/apiserver/sample-pipeline.yaml",
	},
}

var _ = Describe("The DS Pipeline Controller", Ordered, func() {
	client := mfc.NewClient(k8sClient)
	opts := mf.UseClient(client)

	uc := util.UtilContext{}
	BeforeAll(func() {
		uc = util.UtilContext{
			Ctx:    ctx,
			Ns:     WorkingNamespace,
			Opts:   opts,
			Client: k8sClient,
		}
	})

	for tc := range cases {
		// We assign local copies of all looping variables, as they are mutating
		// we want the correct variables captured in each `It` closure, we do this
		// by creating local variables
		// https://onsi.github.io/ginkgo/#dynamically-generating-specs
		testcase := tc
		description := cases[testcase].Description
		dspPath := cases[testcase].Path

		Context(description, func() {
			It(fmt.Sprintf("Should successfully deploy the Custom Resource for case %s", testcase), func() {
				viper.New()
				viper.SetConfigFile(fmt.Sprintf("testdata/deploy/%s/config.yaml", testcase))
				err := viper.ReadInConfig()
				Expect(err).ToNot(HaveOccurred(), "Failed to read config file")
				util.DeployResource(uc, &DSPA{}, dspPath)
				// Deploy any additional resources for this test case
				if cases[testcase].AdditionalResources != nil {
					for res, paths := range cases[testcase].AdditionalResources {
						if res == SecretKind {
							for _, p := range paths {
								util.DeployResource(uc, &v1.Secret{}, p)
							}
						}
					}
				}
			})

			expectedDeployments := deploymentsCreated[testcase]
			for component := range expectedDeployments {
				component := component
				deploymentPath := expectedDeployments[component]
				It(fmt.Sprintf("[%s] Should create deployment for component %s", testcase, component), func() {
					util.CompareResources(uc, &appsv1.Deployment{}, &appsv1.Deployment{}, deploymentPath)
				})
			}

			notExpectedDeployments := deploymentsNotCreated[testcase]
			for component := range deploymentsNotCreated[testcase] {
				deploymentPath := notExpectedDeployments[component]
				It(fmt.Sprintf("[%s] Should NOT create deployments for component %s", testcase, component), func() {
					util.ResourceDoesNotExists(uc, &appsv1.Deployment{}, &appsv1.Deployment{}, deploymentPath)
				})
			}

			for component := range configMapsCreated[testcase] {
				It(fmt.Sprintf("[%s] Should create configmaps for component %s", testcase, component), func() {
					util.CompareResources(uc, &v1.ConfigMap{}, &v1.ConfigMap{}, configMapsCreated[testcase][component])
				})
			}

			for component := range secretsCreated[testcase] {
				It(fmt.Sprintf("[%s] Should create secrets for component %s", testcase, component), func() {
					util.CompareResources(uc, &v1.Secret{}, &v1.Secret{}, secretsCreated[testcase][component])
				})
			}

			for component := range configMapsNotCreated[testcase] {
				It(fmt.Sprintf("[%s] Should NOT create configmaps for component %s", testcase, component), func() {
					util.ResourceDoesNotExists(uc, &v1.ConfigMap{}, &v1.ConfigMap{}, configMapsNotCreated[testcase][component])
				})
			}

			It(fmt.Sprintf("Should successfully delete the Custom Resource for case %s", testcase), func() {
				util.DeleteResource(uc, &DSPA{}, dspPath)
			})
		})
	}
})
