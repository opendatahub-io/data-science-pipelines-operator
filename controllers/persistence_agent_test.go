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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestDeployPersistenceAgent(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedPersistenceAgentName := persistenceAgentDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with deployed PersistenceAgent
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PersistenceAgent: &dspav1.PersistenceAgent{
				Deploy: true,
			},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
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
	persistencePolicy := requireNetworkPolicy(t, ctx, reconciler, expectedPersistenceAgentName)
	assertPolicySelects(t, persistencePolicy, pipelineComponentLabels(expectedPersistenceAgentName))
	assertNoIngress(t, persistencePolicy)
	assertDNSEgress(t, persistencePolicy)
	assertKubernetesAPIEgress(t, persistencePolicy)
	apiServerLabels := pipelineComponentLabels("ds-pipeline-" + testDSPAName)
	assertEgressToPod(t, persistencePolicy, 8888, apiServerLabels)
	assertEgressToPod(t, persistencePolicy, 8887, apiServerLabels)
	require.Len(t, persistencePolicy.Spec.Egress, 3)

	// Ensure readiness is handled
	persistenceAgentReady, err := reconciler.evaluateCondition(ctx, dspa, params.PersistentAgentDefaultResourceName, config.PersistenceAgentReady)
	assert.Equal(t, "Deploying", persistenceAgentReady.Reason)
	assert.Nil(t, err)
}

// The persistence agent reaches the Kubernetes API through the configured proxy when
// the API address is outside NO_PROXY, so the proxy ports must be allowed alongside
// 443/6443. This workload has no public egress override.
func TestPersistenceAgentEgressIncludesProxyPorts(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedPersistenceAgentName := persistenceAgentDefaultResourceNamePrefix + testDSPAName

	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PersistenceAgent: &dspav1.PersistenceAgent{
				Deploy: true,
			},
			Proxy: &dspav1.ProxyConfig{
				HTTPProxy:  "http://proxy.example.com:3128",
				HTTPSProxy: "https://proxy.example.com:8443",
			},
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
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
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcilePersistenceAgent(dspa, params))

	persistencePolicy := requireNetworkPolicy(t, ctx, reconciler, expectedPersistenceAgentName)
	assertPolicySelects(t, persistencePolicy, pipelineComponentLabels(expectedPersistenceAgentName))
	assertNoIngress(t, persistencePolicy)
	assertDNSEgress(t, persistencePolicy)
	assertKubernetesAPIEgress(t, persistencePolicy)
	assertEgressPortOnly(t, persistencePolicy, corev1.ProtocolTCP, 3128)
	assertEgressPortOnly(t, persistencePolicy, corev1.ProtocolTCP, 8443)
	apiServerLabels := pipelineComponentLabels("ds-pipeline-" + testDSPAName)
	assertEgressToPod(t, persistencePolicy, 8888, apiServerLabels)
	assertEgressToPod(t, persistencePolicy, 8887, apiServerLabels)
	require.Len(t, persistencePolicy.Spec.Egress, 5)
}

func TestDontDeployPersistenceAgent(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedPersistenceAgentName := persistenceAgentDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with non-deployed PersistenceAgent
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PersistenceAgent: &dspav1.PersistenceAgent{
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
	assertNetworkPolicyAbsent(t, ctx, reconciler, expectedPersistenceAgentName)
}
