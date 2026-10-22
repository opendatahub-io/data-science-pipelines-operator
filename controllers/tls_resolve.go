/*
Copyright 2024.

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
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
)

// KubeRBACProxyTLSArgs holds the resolved CLI flag string values for kube-rbac-proxy sidecars.
// These are injected as container args in the API server and MLMD Envoy Deployments.
type KubeRBACProxyTLSArgs struct {
	// MinVersion is the --tls-min-version flag value, e.g. "VersionTLS12".
	MinVersion string
	// CipherSuites is the --tls-cipher-suites flag value: comma-separated IANA cipher names.
	// Empty when MinVersion is VersionTLS13 — Go ignores CipherSuites for TLS 1.3, so the
	// flag must be omitted entirely.
	CipherSuites string
}

// resolveKubeRBACProxyTLS returns effective kube-rbac-proxy TLS CLI flag values.
// Applies policy resolution: missing profile → Intermediate; StrictAllComponents → configured profile.
func resolveKubeRBACProxyTLS(profile configv1.TLSProfileSpec, adherence configv1.TLSAdherencePolicy) (*KubeRBACProxyTLSArgs, error) {
	// Missing or zero-value profile — fall back to Intermediate regardless of adherence.
	if profile.MinTLSVersion == "" {
		return intermediateRBACProxyArgs()
	}

	switch adherence {
	case configv1.TLSAdherencePolicyNoOpinion, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly:
		// Non-strict adherence — use Intermediate defaults.
		return intermediateRBACProxyArgs()
	case configv1.TLSAdherencePolicyStrictAllComponents:
		// Strict adherence — use the configured profile, rejecting any unsupported ciphers.
		return profileToRBACProxyArgs(profile)
	default:
		// Unknown adherence — treat as strict (fail-closed).
		return profileToRBACProxyArgs(profile)
	}
}

// intermediateRBACProxyArgs returns Intermediate profile args as fallback.
// Used for missing/zero profiles (including on StrictAllComponents clusters) and
// for NoOpinion/Legacy. An unsupported Intermediate cipher fails closed rather
// than being silently dropped.
func intermediateRBACProxyArgs() (*KubeRBACProxyTLSArgs, error) {
	intermediate := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	return profileToRBACProxyArgs(intermediate)
}

// profileToRBACProxyArgs converts TLSProfileSpec to CLI flag values.
// Fails closed: rejects any profile containing unsupported ciphers.
func profileToRBACProxyArgs(profile configv1.TLSProfileSpec) (*KubeRBACProxyTLSArgs, error) {
	args := &KubeRBACProxyTLSArgs{
		MinVersion: string(profile.MinTLSVersion),
	}

	// TLS 1.3: Go ignores CipherSuites; omit flag.
	if profile.MinTLSVersion == configv1.VersionTLS13 {
		return args, nil
	}

	// Convert cipher names (OpenSSL or IANA) to IANA names for kube-rbac-proxy.
	if len(profile.Ciphers) > 0 {
		ianaNames, unsupported := cipherNamesToIANA(profile.Ciphers)
		if len(ianaNames) == 0 {
			// All ciphers unsupported — fail reconciliation.
			return nil, fmt.Errorf("all %d ciphers in TLS profile are unsupported by Go: %v",
				len(profile.Ciphers), unsupported)
		}
		// Fail closed: reject profiles with any unsupported ciphers.
		if len(unsupported) > 0 {
			return nil, fmt.Errorf("cannot use TLS profile with unsupported ciphers: %v (unsupported: %v)",
				profile.Ciphers, unsupported)
		}
		args.CipherSuites = strings.Join(ianaNames, ",")
	}

	return args, nil
}

// cipherNamesToIANA converts cipher names to IANA format.
// Recognizes IANA (TLS_*) and OpenSSL formats; returns unsupported separately.
func cipherNamesToIANA(ciphers []string) (ianaNames []string, unsupported []string) {
	for _, cipher := range ciphers {
		// IANA format (TLS_*) recognized by library-go; pass through.
		if looksLikeIANA(cipher) {
			if _, err := libgocrypto.CipherSuite(cipher); err == nil {
				ianaNames = append(ianaNames, cipher)
				continue
			}
		}

		// Try OpenSSL → IANA conversion.
		converted := libgocrypto.OpenSSLToIANACipherSuites([]string{cipher})
		if len(converted) == 1 {
			ianaNames = append(ianaNames, converted[0])
			continue
		}

		// Unrecognized — return for caller handling.
		unsupported = append(unsupported, cipher)
	}
	return ianaNames, unsupported
}

// looksLikeIANA reports whether name is structurally an IANA cipher suite name.
// IANA TLS names follow the pattern TLS_* with underscores; OpenSSL names use hyphens.
func looksLikeIANA(name string) bool {
	return strings.HasPrefix(name, "TLS_") && !strings.Contains(name, "-")
}
