//go:build test_all || test_unit

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
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestDeployWorkflowController(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedWorkflowControllerName := "ds-pipeline-workflow-controller-testdspa"

	// Construct DSPASpec with deployed WorkflowController
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer:   &dspav1.APIServer{},
			WorkflowController: &dspav1.WorkflowController{
				Deploy: true,
			},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			MLMD: &dspav1.MLMD{Deploy: true},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Ensure WorkflowController Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(dspa, params)
	assert.Nil(t, err)
	assert.True(t, workflowControllerEnabled)

	// Ensure WorkflowController Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	cm := &corev1.ConfigMap{}
	created, err = reconciler.IsResourceCreated(ctx, cm, expectedWorkflowControllerName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
	assert.Empty(t, cm.Data["workflowDefaults"])
}

func TestDontDeployWorkflowController(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedWorkflowControllerName := "ds-pipeline-workflow-controller-testdspa"

	// Construct DSPASpec with non-deployed WorkflowController
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			WorkflowController: &dspav1.WorkflowController{
				Deploy: false,
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()

	// Ensure WorkflowController Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(dspa, params)
	assert.Nil(t, err)
	assert.False(t, workflowControllerEnabled)

	// Ensure WorkflowController Deployment still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}

func TestChangeManagementStateWorkflowController(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedWorkflowControllerName := "ds-pipeline-workflow-controller-testdspa"

	// Construct DSPASpec with deployed WorkflowController
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer:   &dspav1.APIServer{},
			WorkflowController: &dspav1.WorkflowController{
				Deploy: true,
			},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			MLMD: &dspav1.MLMD{Deploy: true},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Ensure WorkflowController Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation using default global managementState for WorkflowController
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(dspa, params)
	assert.Nil(t, err)
	assert.True(t, workflowControllerEnabled)

	// Ensure WorkflowController Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Now set the global ManagementState of the WorkflowControllers to Removed
	viper.Set("DSPO.ArgoWorkflowsControllers", "{\"managementState\":\"Removed\"}")

	// Run test reconciliation
	workflowControllerEnabled, err = reconciler.ReconcileWorkflowController(dspa, params)
	assert.Nil(t, err)
	assert.False(t, workflowControllerEnabled)

	// Ensure WorkflowController Deployment now removed
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Set the global ManagementState of the WorkflowControllers to Managed
	viper.Set("DSPO.ArgoWorkflowsControllers", "{\"managementState\":\"Managed\"}")

	// Run test reconciliation
	workflowControllerEnabled, err = reconciler.ReconcileWorkflowController(dspa, params)
	assert.Nil(t, err)
	assert.True(t, workflowControllerEnabled)

	// Ensure WorkflowController Deployment now exists again
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
}

func TestBadManagementStateWorkflowController(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedWorkflowControllerName := "ds-pipeline-workflow-controller-testdspa"

	// Construct DSPASpec with deployed WorkflowController
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer:   &dspav1.APIServer{},
			WorkflowController: &dspav1.WorkflowController{
				Deploy: true,
			},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			MLMD: &dspav1.MLMD{Deploy: true},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Ensure WorkflowController Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Now set the global ManagementState of the WorkflowControllers to invalid state
	viper.Set("DSPO.ArgoWorkflowsControllers", "{\"managementState\":\"InvalidState\"}")

	// Run test reconciliation
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(dspa, params)
	assert.NotNil(t, err)
	assert.False(t, workflowControllerEnabled)
}

func TestManagementStateWorkflowControllerInvalidJSONRecovery(t *testing.T) {
	t.Cleanup(func() { viper.Reset() })

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedWorkflowControllerName := "ds-pipeline-workflow-controller-testdspa"

	// Construct DSPASpec with deployed WorkflowController
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer:   &dspav1.APIServer{},
			WorkflowController: &dspav1.WorkflowController{
				Deploy: true,
			},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			MLMD: &dspav1.MLMD{Deploy: true},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Ensure WorkflowController Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Now set the global ManagementState of the WorkflowControllers to Removed
	viper.Set("DSPO.ArgoWorkflowsControllers", "{invalidJSON: 'foo")

	// Run test reconciliation
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(dspa, params)
	assert.Nil(t, err)
	assert.True(t, workflowControllerEnabled)

	// Ensure WorkflowController Deployment still created
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
}

func TestWorkflowControllerConfigMapInjectsSSLCertDirWhenCABundlePresent(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedConfigMapName := "ds-pipeline-workflow-controller-testdspa"

	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer: &dspav1.APIServer{
				Deploy: true,
				CABundle: &dspav1.CABundle{
					ConfigMapKey:  "testcakey",
					ConfigMapName: "testcaname",
				},
			},
			WorkflowController: &dspav1.WorkflowController{
				Deploy: true,
			},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			MLMD: &dspav1.MLMD{Deploy: true},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	ctx, params, reconciler := CreateNewTestObjects()
	t.Setenv("SSL_CERT_FILE", "testdata/tls/empty-ca-bundle.crt")
	require.NoError(t, reconciler.Client.Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "testcaname", Namespace: testNamespace},
		Data:       map[string]string{"testcakey": "bundle-contents"},
	}))

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.Equal(t, config.WorkflowPodSSLCertDir, params.WorkflowPodSSLCertDir)

	enabled, err := reconciler.ReconcileWorkflowController(dspa, params)
	require.NoError(t, err)
	require.True(t, enabled)

	cm := &corev1.ConfigMap{}
	require.NoError(t, reconciler.Get(ctx, types.NamespacedName{
		Name:      expectedConfigMapName,
		Namespace: testNamespace,
	}, cm))
	require.Contains(t, cm.Data["workflowDefaults"], "SSL_CERT_DIR")
	require.Contains(t, cm.Data["workflowDefaults"], config.WorkflowPodSSLCertDir)
}
