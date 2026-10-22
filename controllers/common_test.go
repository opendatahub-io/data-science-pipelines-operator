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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	assertPolicySelects(t, np, pipelineComponentLabels("ds-pipeline-"+testDSPAName))
	assertDirectAPICallers(t, np, 8888, apiServerHTTPPeers(testDSPAName))
	assertDirectAPICallers(t, np, 8887, apiServerGRPCPeers(testDSPAName))
	assertDefaultAPIServerEgress(t, np, testDSPAName)
	require.Len(t, np.Spec.Egress, 5)

	np = &networkingv1.NetworkPolicy{}
	created, err = reconciler.IsResourceCreated(ctx, np, expectedEnvoyNetworkPolicyName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
	// MLMD is omitted and defaults to deploying the Envoy route.
	assertPolicySelects(t, np, pipelineComponentLabels("ds-pipeline-metadata-envoy-"+testDSPAName))
	assertPortPresent(t, np, 8443)
	assertIngressFrom(t, np, 9090, envoyListenerPeers(testDSPAName))
	assertPortAbsent(t, np, 9901)
	assertEnvoyEgress(t, np, testDSPAName, true)

	driverPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipeline-drivers-"+testDSPAName)
	assertNoIngress(t, driverPolicy)
	assertDriverRoleSelector(t, driverPolicy)
	assertDefaultDriverEgress(t, driverPolicy, testDSPAName)
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
	assertPolicySelects(t, apiPolicy, pipelineComponentLabels("ds-pipeline-"+ingressTestDSPAName))
	assertPortPresent(t, apiPolicy, 8443)
	assertIngressFrom(t, apiPolicy, 8445, monitoringNamespacePeers())
	assertDirectAPICallers(t, apiPolicy, 8888, apiServerHTTPPeers(ingressTestDSPAName))
	assertDirectAPICallers(t, apiPolicy, 8887, apiServerGRPCPeers(ingressTestDSPAName))
	assertDefaultAPIServerEgress(t, apiPolicy, ingressTestDSPAName)
	require.Len(t, apiPolicy.Spec.Egress, 5)
	envoyPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-envoy-"+ingressTestDSPAName)
	assertPolicySelects(t, envoyPolicy, pipelineComponentLabels("ds-pipeline-metadata-envoy-"+ingressTestDSPAName))
	assertPortAbsent(t, envoyPolicy, 8443)
	assertIngressFrom(t, envoyPolicy, 9090, envoyListenerPeers(ingressTestDSPAName))
	assertPortAbsent(t, envoyPolicy, 9901)
	assertEnvoyEgress(t, envoyPolicy, ingressTestDSPAName, false)
}

func TestCommonPoliciesKeepMetricsPortWithoutRoute(t *testing.T) {
	dspa := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{EnableRoute: false},
	})

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileCommon(dspa, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertPolicySelects(t, apiPolicy, pipelineComponentLabels("ds-pipeline-"+ingressTestDSPAName))
	assertPortAbsent(t, apiPolicy, 8443)
	assertIngressFrom(t, apiPolicy, 8445, monitoringNamespacePeers())
	assertDefaultAPIServerEgress(t, apiPolicy, ingressTestDSPAName)
	require.Len(t, apiPolicy.Spec.Egress, 5)
}

func TestAPIServerAdditionalEgressKeepsDefaults(t *testing.T) {
	additional := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{},
		NetworkPolicy: &dspav1.NetworkPolicySpec{
			Egress: &dspav1.NetworkPolicyEgressSpec{
				APIServer: &dspav1.WorkloadEgressSpec{
					AdditionalRules: []networkingv1.NetworkPolicyEgressRule{tcpEgressRule(10)},
				},
			},
		},
	})

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, additional, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileCommon(additional, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertDefaultAPIServerEgress(t, apiPolicy, ingressTestDSPAName)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 10)
	require.Len(t, apiPolicy.Spec.Egress, 6)

	driverPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipeline-drivers-"+ingressTestDSPAName)
	assertDefaultDriverEgress(t, driverPolicy, ingressTestDSPAName)
	assertEgressPortAbsent(t, driverPolicy, corev1.ProtocolTCP, 10)

	proxied := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{},
		Proxy:     &dspav1.ProxyConfig{HTTPProxy: "http://proxy.example.com:3128"},
		WorkflowController: &dspav1.WorkflowController{
			Deploy: true,
		},
	})
	ctx, params, reconciler = CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, proxied, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileCommon(proxied, params))
	_, err := reconciler.ReconcileWorkflowController(ctx, proxied, params)
	require.NoError(t, err)

	apiPolicy = requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertDefaultAPIServerEgress(t, apiPolicy, ingressTestDSPAName)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 3128)
	require.Len(t, apiPolicy.Spec.Egress, 6)

	driverPolicy = requireNetworkPolicy(t, ctx, reconciler, "ds-pipeline-drivers-"+ingressTestDSPAName)
	assertDefaultDriverEgress(t, driverPolicy, ingressTestDSPAName)
	assertEgressPortAbsent(t, driverPolicy, corev1.ProtocolTCP, 3128)

	workflowPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipeline-workflow-controller-"+ingressTestDSPAName)
	assertDNSEgress(t, workflowPolicy)
	assertKubernetesAPIEgress(t, workflowPolicy)
	assertEgressPortOnly(t, workflowPolicy, corev1.ProtocolTCP, 3128)
	require.Len(t, workflowPolicy.Spec.Egress, 3)
}

func TestAPIServerEgressUsesPortEmbeddedInS3Host(t *testing.T) {
	embedded := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{},
		ObjectStorage: &dspav1.ObjectStorage{
			ExternalStorage: &dspav1.ExternalStorage{
				Host:            "s3.example.com:9001",
				Bucket:          "mlpipeline",
				Scheme:          "http",
				CredentialsMode: &dspav1.CredentialsMode{FromEnv: true},
			},
		},
	})

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, embedded, reconciler.Client, reconciler.Log))
	require.Empty(t, params.ObjectStorageConnection.Port)
	require.Equal(t, "http://s3.example.com:9001", params.ObjectStorageConnection.Endpoint)
	require.Equal(t, "9001", params.ObjectStorageEgressPort)
	require.NoError(t, reconciler.ReconcileCommon(embedded, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 9001)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 9000)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 8333)
	assertEgressPortAbsent(t, apiPolicy, corev1.ProtocolTCP, 80)
}

func TestAPIServerEgressIncludesCommonS3PortsWhenFromEnvHasNoPort(t *testing.T) {
	fromEnv := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{},
		ObjectStorage: &dspav1.ObjectStorage{
			ExternalStorage: &dspav1.ExternalStorage{
				Host:            "s3.amazonaws.com",
				Bucket:          "mlpipeline",
				Scheme:          "https",
				CredentialsMode: &dspav1.CredentialsMode{FromEnv: true},
			},
		},
	})

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, fromEnv, reconciler.Client, reconciler.Log))
	require.Equal(t, "443", params.ObjectStorageEgressPort)
	require.NoError(t, reconciler.ReconcileCommon(fromEnv, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 443)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 9000)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 8333)
}

// An explicit secure=false overrides scheme https for the S3 client, so the default
// port is 80, not 443. The policy must allow the port the client actually dials.
func TestAPIServerEgressUsesInsecureStorageDefaultPort(t *testing.T) {
	insecure := false
	insecureStorage := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{},
		ObjectStorage: &dspav1.ObjectStorage{
			ExternalStorage: &dspav1.ExternalStorage{
				Host:            "s3.example.com",
				Bucket:          "mlpipeline",
				Scheme:          "https",
				Secure:          &insecure,
				CredentialsMode: &dspav1.CredentialsMode{FromEnv: true},
			},
		},
	})

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, insecureStorage, reconciler.Client, reconciler.Log))
	require.Equal(t, "80", params.ObjectStorageEgressPort)
	require.NoError(t, reconciler.ReconcileCommon(insecureStorage, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 80)
	assertDefaultAPIServerEgress(t, apiPolicy, ingressTestDSPAName)
}

func TestExternalDatabaseEgressUsesPortOnly(t *testing.T) {
	dspa := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{},
		MLMD:      &dspav1.MLMD{Deploy: true},
	})
	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	params.BuiltInDatabase = false
	params.DBConnection.Port = "3307"
	require.NoError(t, reconciler.ReconcileCommon(dspa, params))
	require.NoError(t, reconciler.ReconcileMLMD(ctx, dspa, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	assertEgressPortOnly(t, apiPolicy, corev1.ProtocolTCP, 3307)
	assertNoPortOnlyEgress(t, apiPolicy, corev1.ProtocolTCP, 3306)

	mlmdPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipeline-metadata-grpc-"+ingressTestDSPAName)
	assertDNSEgress(t, mlmdPolicy)
	assertEgressPortOnly(t, mlmdPolicy, corev1.ProtocolTCP, 3307)
	assertNoPortOnlyEgress(t, mlmdPolicy, corev1.ProtocolTCP, 3306)
	require.Len(t, mlmdPolicy.Spec.Egress, 2)
}

func TestMLflowEgressScopedToApplicationsNamespace(t *testing.T) {
	for _, applicationsNamespace := range []string{"opendatahub", "redhat-ods-applications"} {
		t.Run(applicationsNamespace, func(t *testing.T) {
			dspa := newIngressTestDSPA(dspav1.DSPASpec{APIServer: &dspav1.APIServer{}})
			ctx, params, reconciler := CreateNewTestObjects()
			require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
			params.ApplicationsNamespace = applicationsNamespace
			params.MLflowEgressPort = "8443"
			require.NoError(t, reconciler.ReconcileCommon(dspa, params))

			apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
			driverPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipeline-drivers-"+ingressTestDSPAName)
			for _, policy := range []*networkingv1.NetworkPolicy{apiPolicy, driverPolicy} {
				assertEgressToNamespacedPod(t, policy, 8443, applicationsNamespace, "mlflow")
				assertNoPortOnlyEgress(t, policy, corev1.ProtocolTCP, 8443)
			}
			require.Len(t, apiPolicy.Spec.Egress, 6)
			require.Len(t, driverPolicy.Spec.Egress, 5)
		})
	}
}

func TestMLflowEgressOmittedWithoutApplicationsNamespace(t *testing.T) {
	dspa := newIngressTestDSPA(dspav1.DSPASpec{APIServer: &dspav1.APIServer{}})
	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	params.ApplicationsNamespace = ""
	params.MLflowEgressPort = "8443"
	require.NoError(t, reconciler.ReconcileCommon(dspa, params))

	apiPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	driverPolicy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipeline-drivers-"+ingressTestDSPAName)
	assertEgressPortAbsent(t, apiPolicy, corev1.ProtocolTCP, 8443)
	assertEgressPortAbsent(t, driverPolicy, corev1.ProtocolTCP, 8443)
	assertDefaultAPIServerEgress(t, apiPolicy, ingressTestDSPAName)
	assertDefaultDriverEgress(t, driverPolicy, ingressTestDSPAName)
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

func pipelineComponentLabels(app string) map[string]string {
	return map[string]string{
		"app":       app,
		"component": "data-science-pipelines",
	}
}

func assertPolicySelects(t *testing.T, policy *networkingv1.NetworkPolicy, labels map[string]string) {
	t.Helper()
	require.Equal(t, labels, policy.Spec.PodSelector.MatchLabels)
	require.Empty(t, policy.Spec.PodSelector.MatchExpressions)
}

func assertPortOnlyTCP(t *testing.T, policy *networkingv1.NetworkPolicy, port int32) {
	t.Helper()
	require.Len(t, policy.Spec.Ingress, 1)
	rule := policy.Spec.Ingress[0]
	require.Empty(t, rule.From)
	require.Len(t, rule.Ports, 1)
	require.NotNil(t, rule.Ports[0].Protocol)
	require.Equal(t, corev1.ProtocolTCP, *rule.Ports[0].Protocol)
	require.NotNil(t, rule.Ports[0].Port)
	require.Equal(t, intstr.Int, rule.Ports[0].Port.Type)
	require.Equal(t, port, rule.Ports[0].Port.IntVal)
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

// runtimeRolePeers is the shared KFP runtime allow-list. v2_component is on
// both old in-flight pods and new compiles (drivers and executors).
func runtimeRolePeers() []ingressPeer {
	return []ingressPeer{
		{podLabels: map[string]string{"pipelines.kubeflow.org/v2_component": "true"}},
	}
}

func TestRuntimeRolePeersMatchInFlightV2ComponentPods(t *testing.T) {
	require.Equal(t, []ingressPeer{{
		podLabels: map[string]string{"pipelines.kubeflow.org/v2_component": "true"},
	}}, runtimeRolePeers())
}

func envoyListenerPeers(name string) []ingressPeer {
	return append([]ingressPeer{{
		podLabels: pipelineComponentLabels("ds-pipeline-" + name),
	}}, runtimeRolePeers()...)
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
		{podLabels: map[string]string{"pipelines.kubeflow.org/v2_component": "true"}},
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
			require.NotNilf(t, rulePort.Port,
				"port entry with no port number allows all %v traffic", rulePort.Protocol)
			require.Equalf(t, intstr.Int, rulePort.Port.Type,
				"named port %q cannot be verified; use an integer port", rulePort.Port.StrVal)
			start := rulePort.Port.IntVal
			end := start
			if rulePort.EndPort != nil {
				end = *rulePort.EndPort
			}
			if port >= start && port <= end {
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

func assertControlsIngressAndEgress(t *testing.T, policy *networkingv1.NetworkPolicy) {
	t.Helper()
	require.ElementsMatch(t, []networkingv1.PolicyType{
		networkingv1.PolicyTypeIngress,
		networkingv1.PolicyTypeEgress,
	}, policy.Spec.PolicyTypes)
}

func assertDeniesAllEgress(t *testing.T, policy *networkingv1.NetworkPolicy) {
	t.Helper()
	assertControlsIngressAndEgress(t, policy)
	require.Empty(t, policy.Spec.Egress)
}

func assertDNSEgress(t *testing.T, policy *networkingv1.NetworkPolicy) {
	t.Helper()
	assertControlsIngressAndEgress(t, policy)
	assertEgressPortOnly(t, policy, corev1.ProtocolUDP, 53)
	assertEgressPortOnly(t, policy, corev1.ProtocolTCP, 53)
	assertEgressPortOnly(t, policy, corev1.ProtocolUDP, 5353)
	assertEgressPortOnly(t, policy, corev1.ProtocolTCP, 5353)
	assertEgressPortAbsent(t, policy, corev1.ProtocolTCP, 5432)
}

func assertKubernetesAPIEgress(t *testing.T, policy *networkingv1.NetworkPolicy) {
	t.Helper()
	assertEgressPortOnly(t, policy, corev1.ProtocolTCP, 443)
	assertEgressPortOnly(t, policy, corev1.ProtocolTCP, 6443)
}

func assertDefaultAPIServerEgress(t *testing.T, policy *networkingv1.NetworkPolicy, name string) {
	t.Helper()
	assertDNSEgress(t, policy)
	assertKubernetesAPIEgress(t, policy)
	assertEgressToPod(t, policy, 3306, pipelineComponentLabels("mariadb-"+name))
	assertNoPortOnlyEgress(t, policy, corev1.ProtocolTCP, 3306)
	assertEgressPortOnly(t, policy, corev1.ProtocolTCP, 9000)
	assertEgressPortOnly(t, policy, corev1.ProtocolTCP, 8333)
	assertEgressToPod(t, policy, 8080, pipelineComponentLabels("ds-pipeline-metadata-grpc-"+name))
}

func assertDefaultDriverEgress(t *testing.T, policy *networkingv1.NetworkPolicy, name string) {
	t.Helper()
	assertDNSEgress(t, policy)
	assertKubernetesAPIEgress(t, policy)
	apiServerLabels := pipelineComponentLabels("ds-pipeline-" + name)
	assertEgressToPod(t, policy, 8888, apiServerLabels)
	assertEgressToPod(t, policy, 8887, apiServerLabels)
	assertEgressToPod(t, policy, 8080, pipelineComponentLabels("ds-pipeline-metadata-grpc-"+name))
	assertEgressPortAbsent(t, policy, corev1.ProtocolTCP, 3306)
	require.Len(t, policy.Spec.Egress, 4)
}

func assertEnvoyEgress(t *testing.T, policy *networkingv1.NetworkPolicy, name string, allowKubernetesAPI bool) {
	t.Helper()
	assertDNSEgress(t, policy)
	assertEgressToPod(t, policy, 8080, pipelineComponentLabels("ds-pipeline-metadata-grpc-"+name))
	if allowKubernetesAPI {
		assertKubernetesAPIEgress(t, policy)
		require.Len(t, policy.Spec.Egress, 3)
		return
	}
	assertEgressPortAbsent(t, policy, corev1.ProtocolTCP, 443)
	assertEgressPortAbsent(t, policy, corev1.ProtocolTCP, 6443)
	require.Len(t, policy.Spec.Egress, 2)
}

func assertBuiltInMLMDEgress(t *testing.T, policy *networkingv1.NetworkPolicy, name string) {
	t.Helper()
	assertDNSEgress(t, policy)
	assertEgressToPod(t, policy, 3306, pipelineComponentLabels("mariadb-"+name))
	assertNoPortOnlyEgress(t, policy, corev1.ProtocolTCP, 3306)
	require.Len(t, policy.Spec.Egress, 2)
}

func assertEgressPortOnly(t *testing.T, policy *networkingv1.NetworkPolicy, protocol corev1.Protocol, port int32) {
	t.Helper()
	for _, rule := range policy.Spec.Egress {
		if egressRuleHasPort(rule.Ports, protocol, port) && len(rule.To) == 0 {
			return
		}
	}
	require.Failf(t, "missing egress port", "%s/%d with no destination not found", protocol, port)
}

func assertEgressPortAbsent(t *testing.T, policy *networkingv1.NetworkPolicy, protocol corev1.Protocol, port int32) {
	t.Helper()
	for _, rule := range policy.Spec.Egress {
		if len(rule.Ports) == 0 {
			require.Failf(t, "unexpected unrestricted egress", "an unrestricted rule allows %s/%d", protocol, port)
			return
		}
		if egressRuleHasPort(rule.Ports, protocol, port) {
			require.Failf(t, "unexpected egress port", "%s/%d should be absent", protocol, port)
		}
	}
}

func assertEgressToPod(t *testing.T, policy *networkingv1.NetworkPolicy, port int32, labels map[string]string) {
	t.Helper()
	for _, rule := range policy.Spec.Egress {
		if !egressRuleHasPort(rule.Ports, corev1.ProtocolTCP, port) {
			continue
		}
		for _, peer := range rule.To {
			if peer.PodSelector == nil || peer.NamespaceSelector != nil || peer.IPBlock != nil {
				continue
			}
			if labelsMatch(peer.PodSelector.MatchLabels, labels) {
				require.Empty(t, peer.PodSelector.MatchExpressions)
				return
			}
		}
	}
	require.Failf(t, "missing egress peer", "port %d to %v not found", port, labels)
}

func assertEgressToNamespacedPod(t *testing.T, policy *networkingv1.NetworkPolicy, port int32, namespace, app string) {
	t.Helper()
	for _, rule := range policy.Spec.Egress {
		if !egressRuleHasPort(rule.Ports, corev1.ProtocolTCP, port) {
			continue
		}
		for _, peer := range rule.To {
			if peer.NamespaceSelector == nil || peer.PodSelector == nil {
				continue
			}
			if peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == namespace &&
				peer.PodSelector.MatchLabels["app"] == app {
				require.Empty(t, peer.NamespaceSelector.MatchExpressions)
				require.Empty(t, peer.PodSelector.MatchExpressions)
				return
			}
		}
	}
	require.Failf(t, "missing namespaced egress peer", "port %d to app=%s in namespace %s not found", port, app, namespace)
}

func assertNoPortOnlyEgress(t *testing.T, policy *networkingv1.NetworkPolicy, protocol corev1.Protocol, port int32) {
	t.Helper()
	for _, rule := range policy.Spec.Egress {
		if egressRuleHasPort(rule.Ports, protocol, port) && len(rule.To) == 0 {
			require.Failf(t, "unexpected unrestricted egress", "%s/%d has no destination", protocol, port)
		}
	}
}

func egressRuleHasPort(ports []networkingv1.NetworkPolicyPort, protocol corev1.Protocol, port int32) bool {
	for _, rulePort := range ports {
		if rulePort.Protocol == nil || *rulePort.Protocol != protocol || rulePort.Port == nil {
			continue
		}
		if rulePort.Port.Type == intstr.Int && rulePort.Port.IntVal == port {
			return true
		}
	}
	return false
}

func labelsMatch(actual, expected map[string]string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

// TestNoUnrestrictedDirectAPIAccess fails if any operator-rendered ingress rule
// for the unauthenticated ports 8888/8887 has an empty or unlabeled From.
// All policies selecting the API server pods are checked because NP rules are additive.
func TestNoUnrestrictedDirectAPIAccess(t *testing.T) {
	dspa := newIngressTestDSPA(dspav1.DSPASpec{
		APIServer: &dspav1.APIServer{EnableRoute: true},
	})

	ctx, params, reconciler := CreateNewTestObjects()
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NoError(t, reconciler.ReconcileCommon(dspa, params))

	// NP rules are additive — check every operator-rendered policy selecting the API
	// server pods to guard against a future template accidentally opening these ports.
	allPolicies := &networkingv1.NetworkPolicyList{}
	require.NoError(t, reconciler.Client.List(ctx, allPolicies,
		client.InNamespace(ingressTestNamespace)))

	apiServerLabels := labels.Set(pipelineComponentLabels("ds-pipeline-" + ingressTestDSPAName))
	var apiPolicies []*networkingv1.NetworkPolicy
	for i := range allPolicies.Items {
		p := &allPolicies.Items[i]
		// Skip policies that do not enforce Ingress.
		enforcesIngress := false
		for _, pt := range p.Spec.PolicyTypes {
			if pt == networkingv1.PolicyTypeIngress {
				enforcesIngress = true
				break
			}
		}
		if !enforcesIngress {
			continue
		}
		sel, err := metav1.LabelSelectorAsSelector(&p.Spec.PodSelector)
		require.NoError(t, err)
		if sel.Matches(apiServerLabels) {
			apiPolicies = append(apiPolicies, p)
		}
	}
	require.NotEmpty(t, apiPolicies, "no NetworkPolicy found selecting the API server pods")

	for _, policy := range apiPolicies {
		for _, rule := range policy.Spec.Ingress {
			// A portless rule allows every port including 8888/8887.
			require.NotEmptyf(t, rule.Ports,
				"policy %s: portless ingress rule allows all ports", policy.Name)
			for _, rulePort := range rule.Ports {
				// A port entry with no port number allows all ports of that protocol.
				require.NotNilf(t, rulePort.Port,
					"policy %s: port entry with no port number allows all %v traffic", policy.Name, rulePort.Protocol)
				// Named ports cannot be verified against 8888/8887.
				require.Equalf(t, intstr.Int, rulePort.Port.Type,
					"policy %s: named port %q cannot be verified; use an integer port", policy.Name, rulePort.Port.StrVal)
				start := rulePort.Port.IntVal
				end := start
				if rulePort.EndPort != nil {
					end = *rulePort.EndPort
				}
				// Only assert From restrictions for rules that cover 8888 or 8887.
				coversDirectAPI := (start <= 8888 && 8888 <= end) || (start <= 8887 && 8887 <= end)
				if !coversDirectAPI {
					continue
				}
				require.NotEmptyf(t, rule.From,
					"policy %s port %d-%d: empty From grants access to all pods", policy.Name, start, end)
				for _, peer := range rule.From {
					require.Nilf(t, peer.NamespaceSelector,
						"policy %s: namespace-scoped peer grants access to entire namespaces", policy.Name)
					require.NotNilf(t, peer.PodSelector,
						"policy %s: peer with no PodSelector is an unrestricted allow", policy.Name)
					require.NotEmptyf(t, peer.PodSelector.MatchLabels,
						"policy %s: PodSelector with no labels matches all pods", policy.Name)
					require.Emptyf(t, peer.PodSelector.MatchExpressions,
						"policy %s: PodSelector matchExpressions cannot be verified", policy.Name)
				}
			}
		}
	}

	// 8443 is intentionally open — kube-rbac-proxy enforces auth before forwarding.
	policy := requireNetworkPolicy(t, ctx, reconciler, "ds-pipelines-"+ingressTestDSPAName)
	rule8443 := ingressRuleForPort(t, policy, 8443)
	require.Emptyf(t, rule8443.From,
		"port 8443: kube-rbac-proxy handles auth; NP openness is intentional")
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
