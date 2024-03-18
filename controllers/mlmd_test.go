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

	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

func TestDeployMLMD(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedMLMDEnvoyName := "ds-pipeline-metadata-envoy-testdspa"
	expectedMLMDGRPCName := "ds-pipeline-metadata-grpc-testdspa"
	expectedMLMDWriterName := "ds-pipeline-metadata-writer-testdspa"

	// Construct DSPA Spec with MLMD Enabled
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			APIServer: &dspav1alpha1.APIServer{},
			MLMD: &dspav1alpha1.MLMD{
				Deploy: true,
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

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Ensure MLMD-Envoy resources doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedMLMDEnvoyName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-GRPC resources doesn't yet exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDGRPCName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-Writer resources doesn't yet exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDWriterName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileMLMD(dspa, params)
	assert.Nil(t, err)

	// Ensure MLMD-Envoy resources now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDEnvoyName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-GRPC resources now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDGRPCName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-Writer resources now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDWriterName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
}

func TestDontDeployMLMD(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedMLMDEnvoyName := "ds-pipeline-metadata-envoy-testdspa"
	expectedMLMDGRPCName := "ds-pipeline-metadata-grpc-testdspa"
	expectedMLMDWriterName := "ds-pipeline-metadata-writer-testdspa"

	// Construct DSPA Spec with MLMD Not Enabled
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			APIServer: &dspav1alpha1.APIServer{},
			MLMD: &dspav1alpha1.MLMD{
				Deploy: false,
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

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Ensure MLMD-Envoy resources doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedMLMDEnvoyName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-GRPC resources doesn't yet exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDGRPCName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-Writer resources doesn't yet exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDWriterName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileMLMD(dspa, params)
	assert.Nil(t, err)

	// Ensure MLMD-Envoy resources still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDEnvoyName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-GRPC resources stil doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDGRPCName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-Writer resources still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDWriterName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}

func TestDefaultDeployBehaviorMLMD(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedMLMDEnvoyName := "ds-pipeline-metadata-envoy-testdspa"
	expectedMLMDGRPCName := "ds-pipeline-metadata-grpc-testdspa"
	expectedMLMDWriterName := "ds-pipeline-metadata-writer-testdspa"

	// Construct DSPA Spec with MLMD Spec not defined
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			APIServer: &dspav1alpha1.APIServer{},
			MLMD: &dspav1alpha1.MLMD{
				Deploy: true,
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

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Ensure MLMD-Envoy resources doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedMLMDEnvoyName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-GRPC resources doesn't yet exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDGRPCName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-Writer resources doesn't yet exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDWriterName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileMLMD(dspa, params)
	assert.Nil(t, err)

	// Ensure MLMD-Envoy resources still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDEnvoyName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-GRPC resources still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDGRPCName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Ensure MLMD-Writer resources still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedMLMDWriterName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
}
