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
	"context"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDeployCommonPolicies(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedNetworkPolicyName := "ds-pipelines-testdspa"
	expectedEnvoyNetworkPolicyName := "ds-pipelines-envoy-testdspa"

	// Construct Basic DSPA Spec
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
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
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert Common NetworkPolicies don't yet exist
	np := &networkingv1.NetworkPolicy{}
	created, err := reconciler.IsResourceCreated(ctx, np, expectedNetworkPolicyName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	np = &networkingv1.NetworkPolicy{}
	created, err = reconciler.IsResourceCreated(ctx, np, expectedEnvoyNetworkPolicyName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileCommon(dspa, params)
	assert.Nil(t, err)

	// Assert Common NetworkPolicies now exist
	np = &networkingv1.NetworkPolicy{}
	created, err = reconciler.IsResourceCreated(ctx, np, expectedNetworkPolicyName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
	// APIServer is omitted, so the OAuth proxy port is not opened.
	assertPortAbsent(t, np, 8443)
	assertPortAbsent(t, np, 8445)
	assertDirectAPICallers(t, np, 8888, apiServerHTTPPeers(testDSPAName))
	assertDirectAPICallers(t, np, 8887, apiServerGRPCPeers(testDSPAName))

	np = &networkingv1.NetworkPolicy{}
	created, err = reconciler.IsResourceCreated(ctx, np, expectedEnvoyNetworkPolicyName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
	// MLMD is omitted and defaults to deploying the Envoy route.
	assertPortPresent(t, np, 8443)

	driverPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipeline-drivers-"+testDSPAName)
	assertNoIngress(t, driverPolicy)
	assertDriverRoleSelector(t, driverPolicy)
}

func TestCommonPoliciesFollowProxyFlags(t *testing.T) {
	dspa := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{EnableRoute: true},
		MLMD: &dspav1.MLMD{
			Deploy: true,
			Envoy:  &dspav1.Envoy{DeployRoute: false},
		},
	})

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileCommon(dspa, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertPortPresent(t, apiPolicy, 8443)
	assertIngressFrom(t, apiPolicy, 8445, monitoringNamespacePeers())
	assertDirectAPICallers(t, apiPolicy, 8888, apiServerHTTPPeers(ingressTestDSPAName))
	assertDirectAPICallers(t, apiPolicy, 8887, apiServerGRPCPeers(ingressTestDSPAName))
	assertPortAbsent(t, requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-envoy-"+ingressTestDSPAName), 8443)
}

func TestCommonPoliciesKeepMetricsPortWithoutRoute(t *testing.T) {
	dspa := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{EnableRoute: false},
	})

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileCommon(dspa, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertPortAbsent(t, apiPolicy, 8443)
	assertIngressFrom(t, apiPolicy, 8445, monitoringNamespacePeers())
}

const ingressTestDSPAName = "testdspa"
const ingressTestNamespace = "testnamespace"

func newIngressTestDSPA(spec dspav1.DSPASpec) *dspav1.DataSciencePipelinesApplication {
	podToPodTLS := false
	if spec.PodToPodTLS == nil {
		spec.PodToPodTLS = &podToPodTLS
	}
	if spec.DSPVersion == "" {
		spec.DSPVersion = "v2"
	}
	if spec.Database == nil {
		spec.Database = &dspav1.Database{MariaDB: &dspav1.MariaDB{Deploy: true}}
	}
	if spec.ObjectStorage == nil {
		spec.ObjectStorage = &dspav1.ObjectStorage{Minio: &dspav1.Minio{Deploy: false, Image: "someimage"}}
	}
	return &dspav1.DataSciencePipelinesApplication{
		ObjectMeta: metav1.ObjectMeta{Name: ingressTestDSPAName, Namespace: ingressTestNamespace},
		Spec:       spec,
	}
}

func requireNetworkPolicy(t *testing.T, ctx context.Context, reconciler *DSPAReconciler, name string) *networkingv1.NetworkPolicy {
	t.Helper()
	policy := &networkingv1.NetworkPolicy{}
	created, err := reconciler.IsResourceCreated(ctx, policy, name, ingressTestNamespace)
	require.NoError(t, err)
	require.True(t, created)
	return policy
}

func assertNetworkPolicyAbsent(t *testing.T, ctx context.Context, reconciler *DSPAReconciler, name string) {
	t.Helper()
	policy := &networkingv1.NetworkPolicy{}
	created, err := reconciler.IsResourceCreated(ctx, policy, name, ingressTestNamespace)
	require.NoError(t, err)
	require.False(t, created)
}

func assertNoIngress(t *testing.T, policy *networkingv1.NetworkPolicy) {
	t.Helper()
	require.Empty(t, policy.Spec.Ingress)
}

func assertDriverRoleSelector(t *testing.T, policy *networkingv1.NetworkPolicy) {
	t.Helper()
	require.Empty(t, policy.Spec.PodSelector.MatchLabels)
	require.Len(t, policy.Spec.PodSelector.MatchExpressions, 1)
	expr := policy.Spec.PodSelector.MatchExpressions[0]
	require.Equal(t, "pipelines.kubeflow.org/pod-role", expr.Key)
	require.Equal(t, metav1.LabelSelectorOpIn, expr.Operator)
	require.Equal(t, []string{"container-driver", "dag-driver"}, expr.Values)
}

func assertPortPresent(t *testing.T, policy *networkingv1.NetworkPolicy, port int32) {
	t.Helper()
	require.Truef(t, policyHasPort(policy, port), "port %d not found", port)
}

func assertPortAbsent(t *testing.T, policy *networkingv1.NetworkPolicy, port int32) {
	t.Helper()
	require.Falsef(t, policyHasPort(policy, port), "port %d should be absent", port)
}

type ingressPeer struct {
	podLabels       map[string]string
	namespaceLabels map[string]string
}

func runtimeRolePeers() []ingressPeer {
	return []ingressPeer{
		{podLabels: map[string]string{"pipelines.kubeflow.org/pod-role": "container-driver"}},
		{podLabels: map[string]string{"pipelines.kubeflow.org/pod-role": "dag-driver"}},
		{podLabels: map[string]string{"pipelines.kubeflow.org/pod-role": "container-executor"}},
		{podLabels: map[string]string{"pipelines.kubeflow.org/pod-role": "importer"}},
	}
}

func apiServerHTTPPeers(name string) []ingressPeer {
	return append([]ingressPeer{{
		podLabels: map[string]string{
			"app":       "ds-pipeline-persistenceagent-" + name,
			"component": "data-science-pipelines",
		},
	}}, runtimeRolePeers()...)
}

func apiServerGRPCPeers(name string) []ingressPeer {
	peers := apiServerHTTPPeers(name)
	scheduledWorkflow := ingressPeer{podLabels: map[string]string{
		"app":       "ds-pipeline-scheduledworkflow-" + name,
		"component": "data-science-pipelines",
	}}
	return append([]ingressPeer{peers[0], scheduledWorkflow}, peers[1:]...)
}

func mlmdGRPCPeers(name string) []ingressPeer {
	return append([]ingressPeer{
		{podLabels: map[string]string{
			"app":       "ds-pipeline-" + name,
			"component": "data-science-pipelines",
		}},
		{podLabels: map[string]string{
			"app":       "ds-pipeline-metadata-envoy-" + name,
			"component": "data-science-pipelines",
		}},
	}, runtimeRolePeers()...)
}

func minioPeers(name, operatorNamespace string, externalRoute bool) []ingressPeer {
	peers := []ingressPeer{
		{podLabels: map[string]string{
			"app":       "ds-pipeline-" + name,
			"component": "data-science-pipelines",
		}},
		{podLabels: map[string]string{"pipelines.kubeflow.org/pod-role": "container-executor"}},
		{podLabels: map[string]string{"pipelines.kubeflow.org/pod-role": "importer"}},
		{
			podLabels:       map[string]string{"app.kubernetes.io/name": "data-science-pipelines-operator"},
			namespaceLabels: map[string]string{"kubernetes.io/metadata.name": operatorNamespace},
		},
	}
	if externalRoute {
		peers = append(peers, ingressPeer{
			namespaceLabels: map[string]string{"policy-group.network.openshift.io/ingress": ""},
		})
	}
	return peers
}

func monitoringNamespacePeers() []ingressPeer {
	namespaces := []string{
		"openshift-monitoring",
		"openshift-user-workload-monitoring",
		"redhat-ods-monitoring",
		"opendatahub-monitoring",
	}
	peers := make([]ingressPeer, 0, len(namespaces))
	for _, namespace := range namespaces {
		peers = append(peers, ingressPeer{
			namespaceLabels: map[string]string{"kubernetes.io/metadata.name": namespace},
		})
	}
	return peers
}

func assertIngressFrom(t *testing.T, policy *networkingv1.NetworkPolicy, port int32, expected []ingressPeer) {
	t.Helper()
	rule := ingressRuleForPort(t, policy, port)
	require.Len(t, rule.From, len(expected))
	for i, peer := range rule.From {
		require.Nil(t, peer.IPBlock)
		require.Equal(t, normalizeLabels(expected[i].podLabels), selectorLabels(peer.PodSelector))
		require.Equal(t, normalizeLabels(expected[i].namespaceLabels), selectorLabels(peer.NamespaceSelector))
		if peer.PodSelector != nil {
			require.Empty(t, peer.PodSelector.MatchExpressions)
		}
		if peer.NamespaceSelector != nil {
			require.Empty(t, peer.NamespaceSelector.MatchExpressions)
		}
	}
}

// assertDirectAPICallers requires the exact caller list and rejects monitoring
// namespaces, a pod selector that would include workbenches, and an empty from.
func assertDirectAPICallers(t *testing.T, policy *networkingv1.NetworkPolicy, port int32, expected []ingressPeer) {
	t.Helper()
	require.NotEmpty(t, expected)
	assertIngressFrom(t, policy, port, expected)
	rule := ingressRuleForPort(t, policy, port)
	require.NotEmpty(t, rule.From)
	for _, peer := range rule.From {
		require.Nil(t, peer.NamespaceSelector)
		require.NotNil(t, peer.PodSelector)
		require.NotEmpty(t, peer.PodSelector.MatchLabels)
	}
}

func ingressRuleForPort(t *testing.T, policy *networkingv1.NetworkPolicy, port int32) *networkingv1.NetworkPolicyIngressRule {
	t.Helper()
	var match *networkingv1.NetworkPolicyIngressRule
	for i := range policy.Spec.Ingress {
		rule := &policy.Spec.Ingress[i]
		require.NotEmpty(t, rule.Ports, "a portless ingress rule allows every port")
		for _, rulePort := range rule.Ports {
			if rulePort.Port != nil && rulePort.Port.Type == intstr.Int && rulePort.Port.IntVal == port {
				require.Nil(t, match, "multiple ingress rules allow port %d", port)
				match = rule
			}
		}
	}
	require.NotNil(t, match, "port %d not found", port)
	return match
}

func selectorLabels(selector *metav1.LabelSelector) map[string]string {
	if selector == nil {
		return nil
	}
	return normalizeLabels(selector.MatchLabels)
}

func normalizeLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func policyHasPort(policy *networkingv1.NetworkPolicy, port int32) bool {
	for _, rule := range policy.Spec.Ingress {
		if len(rule.Ports) == 0 {
			return true
		}
		for _, rulePort := range rule.Ports {
			if rulePort.Port != nil && rulePort.Port.Type == intstr.Int && rulePort.Port.IntVal == port {
				return true
			}
		}
	}
	return false
}

func policyHasIngressRouterPeer(policy *networkingv1.NetworkPolicy) bool {
	const routerLabel = "policy-group.network.openshift.io/ingress"
	for _, rule := range policy.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.NamespaceSelector == nil || peer.PodSelector == nil {
				continue
			}
			value, ok := peer.NamespaceSelector.MatchLabels[routerLabel]
			if ok && value == "" && len(peer.PodSelector.MatchLabels) == 0 && len(peer.PodSelector.MatchExpressions) == 0 {
				return true
			}
		}
	}
	return false
}
