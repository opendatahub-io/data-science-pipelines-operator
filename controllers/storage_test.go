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
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDeployStorage(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with deployed Minio Object Storage
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
					Deploy: true,
					Image:  "someimage",
					Resources: &dspav1alpha1.ResourceRequirements{ //TODO: fails without this block.  Why?
						Requests: &dspav1alpha1.Resources{
							CPU:    resource.MustParse("250m"),
							Memory: resource.MustParse("500Mi"),
						},
						Limits: &dspav1alpha1.Resources{
							CPU:    resource.MustParse("500m"),
							Memory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileStorage(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
}
func TestDontDeployStorage(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with non-deployed Minio Object Storage
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
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileStorage(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment still doesn't exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}

func TestDefaultDeployBehaviorStorage(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with deployed Minio Object Storage
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
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.NotNil(t, err) // DSPAParams should throw an error if no objstore is provided

	// Assert ObjectStorage Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileStorage(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment still doesn't exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}
