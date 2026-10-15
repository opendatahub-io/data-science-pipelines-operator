//go:build test_all || test_unit

package controllers

import (
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func proxyTLSProfileTestDSPA(podToPodTLS bool) *dspav1.DataSciencePipelinesApplication {
	return &dspav1.DataSciencePipelinesApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "testdspa", Namespace: "testnamespace"},
		Spec: dspav1.DSPASpec{
			PodToPodTLS: testutil.BoolPtr(podToPodTLS),
			APIServer:   &dspav1.APIServer{Deploy: true, EnableRoute: true},
			MLMD:        &dspav1.MLMD{Deploy: true, Envoy: &dspav1.Envoy{DeployRoute: true}},
			Database:    &dspav1.Database{MariaDB: &dspav1.MariaDB{Deploy: true}},
			ObjectStorage: &dspav1.ObjectStorage{
				Minio: &dspav1.Minio{Deploy: false, Image: "minio:test"},
			},
		},
	}
}

func proxyContainer(t testing.TB, deployment *appsv1.Deployment) *corev1.Container {
	t.Helper()
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == "kube-rbac-proxy" {
			return &deployment.Spec.Template.Spec.Containers[i]
		}
	}
	t.Fatal("kube-rbac-proxy container not found")
	return nil
}

func TestAPIServerProxyRendersTLSProfile(t *testing.T) {
	tests := []struct {
		name             string
		podToPodTLS      bool
		minVersion       string
		cipherSuites     string
		expectedUpstream string
	}{
		{
			name:             "intermediate HTTP upstream",
			minVersion:       "VersionTLS12",
			cipherSuites:     "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			expectedUpstream: "--upstream=http://localhost:8888",
		},
		{
			name:             "modern PodToPodTLS upstream",
			podToPodTLS:      true,
			minVersion:       "VersionTLS13",
			cipherSuites:     "TLS_AES_128_GCM_SHA256",
			expectedUpstream: "--upstream=https://ds-pipeline-testdspa.testnamespace.svc.cluster.local:8888",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, params, reconciler := CreateNewTestObjects()
			dspa := proxyTLSProfileTestDSPA(tt.podToPodTLS)
			if tt.podToPodTLS {
				t.Setenv("SSL_CERT_FILE", "testdata/tls/empty-ca-bundle.crt")
				require.NoError(t, reconciler.Create(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "openshift-service-ca.crt", Namespace: dspa.Namespace},
					Data:       map[string]string{"service-ca.crt": "service-ca"},
				}))
			}
			require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
			params.TLSMinVersion = tt.minVersion
			params.TLSCipherSuites = tt.cipherSuites
			require.NoError(t, reconciler.ReconcileAPIServer(ctx, dspa, params))

			deployment := &appsv1.Deployment{}
			require.NoError(t, reconciler.Get(ctx, types.NamespacedName{Name: "ds-pipeline-" + dspa.Name, Namespace: dspa.Namespace}, deployment))
			args := proxyContainer(t, deployment).Args
			assert.Contains(t, args, "--tls-min-version="+tt.minVersion)
			assert.Contains(t, args, "--tls-cipher-suites="+tt.cipherSuites)
			assert.Contains(t, args, tt.expectedUpstream)
		})
	}
}

func TestMLMDProxyRendersTLSProfile(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := proxyTLSProfileTestDSPA(false)
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	params.TLSMinVersion = "VersionTLS12"
	params.TLSCipherSuites = "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	require.NoError(t, reconciler.ReconcileMLMD(ctx, dspa, params))

	deployment := &appsv1.Deployment{}
	require.NoError(t, reconciler.Get(ctx, types.NamespacedName{Name: "ds-pipeline-metadata-envoy-" + dspa.Name, Namespace: dspa.Namespace}, deployment))
	args := proxyContainer(t, deployment).Args
	assert.Contains(t, args, "--tls-min-version=VersionTLS12")
	assert.Contains(t, args, "--tls-cipher-suites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256")
	assert.Contains(t, args, "--upstream=http://127.0.0.1:9090")
}
