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

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestDeployScheduledWorkflow(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedScheduledWorkflowName := scheduledWorkflowDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with deployed ScheduledWorkflow
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			ScheduledWorkflow: &dspav1.ScheduledWorkflow{
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

	// Ensure ScheduledWorkflow Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedScheduledWorkflowName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileScheduledWorkflow(dspa, params)
	assert.Nil(t, err)

	// Ensure ScheduledWorkflow Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedScheduledWorkflowName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
	scheduledWorkflowPolicy := requireNetworkPolicy(t, ctx, reconciler, expectedScheduledWorkflowName)
	assertPolicySelects(t, scheduledWorkflowPolicy, pipelineComponentLabels(expectedScheduledWorkflowName))
	assertNoIngress(t, scheduledWorkflowPolicy)
	assertDNSEgress(t, scheduledWorkflowPolicy)
	assertKubernetesAPIEgress(t, scheduledWorkflowPolicy)
	assertEgressToPod(t, scheduledWorkflowPolicy, 8887, pipelineComponentLabels("ds-pipeline-"+testDSPAName))
	assertEgressPortAbsent(t, scheduledWorkflowPolicy, corev1.ProtocolTCP, 8888)
	require.Len(t, scheduledWorkflowPolicy.Spec.Egress, 3)

	// Ensure readiness is handled
	scheduledWorkflowReady, err := reconciler.evaluateCondition(ctx, dspa, params.ScheduledWorkflowDefaultResourceName, config.ScheduledWorkflowReady)
	assert.Equal(t, "Deploying", scheduledWorkflowReady.Reason)
	assert.Nil(t, err)
}

// The scheduled workflow controller reaches the Kubernetes API through the configured
// proxy when the API address is outside NO_PROXY, so the proxy ports must be allowed
// alongside 443/6443. This workload has no public egress override.
func TestScheduledWorkflowEgressIncludesProxyPorts(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedScheduledWorkflowName := scheduledWorkflowDefaultResourceNamePrefix + testDSPAName

	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			ScheduledWorkflow: &dspav1.ScheduledWorkflow{
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
	require.NoError(t, reconciler.ReconcileScheduledWorkflow(dspa, params))

	scheduledWorkflowPolicy := requireNetworkPolicy(t, ctx, reconciler, expectedScheduledWorkflowName)
	assertPolicySelects(t, scheduledWorkflowPolicy, pipelineComponentLabels(expectedScheduledWorkflowName))
	assertNoIngress(t, scheduledWorkflowPolicy)
	assertDNSEgress(t, scheduledWorkflowPolicy)
	assertKubernetesAPIEgress(t, scheduledWorkflowPolicy)
	assertEgressPortOnly(t, scheduledWorkflowPolicy, corev1.ProtocolTCP, 3128)
	assertEgressPortOnly(t, scheduledWorkflowPolicy, corev1.ProtocolTCP, 8443)
	assertEgressToPod(t, scheduledWorkflowPolicy, 8887, pipelineComponentLabels("ds-pipeline-"+testDSPAName))
	assertEgressPortAbsent(t, scheduledWorkflowPolicy, corev1.ProtocolTCP, 8888)
	require.Len(t, scheduledWorkflowPolicy.Spec.Egress, 5)
}

func TestDontDeployScheduledWorkflow(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedScheduledWorkflowName := scheduledWorkflowDefaultResourceNamePrefix + testDSPAName

	// Construct DSPASpec with non-deployed ScheduledWorkflow
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			ScheduledWorkflow: &dspav1.ScheduledWorkflow{
				Deploy: false,
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()

	// Ensure ScheduledWorkflow Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedScheduledWorkflowName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileScheduledWorkflow(dspa, params)
	assert.Nil(t, err)

	// Ensure ScheduledWorkflow Deployment still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedScheduledWorkflowName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
	assertNetworkPolicyAbsent(t, ctx, reconciler, expectedScheduledWorkflowName)
}
