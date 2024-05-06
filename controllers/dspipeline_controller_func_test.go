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
	util "github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"testing"
)

var uc = util.UtilContext{}

func setup() {
	client := mfc.NewClient(k8sClient)
	opts := mf.UseClient(client)
	uc = util.UtilContext{
		Ctx:    ctx,
		Ns:     WorkingNamespace,
		Opts:   opts,
		Client: k8sClient,
	}
}

func (s *ControllerSuite) TestDSPipelineController() {
	setup()

	testcases := util.GenerateDeclarativeTestCases(s.T())

	for caseCount, tc := range testcases {
		// We assign local copies of all looping variables, as they are mutating
		// we want the correct variables captured in each `It` closure, we do this
		// by creating local variables
		testcase := tc
		description := testcase.Description
		paths := testcase.Deploy
		s.T().Run(fmt.Sprintf("Case %d: %s", caseCount, description), func(t *testing.T) {
			t.Run(fmt.Sprintf("[case %x] Should successfully deploy the Custom Resource (and additional resources)", caseCount), func(t *testing.T) {
				viper.New()
				viper.SetConfigFile(testcase.Config)
				err := viper.ReadInConfig()
				assert.NoError(t, err)
				for _, path := range paths {
					util.DeployResource(uc, path, t)
				}
			})

			t.Run(fmt.Sprintf("[case %x] Should create expected resources", caseCount), func(t *testing.T) {
				for _, resourcesCreated := range testcase.Expected.Created {
					util.CompareResources(uc, resourcesCreated, t)
				}
			})

			t.Run(fmt.Sprintf("[case %x] Should expect NOT to create some resources", caseCount), func(t *testing.T) {
				for _, resourcesNotCreated := range testcase.Expected.NotCreated {
					util.ResourceDoesNotExists(uc, resourcesNotCreated, t)
				}
			})

			t.Run(fmt.Sprintf("[case %x] Should successfully delete the Custom Resource (and additional resources)", caseCount), func(t *testing.T) {
				for _, path := range testcase.Deploy {
					p := path
					util.DeleteResource(uc, p, t)
				}
			})
		})
	}
}
