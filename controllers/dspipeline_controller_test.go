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

type DSPA = dspav1alpha1.DataSciencePipelinesApplication

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

	for tc := range util.Cases {
		// We assign local copies of all looping variables, as they are mutating
		// we want the correct variables captured in each `It` closure, we do this
		// by creating local variables
		// https://onsi.github.io/ginkgo/#dynamically-generating-specs
		testcase := tc
		description := util.Cases[testcase].Description
		dspPath := util.Cases[testcase].Path

		Context(description, func() {
			It(fmt.Sprintf("Should successfully deploy the Custom Resource for case %s", testcase), func() {
				viper.New()
				viper.SetConfigFile(fmt.Sprintf("testdata/deploy/%s/config.yaml", testcase))
				err := viper.ReadInConfig()
				Expect(err).ToNot(HaveOccurred(), "Failed to read config file")
				util.DeployResource(uc, &DSPA{}, dspPath)
				// Deploy any additional resources for this test case
				if util.Cases[testcase].AdditionalResources != nil {
					for res, paths := range util.Cases[testcase].AdditionalResources {
						if res == util.SecretKind {
							for _, p := range paths {
								util.DeployResource(uc, &v1.Secret{}, p)
							}
						}
					}
				}
			})

			expectedDeployments := util.DeploymentsCreated[testcase]
			for component := range expectedDeployments {
				component := component
				deploymentPath := expectedDeployments[component]
				It(fmt.Sprintf("[%s] Should create deployment for component %s", testcase, component), func() {
					util.CompareResources(uc, &appsv1.Deployment{}, &appsv1.Deployment{}, deploymentPath)
				})
			}

			notExpectedDeployments := util.DeploymentsNotCreated[testcase]
			for component := range util.DeploymentsNotCreated[testcase] {
				deploymentPath := notExpectedDeployments[component]
				It(fmt.Sprintf("[%s] Should NOT create deployments for component %s", testcase, component), func() {
					util.ResourceDoesNotExists(uc, &appsv1.Deployment{}, &appsv1.Deployment{}, deploymentPath)
				})
			}

			for component := range util.ConfigMapsCreated[testcase] {
				It(fmt.Sprintf("[%s] Should create configmaps for component %s", testcase, component), func() {
					util.CompareResources(uc, &v1.ConfigMap{}, &v1.ConfigMap{}, util.ConfigMapsCreated[testcase][component])
				})
			}

			for component := range util.SecretsCreated[testcase] {
				It(fmt.Sprintf("[%s] Should create secrets for component %s", testcase, component), func() {
					util.CompareResources(uc, &v1.Secret{}, &v1.Secret{}, util.SecretsCreated[testcase][component])
				})
			}

			for component := range util.ConfigMapsNotCreated[testcase] {
				It(fmt.Sprintf("[%s] Should NOT create configmaps for component %s", testcase, component), func() {
					util.ResourceDoesNotExists(uc, &v1.ConfigMap{}, &v1.ConfigMap{}, util.ConfigMapsNotCreated[testcase][component])
				})
			}

			It(fmt.Sprintf("Should successfully delete the Custom Resource for case %s", testcase), func() {
				util.DeleteResource(uc, &DSPA{}, dspPath)
			})
		})
	}
})
