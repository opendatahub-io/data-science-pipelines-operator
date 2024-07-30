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
	"log/slog"
	"testing"

	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

func TestDeployUI(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedUIName := "ds-pipeline-ui-testdspa"

	// Construct DSPASpec with deployed UI
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			MlPipelineUI: &dspav1alpha1.MlPipelineUI{
				Deploy: true,
				Image:  "test-image:latest",
			},
			Database: &dspav1alpha1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1alpha1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1alpha1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1alpha1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	//Create Logging using slog
	log := slog.With("namespace", dspa.Namespace).With("dspa_name", dspa.Name)

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, log)
	assert.Nil(t, err)

	// Ensure UI Deployement doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedUIName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileUI(dspa, params)
	assert.Nil(t, err)

	// Ensure UI Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedUIName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
}

func TestDontDeployUI(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedUIName := "ds-pipeline-ui-testdspa"

	// Construct DSPASpec with non-deployed UI
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			MlPipelineUI: &dspav1alpha1.MlPipelineUI{
				Deploy: false,
				Image:  "uiimage",
			},
			Database: &dspav1alpha1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1alpha1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1alpha1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1alpha1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	//Create Logging using slog
	log := slog.With("namespace", dspa.Namespace).With("dspa_name", dspa.Name)

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, log)
	assert.Nil(t, err)

	// Ensure UI Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedUIName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileUI(dspa, params)
	assert.Nil(t, err)

	// Ensure UI Deployment still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedUIName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}

func TestDefaultDeployBehaviorUI(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedUIName := "ds-pipeline-ui-testdspa"

	// Construct DSPASpec without UI defined
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			Database: &dspav1alpha1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1alpha1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1alpha1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1alpha1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Namespace = testNamespace
	dspa.Name = testDSPAName

	//Create Logging using slog
	log := slog.With("namespace", dspa.Namespace).With("dspa_name", dspa.Name)

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	params.ExtractParams(ctx, dspa, reconciler.Client, log)

	// Ensure UI Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedUIName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileUI(dspa, params)
	assert.Nil(t, err)

	// Ensure UI Deployment still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedUIName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}
