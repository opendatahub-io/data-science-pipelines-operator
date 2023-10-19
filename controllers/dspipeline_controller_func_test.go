//go:build test_all || test_functional
// +build test_all test_functional

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
	util "github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/spf13/viper"
)

var _ = Describe("The DS Pipeline Controller", Ordered, func() {
	uc := util.UtilContext{}
	BeforeAll(func() {
		client := mfc.NewClient(k8sClient)
		opts := mf.UseClient(client)
		uc = util.UtilContext{
			Ctx:    ctx,
			Ns:     WorkingNamespace,
			Opts:   opts,
			Client: k8sClient,
		}
	})

	testcases := util.GenerateDeclarativeTestCases()

	for caseCount, tc := range testcases {
		// We assign local copies of all looping variables, as they are mutating
		// we want the correct variables captured in each `It` closure, we do this
		// by creating local variables
		// https://onsi.github.io/ginkgo/#dynamically-generating-specs
		testcase := tc
		description := testcase.Description
		Context(description, func() {
			paths := testcase.Deploy
			It(fmt.Sprintf("[case %x] Should successfully deploy the Custom Resource (and additional resources)", caseCount), func() {
				viper.New()
				viper.SetConfigFile(testcase.Config)
				err := viper.ReadInConfig()
				Expect(err).ToNot(HaveOccurred(), "Failed to read config file")
				for _, path := range paths {
					util.DeployResource(uc, path)
				}
			})

			It(fmt.Sprintf("[case %x] Should create expected resources", caseCount), func() {
				for _, resourcesCreated := range testcase.Expected.Created {
					util.CompareResources(uc, resourcesCreated)
				}
			})

			It(fmt.Sprintf("[case %x] Should expect NOT to create some resources", caseCount), func() {
				for _, resourcesNotCreated := range testcase.Expected.NotCreated {
					util.ResourceDoesNotExists(uc, resourcesNotCreated)
				}
			})

			It(fmt.Sprintf("[case %x] Should successfully delete the Custom Resource (and additional resources)", testcase), func() {
				for _, path := range testcase.Deploy {
					p := path
					util.DeleteResource(uc, p)
				}
			})
		})
	}
})
