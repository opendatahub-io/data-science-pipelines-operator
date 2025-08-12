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
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

func TestDeployWorkflowController(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedWorkflowControllerName := "ds-pipeline-workflow-controller-testdspa"

	// Construct DSPASpec with deployed WorkflowController
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: boolPtr(false),
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
			PodToPodTLS: boolPtr(false),
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
			PodToPodTLS: boolPtr(false),
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
			PodToPodTLS: boolPtr(false),
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
