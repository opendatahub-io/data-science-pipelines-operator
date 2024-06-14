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
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"testing"

	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
)

func TestDeployPersistenceAgent(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedPersistenceAgentName := persistenceAgentDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with deployed PersistenceAgent
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			PersistenceAgent: &dspav1alpha1.PersistenceAgent{
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

	// Ensure PersistenceAgent Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedPersistenceAgentName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcilePersistenceAgent(dspa, params)
	assert.Nil(t, err)

	// Ensure PersistenceAgent Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedPersistenceAgentName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Ensure readiness is handled
	persistenceAgentReady, err := reconciler.evaluateCondition(ctx, dspa, params.PersistentAgentDefaultResourceName, config.PersistenceAgentReady)
	assert.Equal(t, "Deploying", persistenceAgentReady.Reason)
	assert.Nil(t, err)
}

func TestDontDeployPersistenceAgent(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedPersistenceAgentName := persistenceAgentDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with non-deployed PersistenceAgent
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			PersistenceAgent: &dspav1alpha1.PersistenceAgent{
				Deploy: false,
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()

	// Ensure PersistenceAgent Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedPersistenceAgentName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcilePersistenceAgent(dspa, params)
	assert.Nil(t, err)

	// Ensure PersistenceAgent Deployment still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedPersistenceAgentName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}
