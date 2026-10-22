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

	// Hostname follows the same resolved endpoint as the egress port. Disabled or
	// unresolved MLflow leaves both empty so no bypass or NetworkPolicy rule is added.
	p.MLflowHostname = ""
	if p.MLflowEgressPort != "" {
		p.MLflowHostname = mlflowHostnameFromURL(p.resolvedMLflowEndpoint)
	}

	// Effective NO_PROXY is only used when spec.proxy is set. The DSPA spec is not mutated.
	p.EffectiveNoProxy = ""
	if p.ProxyConfig != nil {
		p.EffectiveNoProxy = computeEffectiveNoProxy(p.ProxyConfig.NoProxy, p.MLflowHostname)
	}

	objectStoragePort, err := objectStorageEgressPort(p.ObjectStorageConnection.Endpoint, p.ObjectStorageConnection.Secure)
	if err != nil {
		return err
	}
	p.ObjectStorageEgressPort = objectStoragePort

	var apiServerAdditional []networkingv1.NetworkPolicyEgressRule
	if dsp.Spec.NetworkPolicy != nil && dsp.Spec.NetworkPolicy.Egress != nil && dsp.Spec.NetworkPolicy.Egress.APIServer != nil {
		apiServerAdditional = dsp.Spec.NetworkPolicy.Egress.APIServer.AdditionalRules
	}

	p.APIServerAdditionalEgress, err = renderEgressRules(apiServerAdditional)
	if err != nil {
		return fmt.Errorf("api server additional egress rules: %w", err)
	}

	var workflowControllerAdditional []networkingv1.NetworkPolicyEgressRule
	if dsp.Spec.NetworkPolicy != nil && dsp.Spec.NetworkPolicy.Egress != nil && dsp.Spec.NetworkPolicy.Egress.WorkflowController != nil {
		workflowControllerAdditional = dsp.Spec.NetworkPolicy.Egress.WorkflowController.AdditionalRules
	}

	p.WorkflowControllerAdditionalEgress, err = renderEgressRules(workflowControllerAdditional)
	if err != nil {
		return fmt.Errorf("workflow controller additional egress rules: %w", err)
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
		port, err := proxyEgressPort(rawURL)
		if err != nil {
			return nil, fmt.Errorf("proxy URL: %w", err)
		}
		ports = appendDistinctPort(ports, seen, port)
	}
	return ports, nil
}

// objectStorageEgressPort returns the TCP port the API server uses for object storage.
// The endpoint already includes a port embedded in the host when the port field is empty;
// an embedded port stays authoritative. Otherwise the default follows the effective TLS
// setting rather than the scheme, because an explicit externalStorage.secure overrides
// the scheme for the S3 client: secure=false with scheme https still connects over HTTP
// on port 80. A nil secure falls back to the scheme (https 443, http 80).
// An empty endpoint returns an empty port.
func objectStorageEgressPort(endpoint string, secure *bool) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("object storage endpoint: invalid URL: must be http or https")
	}
	if port := parsed.Port(); port != "" {
		return port, nil
	}
	if secure == nil {
		if parsed.Scheme == "https" {
			return "443", nil
		}
		return "80", nil
	}
	if *secure {
		return "443", nil
	}
	return "80", nil
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

// proxyEgressPort returns the proxy TCP port. A missing scheme is treated as http.
// Supports http, https, socks5, and socks5h URLs.
func proxyEgressPort(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		parsed, err = url.Parse("http://" + rawURL)
	}
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h") {
		return "", fmt.Errorf("invalid proxy URL: must be http, https, socks5, or socks5h with a host")
	}
	return portFromURL(parsed), nil
}

// egressPortFromURL returns the TCP port for an http or https URL.
func egressPortFromURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid endpoint URL: must be http or https with a host")
	}
	return portFromURL(parsed), nil
}

func portFromURL(parsed *url.URL) string {
	if port := parsed.Port(); port != "" {
		return port
	}
	switch parsed.Scheme {
	case "https":
		return "443"
	case "socks5", "socks5h":
		return "1080"
	default:
		return "80"
	}
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

// kfpDefaultNoProxy is the in-cluster bypass KFP applies when NO_PROXY is unset.
// Setting NO_PROXY to only the MLflow host would suppress this fallback.
const kfpDefaultNoProxy = ".svc.cluster.local,127.0.0.1,localhost"

// mlflowHostnameFromURL extracts the hostname (without port) from an MLflow tracking URL.
// Example: "https://mlflow.opendatahub.svc:8443/mlflow" -> "mlflow.opendatahub.svc"
func mlflowHostnameFromURL(urlString string) string {
	urlString = strings.TrimSpace(urlString)
	if urlString == "" {
		return ""
	}
	parsed, err := url.Parse(urlString)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Hostname()
}

// computeEffectiveNoProxy returns the API server NO_PROXY value.
// User entries are preserved as-is. The MLflow hostname is appended when resolved.
// When noProxy is empty, the KFP baseline is included so setting NO_PROXY for MLflow
// does not suppress KFP's fallback. This does not mutate the DSPA spec.
func computeEffectiveNoProxy(userNoProxy, mlflowHostname string) string {
	userNoProxy = strings.TrimSpace(userNoProxy)
	mlflowHostname = strings.TrimSpace(mlflowHostname)

	if userNoProxy != "" {
		return appendNoProxyHost(userNoProxy, mlflowHostname)
	}
	if mlflowHostname == "" {
		return ""
	}
	return mlflowHostname + "," + kfpDefaultNoProxy
}

func appendNoProxyHost(base, host string) string {
	if host == "" || noProxyContainsHost(base, host) {
		return base
	}
	return base + "," + host
}

func noProxyContainsHost(noProxy, host string) bool {
	for _, entry := range strings.Split(noProxy, ",") {
		if strings.TrimSpace(entry) == host {
			return true
		}
	}
	return false
}
