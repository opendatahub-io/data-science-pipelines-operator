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
	"os"
	"path/filepath"
	"testing"

	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	util "github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
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

	testcases := util.GenerateDeclarativeTestCases(s.T(), util.CasesDir)

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

func (s *ControllerSuite) TestPostgreSQLDeclarative() {
	setup()

	reconciler := &DSPAReconciler{
		Client:        k8sClient,
		Log:           ctrl.Log.WithName("postgresql-declarative"),
		Scheme:        k8sClient.Scheme(),
		TemplatesPath: "../config/internal/",
	}

	for _, tc := range util.GenerateDeclarativeTestCases(s.T(), "./testdata/postgresql") {
		s.T().Run(filepath.Base(filepath.Dir(tc.Config)), func(t *testing.T) {
			t.Setenv("SSL_CERT_FILE", "testdata/tls/ca-bundle.crt")
			viper.SetConfigFile(tc.Config)
			require.NoError(t, viper.ReadInConfig())
			for _, path := range tc.Deploy {
				util.DeployResource(uc, path, t)
				t.Cleanup(func() { util.DeleteResource(uc, path, t) })
			}

			manifest, err := os.ReadFile(filepath.Join(filepath.Dir(tc.Config), "dspa.yaml"))
			require.NoError(t, err)
			dspa := &dspav1.DataSciencePipelinesApplication{}
			require.NoError(t, yaml.UnmarshalStrict(manifest, dspa))
			dspa.Namespace = uc.Ns
			typeMeta := dspa.TypeMeta
			// Exercise CRD validation and defaults without handing the DSPA to
			// the manager, which still gates PostgreSQL workload deployment.
			require.NoError(t, k8sClient.Create(ctx, dspa, client.DryRunAll))
			// The typed client clears type metadata, which owner references need.
			dspa.TypeMeta = typeMeta

			params := &DSPAParams{}
			require.NoError(t, params.ExtractParams(ctx, dspa, k8sClient, reconciler.Log))
			require.NoError(t, reconciler.ReconcileDatabase(ctx, dspa, params))
			require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

			for _, path := range tc.Expected.Created {
				util.CompareResources(uc, path, t)
			}
			for _, path := range tc.Expected.NotCreated {
				util.ResourceDoesNotExists(uc, path, t)
			}
		})
	}
}
