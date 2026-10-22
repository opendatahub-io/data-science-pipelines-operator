//go:build test_all || test_unit

/*
Copyright 2023.

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
	"strings"
	"testing"

	dspa "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestProxyEgressPorts(t *testing.T) {
	t.Parallel()

	ports, err := proxyEgressPorts(nil)
	require.NoError(t, err)
	require.Empty(t, ports)

	ports, err = proxyEgressPorts(&dspa.ProxyConfig{HTTPProxy: "http://proxy.example.com:3128"})
	require.NoError(t, err)
	require.Equal(t, []string{"3128"}, ports)

	ports, err = proxyEgressPorts(&dspa.ProxyConfig{HTTPProxy: "http://proxy.example.com"})
	require.NoError(t, err)
	require.Equal(t, []string{"80"}, ports)

	ports, err = proxyEgressPorts(&dspa.ProxyConfig{HTTPSProxy: "https://proxy.example.com"})
	require.NoError(t, err)
	require.Equal(t, []string{"443"}, ports)

	ports, err = proxyEgressPorts(&dspa.ProxyConfig{
		HTTPProxy:  "http://proxy.example.com:8080",
		HTTPSProxy: "https://proxy.example.com",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"8080", "443"}, ports)

	ports, err = proxyEgressPorts(&dspa.ProxyConfig{
		HTTPProxy:  "http://proxy.example.com:3128",
		HTTPSProxy: "https://proxy.example.com:3128",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"3128"}, ports)

	ports, err = proxyEgressPorts(&dspa.ProxyConfig{HTTPProxy: "proxy.example.com:3128"})
	require.NoError(t, err)
	require.Equal(t, []string{"3128"}, ports)

	ports, err = proxyEgressPorts(&dspa.ProxyConfig{HTTPProxy: "proxy.example.com"})
	require.NoError(t, err)
	require.Equal(t, []string{"80"}, ports)

	// SOCKS5 proxy with explicit port
	ports, err = proxyEgressPorts(&dspa.ProxyConfig{HTTPProxy: "socks5://proxy.example.com:1080"})
	require.NoError(t, err)
	require.Equal(t, []string{"1080"}, ports)

	// SOCKS5h proxy with default port
	ports, err = proxyEgressPorts(&dspa.ProxyConfig{HTTPProxy: "socks5h://proxy.example.com"})
	require.NoError(t, err)
	require.Equal(t, []string{"1080"}, ports)
}

func TestProxyEgressPorts_InvalidCredentialedURL(t *testing.T) {
	t.Parallel()

	// Test that invalid URLs with credentials don't expose the credentials in error messages.
	// Use an unsupported scheme with credentials.
	invalidCredentialedURL := "ftp://alice:secret@proxy.example.com:2121"
	_, err := proxyEgressPorts(&dspa.ProxyConfig{HTTPProxy: invalidCredentialedURL})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "alice")
	// Ensure the error message doesn't echo the full URL
	require.NotContains(t, err.Error(), "ftp://")

	// Test MLflow endpoint with credentials - invalid scheme
	invalidMLflowURL := "ftp://user:password@mlflow.example.com:5000"
	_, err = mlflowEgressPort(&dspa.MLflowConfig{}, invalidMLflowURL)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "password")
	require.NotContains(t, err.Error(), "user")
	require.NotContains(t, err.Error(), "ftp://")
}

func TestMLflowEgressPort(t *testing.T) {
	t.Parallel()

	autoDetect := dspa.AutoDetect
	disabled := dspa.Disabled

	port, err := mlflowEgressPort(&dspa.MLflowConfig{IntegrationMode: &autoDetect}, "https://mlflow.example.com:5000")
	require.NoError(t, err)
	require.Equal(t, "5000", port)

	port, err = mlflowEgressPort(&dspa.MLflowConfig{IntegrationMode: &autoDetect}, "https://mlflow.example.com")
	require.NoError(t, err)
	require.Equal(t, "443", port)

	port, err = mlflowEgressPort(&dspa.MLflowConfig{IntegrationMode: &autoDetect}, "")
	require.NoError(t, err)
	require.Empty(t, port)

	port, err = mlflowEgressPort(&dspa.MLflowConfig{IntegrationMode: &disabled}, "https://mlflow.example.com:5000")
	require.NoError(t, err)
	require.Empty(t, port)
}

func TestObjectStorageEgressPort(t *testing.T) {
	t.Parallel()

	secure := true
	insecure := false

	// An explicit port embedded in the endpoint stays authoritative regardless of secure.
	port, err := objectStorageEgressPort("http://s3.example.com:9001", &insecure)
	require.NoError(t, err)
	require.Equal(t, "9001", port)

	port, err = objectStorageEgressPort("https://s3.example.com:9001", &secure)
	require.NoError(t, err)
	require.Equal(t, "9001", port)

	// The default port follows the effective TLS setting: an explicit secure=false
	// overrides scheme https, so the S3 client connects over HTTP on port 80.
	port, err = objectStorageEgressPort("http://s3.example.com", &insecure)
	require.NoError(t, err)
	require.Equal(t, "80", port)

	port, err = objectStorageEgressPort("https://s3.example.com", &secure)
	require.NoError(t, err)
	require.Equal(t, "443", port)

	port, err = objectStorageEgressPort("https://s3.example.com", &insecure)
	require.NoError(t, err)
	require.Equal(t, "80", port)

	port, err = objectStorageEgressPort("http://s3.example.com", &secure)
	require.NoError(t, err)
	require.Equal(t, "443", port)

	// Without an explicit secure setting the scheme picks the default.
	port, err = objectStorageEgressPort("https://s3.example.com", nil)
	require.NoError(t, err)
	require.Equal(t, "443", port)

	port, err = objectStorageEgressPort("http://s3.example.com", nil)
	require.NoError(t, err)
	require.Equal(t, "80", port)

	// External MinIO route has no host until the Route object exists.
	port, err = objectStorageEgressPort("https://", &secure)
	require.NoError(t, err)
	require.Equal(t, "443", port)

	port, err = objectStorageEgressPort("", &secure)
	require.NoError(t, err)
	require.Empty(t, port)

	_, err = objectStorageEgressPort("ftp://alice:secret@s3.example.com:2121", nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "alice")
	require.NotContains(t, err.Error(), "ftp://")
}

func TestBuiltInDatabase(t *testing.T) {
	t.Parallel()

	params := &DSPAParams{}
	builtIn := &dspa.DataSciencePipelinesApplication{}
	require.NoError(t, params.setEgressTemplateFields(builtIn))
	require.True(t, params.BuiltInDatabase)

	external := &dspa.DataSciencePipelinesApplication{
		Spec: dspa.DSPASpec{
			Database: &dspa.Database{
				ExternalDB: &dspa.ExternalDB{},
			},
		},
	}
	require.NoError(t, params.setEgressTemplateFields(external))
	require.False(t, params.BuiltInDatabase)
}

func TestRenderEgressRules(t *testing.T) {
	t.Parallel()

	rendered, err := renderEgressRules(nil)
	require.NoError(t, err)
	require.Empty(t, rendered)

	rendered, err = renderEgressRules([]networkingv1.NetworkPolicyEgressRule{tcpEgressRule(3306)})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(rendered, "    - "))
	require.Contains(t, rendered, "port: 3306")
	require.Contains(t, rendered, "protocol: TCP")
	for _, line := range strings.Split(strings.TrimRight(rendered, "\n"), "\n") {
		require.Truef(t, strings.HasPrefix(line, "    "), "line %q is not indented for the egress template", line)
	}

	_, err = renderEgressRules([]networkingv1.NetworkPolicyEgressRule{{}})
	require.Error(t, err)
}

func TestSetEgressTemplateFieldsAPIServerAdditionalRules(t *testing.T) {
	t.Parallel()

	params := &DSPAParams{}
	dsp := &dspa.DataSciencePipelinesApplication{
		Spec: dspa.DSPASpec{
			NetworkPolicy: &dspa.NetworkPolicySpec{
				Egress: &dspa.NetworkPolicyEgressSpec{
					APIServer: &dspa.WorkloadEgressSpec{
						AdditionalRules: []networkingv1.NetworkPolicyEgressRule{tcpEgressRule(5000)},
					},
				},
			},
		},
	}
	require.NoError(t, params.setEgressTemplateFields(dsp))
	require.Contains(t, params.APIServerAdditionalEgress, "port: 5000")
	require.Empty(t, params.WorkflowControllerAdditionalEgress)

	require.NoError(t, params.setEgressTemplateFields(&dspa.DataSciencePipelinesApplication{}))
	require.Empty(t, params.APIServerAdditionalEgress)
}

func TestSetEgressTemplateFieldsWorkflowControllerAdditionalRules(t *testing.T) {
	t.Parallel()

	params := &DSPAParams{}
	dsp := &dspa.DataSciencePipelinesApplication{
		Spec: dspa.DSPASpec{
			NetworkPolicy: &dspa.NetworkPolicySpec{
				Egress: &dspa.NetworkPolicyEgressSpec{
					WorkflowController: &dspa.WorkloadEgressSpec{
						AdditionalRules: []networkingv1.NetworkPolicyEgressRule{tcpEgressRule(8443)},
					},
				},
			},
		},
	}
	require.NoError(t, params.setEgressTemplateFields(dsp))
	require.Contains(t, params.WorkflowControllerAdditionalEgress, "port: 8443")
	require.Empty(t, params.APIServerAdditionalEgress)

	require.NoError(t, params.setEgressTemplateFields(&dspa.DataSciencePipelinesApplication{}))
	require.Empty(t, params.WorkflowControllerAdditionalEgress)
}

func TestMLflowHostnameFromURL(t *testing.T) {
	t.Parallel()

	// With explicit port
	hostname := mlflowHostnameFromURL("https://mlflow.opendatahub.svc:8443/mlflow")
	require.Equal(t, "mlflow.opendatahub.svc", hostname)

	// Without port
	hostname = mlflowHostnameFromURL("https://mlflow.redhat-ods-applications.svc/mlflow")
	require.Equal(t, "mlflow.redhat-ods-applications.svc", hostname)

	// With HTTP and port
	hostname = mlflowHostnameFromURL("http://mlflow.example.com:5000")
	require.Equal(t, "mlflow.example.com", hostname)

	// Empty input
	hostname = mlflowHostnameFromURL("")
	require.Empty(t, hostname)

	// Invalid URL
	hostname = mlflowHostnameFromURL("not a url")
	require.Empty(t, hostname)
}

func TestComputeEffectiveNoProxy(t *testing.T) {
	t.Parallel()

	// Empty noProxy and unresolved MLflow: leave unset so KFP's fallback applies.
	require.Empty(t, computeEffectiveNoProxy("", ""))

	// Empty noProxy with MLflow: include the KFP baseline, otherwise NO_PROXY would
	// be only the MLflow host and suppress KFP's fallback.
	require.Equal(t, "mlflow.opendatahub.svc,"+kfpDefaultNoProxy, computeEffectiveNoProxy("", "mlflow.opendatahub.svc"))
	require.Equal(t, "mlflow.redhat-ods-applications.svc,"+kfpDefaultNoProxy, computeEffectiveNoProxy("", "mlflow.redhat-ods-applications.svc"))

	// Existing user noProxy is preserved and not rewritten with the KFP baseline.
	require.Equal(t, "example.com,.svc.cluster.local", computeEffectiveNoProxy("example.com,.svc.cluster.local", ""))
	require.Equal(t, "example.com,other.com,mlflow.opendatahub.svc", computeEffectiveNoProxy("example.com,other.com", "mlflow.opendatahub.svc"))
	require.Equal(t, "example.com,mlflow.svc", computeEffectiveNoProxy("  example.com  ", "  mlflow.svc  "))
	require.Equal(t, "mlflow.opendatahub.svc,.svc.cluster.local", computeEffectiveNoProxy("mlflow.opendatahub.svc,.svc.cluster.local", "mlflow.opendatahub.svc"))
}

func TestSetEgressTemplateFieldsMLflowNoProxy(t *testing.T) {
	t.Parallel()

	autoDetect := dspa.AutoDetect
	disabled := dspa.Disabled
	mlflowURL := "https://mlflow.opendatahub.svc:8443/mlflow"
	userNoProxy := "localhost,127.0.0.1,.svc.cluster.local,kubernetes.default.svc"

	t.Run("existing noProxy appends hostname and does not mutate spec", func(t *testing.T) {
		t.Parallel()
		proxy := &dspa.ProxyConfig{HTTPProxy: "http://proxy.example.com:3128", NoProxy: userNoProxy}
		params := &DSPAParams{
			MLflow:                 &dspa.MLflowConfig{IntegrationMode: &autoDetect},
			resolvedMLflowEndpoint: mlflowURL,
			ProxyConfig:            proxy,
		}
		dsp := &dspa.DataSciencePipelinesApplication{Spec: dspa.DSPASpec{Proxy: proxy}}
		require.NoError(t, params.setEgressTemplateFields(dsp))
		require.Equal(t, "8443", params.MLflowEgressPort)
		require.Equal(t, "mlflow.opendatahub.svc", params.MLflowHostname)
		require.Equal(t, userNoProxy+",mlflow.opendatahub.svc", params.EffectiveNoProxy)
		require.Equal(t, userNoProxy, dsp.Spec.Proxy.NoProxy)
		require.Equal(t, userNoProxy, proxy.NoProxy)
	})

	t.Run("empty noProxy keeps KFP baseline with both platform hosts", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			namespace string
			url       string
			host      string
		}{
			{namespace: "opendatahub", url: "https://mlflow.opendatahub.svc:8443/mlflow", host: "mlflow.opendatahub.svc"},
			{namespace: "redhat-ods-applications", url: "https://mlflow.redhat-ods-applications.svc:8443/mlflow", host: "mlflow.redhat-ods-applications.svc"},
		} {
			params := &DSPAParams{
				MLflow:                 &dspa.MLflowConfig{IntegrationMode: &autoDetect},
				resolvedMLflowEndpoint: tc.url,
				ProxyConfig:            &dspa.ProxyConfig{HTTPSProxy: "https://proxy.example.com:8443"},
				ApplicationsNamespace:  tc.namespace,
			}
			require.NoError(t, params.setEgressTemplateFields(&dspa.DataSciencePipelinesApplication{}))
			require.Equal(t, "8443", params.MLflowEgressPort)
			require.Equal(t, tc.host, params.MLflowHostname)
			require.Equal(t, tc.host+","+kfpDefaultNoProxy, params.EffectiveNoProxy)
		}
	})

	t.Run("disabled MLflow", func(t *testing.T) {
		t.Parallel()
		proxy := &dspa.ProxyConfig{HTTPProxy: "http://proxy.example.com:3128", NoProxy: userNoProxy}
		params := &DSPAParams{
			MLflow:                 &dspa.MLflowConfig{IntegrationMode: &disabled},
			resolvedMLflowEndpoint: mlflowURL,
			ProxyConfig:            proxy,
		}
		dsp := &dspa.DataSciencePipelinesApplication{Spec: dspa.DSPASpec{Proxy: proxy}}
		require.NoError(t, params.setEgressTemplateFields(dsp))
		require.Empty(t, params.MLflowEgressPort)
		require.Empty(t, params.MLflowHostname)
		require.Equal(t, userNoProxy, params.EffectiveNoProxy)
		require.Equal(t, userNoProxy, dsp.Spec.Proxy.NoProxy)
	})

	t.Run("unresolved MLflow", func(t *testing.T) {
		t.Parallel()
		params := &DSPAParams{
			MLflow:      &dspa.MLflowConfig{IntegrationMode: &autoDetect},
			ProxyConfig: &dspa.ProxyConfig{HTTPProxy: "http://proxy.example.com:3128"},
		}
		require.NoError(t, params.setEgressTemplateFields(&dspa.DataSciencePipelinesApplication{}))
		require.Empty(t, params.MLflowEgressPort)
		require.Empty(t, params.MLflowHostname)
		require.Empty(t, params.EffectiveNoProxy)
	})

	t.Run("no proxy spec leaves NO_PROXY unset", func(t *testing.T) {
		t.Parallel()
		params := &DSPAParams{
			MLflow:                 &dspa.MLflowConfig{IntegrationMode: &autoDetect},
			resolvedMLflowEndpoint: mlflowURL,
		}
		require.NoError(t, params.setEgressTemplateFields(&dspa.DataSciencePipelinesApplication{}))
		require.Equal(t, "8443", params.MLflowEgressPort)
		require.Equal(t, "mlflow.opendatahub.svc", params.MLflowHostname)
		require.Empty(t, params.EffectiveNoProxy)
	})
}

func tcpEgressRule(port int32) networkingv1.NetworkPolicyEgressRule {
	protocol := corev1.ProtocolTCP
	portValue := intstr.FromInt32(port)
	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{{
			Protocol: &protocol,
			Port:     &portValue,
		}},
	}
}
