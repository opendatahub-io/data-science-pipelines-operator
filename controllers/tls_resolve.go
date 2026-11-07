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
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/util"
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
	// CurvePreferences is the --tls-curve-preferences flag value: comma-separated numeric IDs.
	CurvePreferences string
}

// resolveKubeRBACProxyTLS returns effective kube-rbac-proxy TLS CLI flag values.
// Applies policy resolution: missing profile → Intermediate; StrictAllComponents → configured profile.
func resolveKubeRBACProxyTLS(profile configv1.TLSProfileSpec, profileType configv1.TLSProfileType, adherence configv1.TLSAdherencePolicy) (*KubeRBACProxyTLSArgs, error) {
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
		if err := rejectInsecureCustomCiphers(profileType, profile.Ciphers); err != nil {
			return nil, err
		}
		return profileToRBACProxyArgsForType(profile, profileType)
	default:
		// Unknown adherence — treat as strict (fail-closed).
		if err := rejectInsecureCustomCiphers(profileType, profile.Ciphers); err != nil {
			return nil, err
		}
		return profileToRBACProxyArgsForType(profile, profileType)
	}
}

func rejectInsecureCustomCiphers(profileType configv1.TLSProfileType, ciphers []string) error {
	if profileType != configv1.TLSProfileCustomType {
		return nil
	}

	rejected := util.ProhibitedTLSCipherNames(ciphers)
	if len(rejected) > 0 {
		return fmt.Errorf("Custom TLS profile contains prohibited cipher suites: %v", rejected)
	}
	return nil
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
	return profileToRBACProxyArgsForType(profile, "")
}

func profileToRBACProxyArgsForType(profile configv1.TLSProfileSpec, profileType configv1.TLSProfileType) (*KubeRBACProxyTLSArgs, error) {
	args := &KubeRBACProxyTLSArgs{
		MinVersion: string(profile.MinTLSVersion),
	}
	curveIDs, unsupportedGroups := libgocrypto.TLSGroupsToCurveIDs(profile.Groups)
	if len(profile.Groups) > 0 && len(curveIDs) == 0 {
		return nil, fmt.Errorf("all %d groups in TLS profile are unsupported by Go: %v",
			len(profile.Groups), unsupportedGroups)
	}
	if len(curveIDs) > 0 {
		curvePreferenceIDs := make([]string, 0, len(curveIDs))
		for _, curveID := range curveIDs {
			curvePreferenceIDs = append(curvePreferenceIDs, strconv.Itoa(int(curveID)))
		}
		args.CurvePreferences = strings.Join(curvePreferenceIDs, ",")
	}

	// TLS 1.3: Go ignores CipherSuites; omit flag.
	if profile.MinTLSVersion == configv1.VersionTLS13 {
		return args, nil
	}

	// Convert cipher names (OpenSSL or IANA) to IANA names for kube-rbac-proxy.
	var ianaNames []string
	if len(profile.Ciphers) > 0 {
		var unsupported []string
		ianaNames, unsupported = cipherNamesToIANA(profile.Ciphers)
		if profileType == configv1.TLSProfileOldType {
			ianaNames = supportedTLS12CipherNames(ianaNames)
			unsupported = nil
		}
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
		if !util.HasHTTP2CompatibleCipherNames(ianaNames) {
			return nil, fmt.Errorf("TLS profile with minimum version %q: kube-rbac-proxy cannot serve HTTP/2 with that cipher list %q; "+
				"the list must include TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 or TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
				profile.MinTLSVersion, strings.Join(ianaNames, ","))
		}
	}

	// Mirror the manager-side validation: hybrid ML-KEM groups only
	// negotiate TLS 1.3, so below a TLS 1.3 minimum a group list with no
	// TLS 1.2-capable curve leaves TLS 1.2 clients unable to complete
	// handshakes, unless a static RSA key exchange cipher is configured
	// (it needs no curve). With no configured ciphers there is no static
	// RSA exception, matching the manager-side semantics.
	if len(curveIDs) > 0 && !util.HasTLS12CapableCurveIDs(curveIDs) &&
		!util.HasStaticRSAKeyExchangeCipherNames(ianaNames) {
		return nil, fmt.Errorf("TLS profile with minimum %q has no groups usable for TLS 1.2: %v",
			profile.MinTLSVersion, profile.Groups)
	}

	if len(ianaNames) > 0 {
		args.CipherSuites = strings.Join(ianaNames, ",")
	}

	return args, nil
}

func supportedTLS12CipherNames(names []string) []string {
	runtimeSuites := make(map[string]*tls.CipherSuite)
	for _, suite := range tls.CipherSuites() {
		runtimeSuites[suite.Name] = suite
	}
	for _, suite := range tls.InsecureCipherSuites() {
		runtimeSuites[suite.Name] = suite
	}

	supported := make([]string, 0, len(names))
	for _, name := range names {
		suite, ok := runtimeSuites[name]
		if !ok {
			continue
		}
		for _, version := range suite.SupportedVersions {
			if version <= tls.VersionTLS12 {
				supported = append(supported, name)
				break
			}
		}
	}
	return supported
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

// APIServerTLSArgs holds the resolved CLI flag values for the DSP API server
// and webhook operands (the /bin/apiserver binary from kubeflow/pipelines).
type APIServerTLSArgs struct {
	// MinVersion is the --tlsMinVersion flag value: VersionTLS12 or VersionTLS13.
	MinVersion string
	// CipherSuites is the --tlsCipherSuites flag value: comma-separated Go
	// cipher suite names (IANA format). Empty under TLS 1.3 or when the
	// profile lists no ciphers — the flag must be omitted so the operand
	// applies Go's default suites.
	CipherSuites string
}

// resolveAPIServerTLS returns effective operand TLS flag values, or nil when
// the adherence policy preserves the operand's serving defaults.
// NoOpinion and LegacyAdheringComponentsOnly leave the operand unconfigured
// (binary defaults: TLS 1.2 minimum, Go default suites). StrictAllComponents
// and unknown adherence values project the effective profile; a nil profile
// resolves to Intermediate.
func resolveAPIServerTLS(profile configv1.TLSProfileSpec, profileType configv1.TLSProfileType, adherence configv1.TLSAdherencePolicy) (*APIServerTLSArgs, error) {
	switch adherence {
	case configv1.TLSAdherencePolicyNoOpinion, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly:
		return nil, nil
	case configv1.TLSAdherencePolicyStrictAllComponents:
	default:
		// Unknown adherence — treat as strict (fail-closed).
	}
	if err := rejectInsecureCustomCiphers(profileType, profile.Ciphers); err != nil {
		return nil, err
	}

	if profile.MinTLSVersion == "" {
		profile = *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	}
	return profileToAPIServerArgs(profile)
}

// profileToAPIServerArgs converts TLSProfileSpec to operand flag values.
// Fails closed: the operand accepts only VersionTLS12/VersionTLS13 minimums
// and rejects any unsupported cipher, so unrepresentable profiles (including
// Old under Strict, which the operand cannot serve) abort the reconcile and
// leave existing workloads unchanged rather than being downgraded.
func profileToAPIServerArgs(profile configv1.TLSProfileSpec) (*APIServerTLSArgs, error) {
	switch profile.MinTLSVersion {
	case configv1.VersionTLS12, configv1.VersionTLS13:
	default:
		return nil, fmt.Errorf("DSP API server does not support minimum TLS version %q (supported: VersionTLS12, VersionTLS13)", profile.MinTLSVersion)
	}

	args := &APIServerTLSArgs{MinVersion: string(profile.MinTLSVersion)}

	// TLS 1.3: cipher suites are not configurable in Go; omit the flag.
	if profile.MinTLSVersion == configv1.VersionTLS13 {
		return args, nil
	}

	if len(profile.Ciphers) > 0 {
		ianaNames, unsupported := cipherNamesToIANA(profile.Ciphers)
		if len(ianaNames) == 0 {
			return nil, fmt.Errorf("all %d ciphers in TLS profile are unsupported by Go: %v",
				len(profile.Ciphers), unsupported)
		}
		if len(unsupported) > 0 {
			return nil, fmt.Errorf("cannot use TLS profile with unsupported ciphers: %v (unsupported: %v)",
				profile.Ciphers, unsupported)
		}
		// The operand advertises HTTP/2 (ALPN "h2") on its HTTP and webhook
		// surfaces, and Go's net/http requires an AES-128-GCM cipher suite
		// for HTTP/2. A list without one is unrepresentable: fail closed
		// rather than deploy listeners that negotiate h2 and then break.
		hasHTTP2Suite := false
		for _, name := range ianaNames {
			if name == "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256" ||
				name == "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256" {
				hasHTTP2Suite = true
				break
			}
		}
		if !hasHTTP2Suite {
			return nil, fmt.Errorf("TLS 1.2 cipher list lacks an HTTP/2-required AES-128-GCM cipher suite "+
				"(need TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 or TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256): %v",
				profile.Ciphers)
		}
		args.CipherSuites = strings.Join(ianaNames, ",")
	}

	return args, nil
}
