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
	"fmt"
	"net/url"
	"strings"

	dspa "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/yaml"
)

const egressTemplateIndent = "    "

// setEgressTemplateFields fills the values the NetworkPolicy templates cannot compute.
// Default DNS, Kubernetes API, database, object storage, and pod-selector rules stay in the templates.
func (p *DSPAParams) setEgressTemplateFields(dsp *dspa.DataSciencePipelinesApplication) error {
	p.BuiltInDatabase = !p.UsingExternalDB(dsp)

	proxyPorts, err := proxyEgressPorts(p.ProxyConfig)
	if err != nil {
		return err
	}
	p.ProxyEgressPorts = proxyPorts

	mlflowPort, err := mlflowEgressPort(p.MLflow, p.resolvedMLflowEndpoint)
	if err != nil {
		return err
	}
	p.MLflowEgressPort = mlflowPort

	var apiServerRules, apiServerAdditional, driverRules, driverAdditional []networkingv1.NetworkPolicyEgressRule
	if dsp.Spec.NetworkPolicy != nil && dsp.Spec.NetworkPolicy.Egress != nil {
		if dsp.Spec.NetworkPolicy.Egress.APIServer != nil {
			apiServerRules = dsp.Spec.NetworkPolicy.Egress.APIServer.Rules
			apiServerAdditional = dsp.Spec.NetworkPolicy.Egress.APIServer.AdditionalRules
		}
		if dsp.Spec.NetworkPolicy.Egress.Driver != nil {
			driverRules = dsp.Spec.NetworkPolicy.Egress.Driver.Rules
			driverAdditional = dsp.Spec.NetworkPolicy.Egress.Driver.AdditionalRules
		}
	}

	p.APIServerReplacementEgress, err = renderEgressRules(apiServerRules)
	if err != nil {
		return fmt.Errorf("api server egress rules: %w", err)
	}
	p.APIServerAdditionalEgress, err = renderEgressRules(apiServerAdditional)
	if err != nil {
		return fmt.Errorf("api server additional egress rules: %w", err)
	}
	p.DriverReplacementEgress, err = renderEgressRules(driverRules)
	if err != nil {
		return fmt.Errorf("driver egress rules: %w", err)
	}
	p.DriverAdditionalEgress, err = renderEgressRules(driverAdditional)
	if err != nil {
		return fmt.Errorf("driver additional egress rules: %w", err)
	}
	return nil
}

func proxyEgressPorts(proxy *dspa.ProxyConfig) ([]string, error) {
	if proxy == nil {
		return nil, nil
	}
	var ports []string
	seen := map[string]struct{}{}
	for _, rawURL := range []string{proxy.HTTPProxy, proxy.HTTPSProxy} {
		port, err := egressPortFromURL(rawURL)
		if err != nil {
			return nil, fmt.Errorf("proxy URL: %w", err)
		}
		ports = appendDistinctPort(ports, seen, port)
	}
	return ports, nil
}

func mlflowEgressPort(mlflow *dspa.MLflowConfig, endpoint string) (string, error) {
	if mlflow != nil && mlflow.IntegrationMode != nil && *mlflow.IntegrationMode == dspa.Disabled {
		return "", nil
	}
	port, err := egressPortFromURL(endpoint)
	if err != nil {
		return "", fmt.Errorf("MLflow endpoint: %w", err)
	}
	return port, nil
}

// egressPortFromURL returns the TCP port for an http or https URL.
// An explicit port wins. When the URL omits a port, http uses 80 and https uses 443.
// An empty URL returns an empty port.
func egressPortFromURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid URL %q: must be http or https with a host", rawURL)
	}
	if port := parsed.Port(); port != "" {
		return port, nil
	}
	if parsed.Scheme == "https" {
		return "443", nil
	}
	return "80", nil
}

func appendDistinctPort(ports []string, seen map[string]struct{}, port string) []string {
	if port == "" {
		return ports
	}
	if _, alreadySeen := seen[port]; alreadySeen {
		return ports
	}
	seen[port] = struct{}{}
	return append(ports, port)
}

// renderEgressRules returns YAML indented for a template egress list.
// An empty list returns an empty string so the template keeps its default rules.
// A rule with neither ports nor a destination is rejected.
func renderEgressRules(rules []networkingv1.NetworkPolicyEgressRule) (string, error) {
	if len(rules) == 0 {
		return "", nil
	}
	for i, rule := range rules {
		if len(rule.Ports) == 0 && len(rule.To) == 0 {
			return "", fmt.Errorf("egress rule %d must specify at least one port or one destination", i)
		}
	}
	raw, err := yaml.Marshal(rules)
	if err != nil {
		return "", fmt.Errorf("marshal egress rules: %w", err)
	}
	var rendered strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		rendered.WriteString(egressTemplateIndent)
		rendered.WriteString(line)
		rendered.WriteByte('\n')
	}
	return rendered.String(), nil
}
