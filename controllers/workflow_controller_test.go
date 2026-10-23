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

	aipipelinesv1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestAIPipelinesConfigOverridesLegacyArgoConfigInModularMode(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set(config.EnableAIPipelinesModuleControllerConfigName, true)
	viper.Set("DSPO.ArgoWorkflowsControllers", `{"managementState":"Managed"}`)

	ctx, _, reconciler := CreateNewTestObjects()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(reconciler.Scheme))
	require.NoError(t, reconciler.Create(ctx, &aipipelinesv1alpha1.AIPipelines{
		ObjectMeta: metav1.ObjectMeta{Name: aipipelinesv1alpha1.AIPipelinesInstanceName},
		Spec: aipipelinesv1alpha1.AIPipelinesSpec{ArgoWorkflowsControllers: &aipipelinesv1alpha1.ArgoWorkflowsControllersSpec{
			ManagementState: common.Removed,
		}},
	}))

	argoConfig, err := reconciler.argoWorkflowsControllersConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "Removed", argoConfig.GetManagementState())
}

func TestMissingAIPipelinesConfigPreservesWorkflowControllersInModularMode(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set(config.EnableAIPipelinesModuleControllerConfigName, true)
	viper.Set("DSPO.ArgoWorkflowsControllers", `{"managementState":"Removed"}`)

	ctx, _, reconciler := CreateNewTestObjects()
	require.NoError(t, aipipelinesv1alpha1.AddToScheme(reconciler.Scheme))

	argoConfig, err := reconciler.argoWorkflowsControllersConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, config.DefaultArgoWorkflowsControllersManagementState, argoConfig.GetManagementState())
}

func TestAIPipelinesChangeEnqueuesSupportedDSPAs(t *testing.T) {
	ctx, _, reconciler := CreateNewTestObjects()
	require.NoError(t, reconciler.Create(ctx, &dspav1.DataSciencePipelinesApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "supported", Namespace: "team-a"},
		Spec:       dspav1.DSPASpec{DSPVersion: config.DSPV2VersionString},
	}))
	require.NoError(t, reconciler.Create(ctx, &dspav1.DataSciencePipelinesApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "unsupported", Namespace: "team-b"},
		Spec:       dspav1.DSPASpec{DSPVersion: "v1"},
	}))

	requests := reconciler.enqueueAllSupportedDSPAs(ctx, nil)
	require.Len(t, requests, 1)
	require.Equal(t, types.NamespacedName{Name: "supported", Namespace: "team-a"}, requests[0].NamespacedName)
}

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
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(ctx, dspa, params)
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

	workflowPolicy := requireNetworkPolicy(t, ctx, reconciler, expectedWorkflowControllerName)
	assertPolicySelects(t, workflowPolicy, pipelineComponentLabels(expectedWorkflowControllerName))
	assertIngressFrom(t, workflowPolicy, 9090, monitoringNamespacePeers())
	assertPortAbsent(t, workflowPolicy, 6060)
	assertDNSEgress(t, workflowPolicy)
	assertKubernetesAPIEgress(t, workflowPolicy)
	require.Len(t, workflowPolicy.Spec.Egress, 2)
}

func TestWorkflowControllerAdditionalEgressRulesRender(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedWorkflowControllerName := "ds-pipeline-workflow-controller-testdspa"

	// Construct DSPASpec with a deployed WorkflowController and user-specified
	// additional egress rules: one port-only rule and one destination-only rule.
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(false),
			APIServer:   &dspav1.APIServer{},
			WorkflowController: &dspav1.WorkflowController{
				Deploy: true,
			},
			NetworkPolicy: &dspav1.NetworkPolicySpec{
				Egress: &dspav1.NetworkPolicyEgressSpec{
					WorkflowController: &dspav1.WorkloadEgressSpec{
						AdditionalRules: []networkingv1.NetworkPolicyEgressRule{
							tcpEgressRule(4242),
							{
								To: []networkingv1.NetworkPolicyPeer{{
									IPBlock: &networkingv1.IPBlock{CIDR: "203.0.113.0/24"},
								}},
							},
						},
					},
					// API server additions must not leak into the workflow controller policy.
					APIServer: &dspav1.WorkloadEgressSpec{
						AdditionalRules: []networkingv1.NetworkPolicyEgressRule{tcpEgressRule(5000)},
					},
				},
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
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Run test reconciliation
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(ctx, dspa, params)
	assert.Nil(t, err)
	assert.True(t, workflowControllerEnabled)

	workflowPolicy := requireNetworkPolicy(t, ctx, reconciler, expectedWorkflowControllerName)
	assertPolicySelects(t, workflowPolicy, pipelineComponentLabels(expectedWorkflowControllerName))
	assertIngressFrom(t, workflowPolicy, 9090, monitoringNamespacePeers())

	// Operator-generated defaults are kept: DNS and the Kubernetes API.
	assertEgressPortOnly(t, workflowPolicy, corev1.ProtocolUDP, 53)
	assertEgressPortOnly(t, workflowPolicy, corev1.ProtocolTCP, 53)
	assertEgressPortOnly(t, workflowPolicy, corev1.ProtocolUDP, 5353)
	assertEgressPortOnly(t, workflowPolicy, corev1.ProtocolTCP, 5353)
	assertEgressPortOnly(t, workflowPolicy, corev1.ProtocolTCP, 443)
	assertEgressPortOnly(t, workflowPolicy, corev1.ProtocolTCP, 6443)

	// The user-specified port-only rule is appended after the defaults.
	assertEgressPortOnly(t, workflowPolicy, corev1.ProtocolTCP, 4242)
	require.Len(t, workflowPolicy.Spec.Egress, 4)

	// The user-specified destination-only rule renders its IPBlock peer.
	ipBlockRule := workflowPolicy.Spec.Egress[3]
	require.Empty(t, ipBlockRule.Ports)
	require.Len(t, ipBlockRule.To, 1)
	require.NotNil(t, ipBlockRule.To[0].IPBlock)
	require.Equal(t, "203.0.113.0/24", ipBlockRule.To[0].IPBlock.CIDR)

	// API server additions do not leak into the workflow controller policy.
	for _, rule := range workflowPolicy.Spec.Egress {
		require.False(t, egressRuleHasPort(rule.Ports, corev1.ProtocolTCP, 5000),
			"api server additional egress port leaked into the workflow controller policy")
	}
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
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(ctx, dspa, params)
	assert.Nil(t, err)
	assert.False(t, workflowControllerEnabled)

	// Ensure WorkflowController Deployment still doesn't exist
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedWorkflowControllerName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
	assertNetworkPolicyAbsent(t, ctx, reconciler, expectedWorkflowControllerName)
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
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(ctx, dspa, params)
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
	workflowControllerEnabled, err = reconciler.ReconcileWorkflowController(ctx, dspa, params)
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
	workflowControllerEnabled, err = reconciler.ReconcileWorkflowController(ctx, dspa, params)
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
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(ctx, dspa, params)
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
	workflowControllerEnabled, err := reconciler.ReconcileWorkflowController(ctx, dspa, params)
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

	enabled, err := reconciler.ReconcileWorkflowController(ctx, dspa, params)
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
