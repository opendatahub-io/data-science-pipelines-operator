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

func TestSetEgressTemplateFieldsAdminRules(t *testing.T) {
	t.Parallel()

	params := &DSPAParams{}
	dsp := &dspa.DataSciencePipelinesApplication{
		Spec: dspa.DSPASpec{
			NetworkPolicy: &dspa.NetworkPolicySpec{
				Egress: &dspa.NetworkPolicyEgressSpec{
					APIServer: &dspa.WorkloadEgressSpec{
						Rules: []networkingv1.NetworkPolicyEgressRule{tcpEgressRule(3306)},
					},
					Driver: &dspa.WorkloadEgressSpec{
						AdditionalRules: []networkingv1.NetworkPolicyEgressRule{tcpEgressRule(5000)},
					},
				},
			},
		},
	}
	require.NoError(t, params.setEgressTemplateFields(dsp))
	require.Contains(t, params.APIServerReplacementEgress, "port: 3306")
	require.Empty(t, params.APIServerAdditionalEgress)
	require.Empty(t, params.DriverReplacementEgress)
	require.Contains(t, params.DriverAdditionalEgress, "port: 5000")

	require.NoError(t, params.setEgressTemplateFields(&dspa.DataSciencePipelinesApplication{}))
	require.Empty(t, params.APIServerReplacementEgress)
	require.Empty(t, params.APIServerAdditionalEgress)
	require.Empty(t, params.DriverReplacementEgress)
	require.Empty(t, params.DriverAdditionalEgress)
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
