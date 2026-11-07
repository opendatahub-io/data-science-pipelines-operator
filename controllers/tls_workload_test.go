//go:build test_all || test_unit

/*
Copyright 2026.

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
	"errors"
	"fmt"
	"strings"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestReconcile_PreservesDeploymentsOnTLSResolutionError verifies workload preservation:
// when TLS resolution fails, Deployments are NOT modified. Reconciler sets
// APIServerReady=False, requeues, and leaves existing Deployment specs unchanged.
func TestReconcile_PreservesDeploymentsOnTLSResolutionError(t *testing.T) {
	ctx := context.Background()

	// Use the factory function to set up reconciler properly
	_, _, reconciler := CreateNewTestObjects()
	reconciler.HasOpenShiftConfigAPI = true // Enable TLS profile watch

	dspaName := "test-dspa-tls-error"
	namespace := "testnamespace"

	// Create DSPA
	dspa := testutil.CreateEmptyDSPA()
	dspa.Name = dspaName
	dspa.Namespace = namespace
	dspa.Spec.DSPVersion = "v2" // Required version
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true}
	require.NoError(t, reconciler.Client.Create(ctx, dspa))

	// Create existing API-server Deployment (with known spec)
	apiServerDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("ds-pipeline-%s", dspaName),
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "ds-pipeline"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "ds-pipeline"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "api-server",
							Image: "api-server:v1",
							Args:  []string{"--port=8888"},
						},
					},
				},
			},
		},
	}
	require.NoError(t, reconciler.Client.Create(ctx, apiServerDep))

	// Create existing MLMD Envoy Deployment
	mlmdDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("ds-pipeline-metadata-envoy-%s", dspaName),
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "mlmd-envoy"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "mlmd-envoy"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "envoy",
							Image: "envoy:v1",
							Args:  []string{"--config=/etc/envoy/config.yaml"},
						},
					},
				},
			},
		},
	}
	require.NoError(t, reconciler.Client.Create(ctx, mlmdDep))

	// Create APIServer with invalid strict profile to trigger TLS error
	// (all unsupported ciphers + strict adherence cause ExtractParams to fail)
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers: []string{
							"ECDHE-INVALID-CIPHER-1",
							"ECDHE-INVALID-CIPHER-2",
						},
					},
				},
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	require.NoError(t, reconciler.Client.Create(ctx, apiServer))

	// Reconcile - should fail at ExtractParams (TLS resolution)
	result, err := reconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: dspaName, Namespace: namespace},
	})
	require.NoError(t, err)

	// Assert requeue behavior
	assert.True(t, result.Requeue, "should requeue when ExtractParams fails")
	wantRequeueAfter := config.GetDurationConfigWithDefault(config.RequeueTimeConfigName, config.DefaultRequeueTime)
	assert.Equal(t, wantRequeueAfter, result.RequeueAfter, "RequeueAfter should match configured value")

	// Assert DSPA status updated correctly
	updated := &dspav1.DataSciencePipelinesApplication{}
	require.NoError(t, reconciler.Client.Get(ctx, types.NamespacedName{Name: dspaName, Namespace: namespace}, updated))

	apiCond := getConditionByType(updated, config.APIServerReady)
	require.NotNil(t, apiCond)
	assert.Equal(t, metav1.ConditionFalse, apiCond.Status)
	assert.Equal(t, config.FailingToDeploy, apiCond.Reason)
	assert.Contains(t, apiCond.Message, "ciphers") // TLS error message

	// Assert Deployments were NOT modified (workload preservation check)
	apiServerCheck := &appsv1.Deployment{}
	require.NoError(t, reconciler.Client.Get(ctx, types.NamespacedName{Name: apiServerDep.Name, Namespace: namespace}, apiServerCheck))
	assert.Equal(t, apiServerDep.Spec, apiServerCheck.Spec, "API-server Deployment spec should be unchanged on TLS error")

	mlmdCheck := &appsv1.Deployment{}
	require.NoError(t, reconciler.Client.Get(ctx, types.NamespacedName{Name: mlmdDep.Name, Namespace: namespace}, mlmdCheck))
	assert.Equal(t, mlmdDep.Spec, mlmdCheck.Spec, "MLMD Envoy Deployment spec should be unchanged on TLS error")
}

func TestReconcile_APIServerInternalErrorReturnsRetryErrorAndPreservesDeployments(t *testing.T) {
	ctx := context.Background()
	_, _, reconciler := CreateNewTestObjects()
	reconciler.HasOpenShiftConfigAPI = true

	dspa := testutil.CreateEmptyDSPA()
	dspa.Name = "test-dspa-apiserver-internal-error"
	dspa.Namespace = "testnamespace"
	dspa.Spec.DSPVersion = "v2"
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true}

	apiServerDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "ds-pipeline-test-dspa-apiserver-internal-error", Namespace: dspa.Namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ds-pipeline"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ds-pipeline"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api-server", Image: "api-server:v1", Args: []string{"--port=8888"}}}},
			},
		},
	}
	mlmdDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "ds-pipeline-metadata-envoy-test-dspa-apiserver-internal-error", Namespace: dspa.Namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "mlmd-envoy"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "mlmd-envoy"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "envoy", Image: "envoy:v1", Args: []string{"--config=/etc/envoy/config.yaml"}}}},
			},
		},
	}

	reconciler.Client = fake.NewClientBuilder().
		WithScheme(reconciler.Scheme).
		WithObjects(dspa, apiServerDep, mlmdDep).
		WithStatusSubresource(&dspav1.DataSciencePipelinesApplication{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, underlying client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*configv1.APIServer); ok {
					return apierrors.NewInternalError(errors.New("etcd leader election"))
				}
				return underlying.Get(ctx, key, obj, opts...)
			},
		}).Build()
	reconciler.APIReader = reconciler.Client

	result, err := reconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: dspa.Name, Namespace: dspa.Namespace},
	})
	require.Error(t, err)
	assert.True(t, apierrors.IsInternalError(err), "internal API errors must be returned for controller-runtime retry: %v", err)
	assert.Empty(t, result)

	for _, deployment := range []*appsv1.Deployment{apiServerDep, mlmdDep} {
		current := &appsv1.Deployment{}
		require.NoError(t, reconciler.Client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, current))
		assert.Equal(t, deployment.Spec, current.Spec, "Deployment %s spec should be unchanged after transient APIServer read error", deployment.Name)
	}
}

// TestReconcile_AppliesTLSProfileAndAdherenceToBothDeployments fetches a live
// APIServer via fetchLiveTLSProfile (not static params), reconciles, and asserts
// both kube-rbac-proxy PodTemplates update when profile and adherence change.
func TestReconcile_AppliesTLSProfileAndAdherenceToBothDeployments(t *testing.T) {
	ctx, _, reconciler := CreateNewTestObjects()
	reconciler.HasOpenShiftConfigAPI = true

	dspa := testutil.CreateEmptyDSPA()
	dspa.Name = "tls-apply"
	dspa.Namespace = "testnamespace"
	dspa.Spec.DSPVersion = "v2"
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true, EnableRoute: true}
	dspa.Spec.MLMD = &dspav1.MLMD{Deploy: true}
	dspa.Spec.Database.DisableHealthCheck = true
	dspa.Spec.ObjectStorage.DisableHealthCheck = true
	dspa.Annotations = map[string]string{dspav1.DisableManagedPipelinesAnnotation: "true"}
	require.NoError(t, reconciler.Client.Create(ctx, dspa))

	const intermediateOnlyCipher = "TLS_CHACHA20_POLY1305_SHA256"
	const customCipher = "ECDHE-RSA-AES128-GCM-SHA256"

	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{customCipher},
					},
				},
			},
		},
	}
	require.NoError(t, reconciler.Client.Create(ctx, apiServer))

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: dspa.Name, Namespace: dspa.Namespace}}
	_, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	apiName := fmt.Sprintf("ds-pipeline-%s", dspa.Name)
	mlmdName := fmt.Sprintf("ds-pipeline-metadata-envoy-%s", dspa.Name)

	assert.True(t, argsContain(kubeRBACProxyArgs(t, ctx, reconciler, apiName, dspa.Namespace), intermediateOnlyCipher),
		"API-server kube-rbac-proxy should use Intermediate ciphers under NoOpinion, ignoring the Custom profile")
	assert.True(t, argsContain(kubeRBACProxyArgs(t, ctx, reconciler, mlmdName, dspa.Namespace), intermediateOnlyCipher),
		"MLMD Envoy kube-rbac-proxy should use Intermediate ciphers under NoOpinion, ignoring the Custom profile")
	assertOperandFlags(t, ctx, reconciler, apiName, dspa.Namespace, "ds-pipeline-api-server", "", "",
		"operand must keep serving defaults under NoOpinion")

	require.NoError(t, reconciler.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, apiServer))
	apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
	require.NoError(t, reconciler.Client.Update(ctx, apiServer))

	_, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	apiArgs := kubeRBACProxyArgs(t, ctx, reconciler, apiName, dspa.Namespace)
	mlmdArgs := kubeRBACProxyArgs(t, ctx, reconciler, mlmdName, dspa.Namespace)
	assert.True(t, argsHavePrefix(apiArgs, "--tls-cipher-suites") && !argsContain(apiArgs, intermediateOnlyCipher),
		"API-server should apply the Custom cipher list under StrictAllComponents")
	assert.True(t, argsHavePrefix(mlmdArgs, "--tls-cipher-suites") && !argsContain(mlmdArgs, intermediateOnlyCipher),
		"MLMD Envoy should apply the Custom cipher list under StrictAllComponents")
	assertOperandFlags(t, ctx, reconciler, apiName, dspa.Namespace, "ds-pipeline-api-server",
		"VersionTLS12", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		"operand should apply the Custom profile under StrictAllComponents")

	require.NoError(t, reconciler.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, apiServer))
	apiServer.Spec.TLSSecurityProfile = &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
	require.NoError(t, reconciler.Client.Update(ctx, apiServer))

	_, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	apiArgs = kubeRBACProxyArgs(t, ctx, reconciler, apiName, dspa.Namespace)
	mlmdArgs = kubeRBACProxyArgs(t, ctx, reconciler, mlmdName, dspa.Namespace)
	assert.True(t, argsHavePrefix(apiArgs, "--tls-min-version=VersionTLS13") && !argsHavePrefix(apiArgs, "--tls-cipher-suites"),
		"API-server should use TLS 1.3 with no cipher flag after Modern profile")
	assert.True(t, argsHavePrefix(mlmdArgs, "--tls-min-version=VersionTLS13") && !argsHavePrefix(mlmdArgs, "--tls-cipher-suites"),
		"MLMD Envoy should use TLS 1.3 with no cipher flag after Modern profile")
	assertOperandFlags(t, ctx, reconciler, apiName, dspa.Namespace, "ds-pipeline-api-server",
		"VersionTLS13", "", "operand should use TLS 1.3 with no cipher flag after Modern profile")
}

// assertOperandFlags asserts the operand container's --tlsMinVersion and
// --tlsCipherSuites args. The webhook deployment's operand args are covered
// by the template tests in tls_resolve_test.go; it only deploys under
// PipelineStore=kubernetes with the DSPO operator Deployment present. Empty wantMinVersion means both flags must be absent;
// empty wantCiphers means the cipher flag must be absent.
func assertOperandFlags(t *testing.T, ctx context.Context, r *DSPAReconciler, name, namespace, container, wantMinVersion, wantCiphers, msg string) {
	t.Helper()
	dep := &appsv1.Deployment{}
	require.NoError(t, r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, dep), msg)
	var args []string
	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == container {
			args = c.Args
		}
	}
	require.NotNil(t, args, "container %q not found in Deployment %s: %s", container, name, msg)
	if wantMinVersion == "" {
		assert.False(t, argsHavePrefix(args, "--tlsMinVersion"), "%s: operand must not have --tlsMinVersion, got %v", msg, args)
		assert.False(t, argsHavePrefix(args, "--tlsCipherSuites"), "%s: operand must not have --tlsCipherSuites, got %v", msg, args)
		return
	}
	assert.True(t, argsHavePrefix(args, "--tlsMinVersion="+wantMinVersion), "%s: operand must have --tlsMinVersion=%s, got %v", msg, wantMinVersion, args)
	if wantCiphers == "" {
		assert.False(t, argsHavePrefix(args, "--tlsCipherSuites"), "%s: operand must not have --tlsCipherSuites, got %v", msg, args)
		return
	}
	assert.True(t, argsHavePrefix(args, "--tlsCipherSuites="+wantCiphers), "%s: operand must have --tlsCipherSuites=%s, got %v", msg, wantCiphers, args)
}

func kubeRBACProxyArgs(t *testing.T, ctx context.Context, r *DSPAReconciler, name, namespace string) []string {
	t.Helper()
	dep := &appsv1.Deployment{}
	require.NoError(t, r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, dep))
	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == "kube-rbac-proxy" {
			return c.Args
		}
	}
	t.Fatalf("kube-rbac-proxy container not found in Deployment %s", name)
	return nil
}

func argsHavePrefix(args []string, prefix string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

func argsContain(args []string, substr string) bool {
	for _, a := range args {
		if strings.Contains(a, substr) {
			return true
		}
	}
	return false
}

// pointerToInt32 is a helper to create a pointer to int32
func pointerToInt32(i int32) *int32 {
	return &i
}

// getConditionByType retrieves a condition from DSPA status by type
func getConditionByType(dspa *dspav1.DataSciencePipelinesApplication, condType string) *metav1.Condition {
	for i := range dspa.Status.Conditions {
		if dspa.Status.Conditions[i].Type == condType {
			return &dspa.Status.Conditions[i]
		}
	}
	return nil
}

// TestReconcile_PreservesDeploymentsOnOperandTLSResolutionError verifies the
// operand-only fail-closed path: a Strict profile the operand cannot represent
// (Old, TLS 1.0) with ciphers the kube-rbac-proxy resolver accepts must abort
// the reconcile at the operand resolver and leave existing Deployments
// unchanged, rather than downgrade the minimum version.
func TestReconcile_PreservesDeploymentsOnOperandTLSResolutionError(t *testing.T) {
	ctx := context.Background()

	_, _, reconciler := CreateNewTestObjects()
	reconciler.HasOpenShiftConfigAPI = true

	dspaName := "test-dspa-operand-tls-error"
	namespace := "testnamespace"

	dspa := testutil.CreateEmptyDSPA()
	dspa.Name = dspaName
	dspa.Namespace = namespace
	dspa.Spec.DSPVersion = "v2"
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true}
	require.NoError(t, reconciler.Client.Create(ctx, dspa))

	apiServerDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("ds-pipeline-%s", dspaName),
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "ds-pipeline"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "ds-pipeline"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "api-server",
							Image: "api-server:v1",
							Args:  []string{"--port=8888"},
						},
					},
				},
			},
		},
	}
	require.NoError(t, reconciler.Client.Create(ctx, apiServerDep))

	// Old profile (TLS 1.0) with a cipher valid for kube-rbac-proxy, so the
	// sidecar resolver succeeds and the failure comes from the operand-only
	// minimum-version validation.
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					MinTLSVersion: configv1.VersionTLS10,
				}},
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	require.NoError(t, reconciler.Client.Create(ctx, apiServer))

	result, err := reconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: dspaName, Namespace: namespace},
	})
	require.NoError(t, err)
	assert.True(t, result.Requeue, "should requeue when operand TLS resolution fails")

	updated := &dspav1.DataSciencePipelinesApplication{}
	require.NoError(t, reconciler.Client.Get(ctx, types.NamespacedName{Name: dspaName, Namespace: namespace}, updated))
	apiCond := getConditionByType(updated, config.APIServerReady)
	require.NotNil(t, apiCond)
	assert.Equal(t, metav1.ConditionFalse, apiCond.Status)
	assert.Contains(t, apiCond.Message, "does not support minimum TLS version")

	apiServerCheck := &appsv1.Deployment{}
	require.NoError(t, reconciler.Client.Get(ctx, types.NamespacedName{Name: apiServerDep.Name, Namespace: namespace}, apiServerCheck))
	assert.Equal(t, apiServerDep.Spec, apiServerCheck.Spec, "API-server Deployment spec should be unchanged on operand TLS error")
}
