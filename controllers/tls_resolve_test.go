//go:build test_all || test_unit

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
	"context"
	"errors"
	"strings"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// intermediateProfile is the hardened Intermediate TLS profile used as the safe fallback.
var intermediateProfile = *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

// modernProfile is the Modern TLS profile (TLS 1.3 only).
var modernProfile = *configv1.TLSProfiles[configv1.TLSProfileModernType]

// --- resolveKubeRBACProxyTLS ---

func TestResolveKubeRBACProxyTLS_NoOpinion_UsesIntermediate(t *testing.T) {
	args, err := resolveKubeRBACProxyTLS(modernProfile, configv1.TLSProfileModernType, configv1.TLSAdherencePolicyNoOpinion)
	require.NoError(t, err)
	// NoOpinion must always fall back to Intermediate, ignoring the supplied profile.
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites, "Intermediate profile should have cipher suites")
}

func TestResolveKubeRBACProxyTLS_LegacyAdhering_UsesIntermediate(t *testing.T) {
	args, err := resolveKubeRBACProxyTLS(modernProfile, configv1.TLSProfileModernType, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly)
	require.NoError(t, err)
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites)
}

func TestResolveKubeRBACProxyTLS_StrictAllComponents_UsesProfile(t *testing.T) {
	args, err := resolveKubeRBACProxyTLS(intermediateProfile, configv1.TLSProfileIntermediateType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites)
}

func TestResolveKubeRBACProxyTLS_UnknownAdherence_TreatedAsStrict(t *testing.T) {
	// Unknown adherence must use strict path. TLS 1.3 has no ciphers; Intermediate fallback would have ciphers.
	tls13Profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS13,
		Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
	}
	args, err := resolveKubeRBACProxyTLS(tls13Profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicy("SomeFuturePolicy"))
	require.NoError(t, err)
	assert.Equal(t, "VersionTLS13", args.MinVersion)
	assert.Empty(t, args.CipherSuites, "strict path with TLS 1.3 must not emit ciphers")
}

func TestResolveKubeRBACProxyTLS_ZeroProfile_UsesIntermediate(t *testing.T) {
	// A zero-value TLSProfileSpec (empty MinTLSVersion) must fall back to Intermediate.
	args, err := resolveKubeRBACProxyTLS(configv1.TLSProfileSpec{}, configv1.TLSProfileIntermediateType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites)
}

func TestResolveKubeRBACProxyTLS_StrictAllComponents_TLS13_NoCiphers(t *testing.T) {
	// Strict + TLS 1.3 profile: cipher flag must be omitted.
	args, err := resolveKubeRBACProxyTLS(modernProfile, configv1.TLSProfileModernType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	assert.Equal(t, "VersionTLS13", args.MinVersion)
	assert.Empty(t, args.CipherSuites, "TLS 1.3 must not set cipher suites even under strict adherence")
}

func TestResolveKubeRBACProxyTLS_StrictAllComponents_InvalidProfile_ReturnsError(t *testing.T) {
	// Strict + all ciphers unsupported → error; existing Deployment left untouched.
	badProfile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"BOGUS_CIPHER_1", "BOGUS_CIPHER_2"},
	}
	_, err := resolveKubeRBACProxyTLS(badProfile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.Error(t, err, "invalid strict profile must fail reconciliation")
	assert.Contains(t, err.Error(), "all 2 ciphers")
}

func TestResolveKubeRBACProxyTLS_StrictAllComponents_AllGroupsUnsupported_ReturnsError(t *testing.T) {
	badProfile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Groups:        []configv1.TLSGroup{"NOT_A_GROUP"},
	}
	_, err := resolveKubeRBACProxyTLS(badProfile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.Error(t, err, "all-unusable strict groups must fail reconciliation")
	assert.Contains(t, err.Error(), "all 1 groups in TLS profile are unsupported by Go")
}

func TestResolveKubeRBACProxyTLS_NoOpinion_BadProfileIgnored(t *testing.T) {
	// NoOpinion adherence must use Intermediate even if the supplied profile is invalid.
	badProfile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"BOGUS_CIPHER_1"},
	}
	args, err := resolveKubeRBACProxyTLS(badProfile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyNoOpinion)
	require.NoError(t, err, "NoOpinion must fall back to Intermediate, never error on bad profile")
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
}

func TestResolveKubeRBACProxyTLS_CustomProfileRejectsInsecureCiphers(t *testing.T) {
	tests := []struct {
		name    string
		ciphers []string
	}{
		{name: "RC4 IANA", ciphers: []string{"TLS_RSA_WITH_RC4_128_SHA"}},
		{name: "RC4 OpenSSL", ciphers: []string{"RC4-SHA"}},
		{name: "3DES IANA", ciphers: []string{"TLS_RSA_WITH_3DES_EDE_CBC_SHA"}},
		{name: "3DES OpenSSL", ciphers: []string{"DES-CBC3-SHA"}},
		{name: "SHA-1 CBC IANA", ciphers: []string{"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA"}},
		{name: "SHA-1 CBC OpenSSL", ciphers: []string{"ECDHE-RSA-AES128-SHA"}},
		{name: "SHA-1 CBC static RSA", ciphers: []string{"TLS_RSA_WITH_AES_128_CBC_SHA"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       tt.ciphers,
			}
			args, err := resolveKubeRBACProxyTLS(profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
			require.Error(t, err)
			assert.Nil(t, args)
			assert.Contains(t, err.Error(), "prohibited cipher suites")
		})
	}
}

func TestResolveKubeRBACProxyTLS_CustomProfileAllowsSHA256CBC(t *testing.T) {
	// SHA-256 CBC suites are runtime-supported and not in the prohibited set
	// (RC4, 3DES, SHA-1 CBC). A list mixing them with an AES-128-GCM suite
	// resolves; a SHA-256 CBC-only list is rejected by the HTTP/2 capability
	// check, not the prohibited-cipher check.
	tests := []struct {
		name       string
		ciphers    []string
		wantErrSub string
	}{
		{
			name: "mixed GCM and SHA-256 CBC IANA",
			ciphers: []string{
				"ECDHE-RSA-AES128-GCM-SHA256",
				"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
			},
		},
		{
			name: "mixed GCM and SHA-256 CBC OpenSSL",
			ciphers: []string{
				"ECDHE-RSA-AES128-GCM-SHA256",
				"ECDHE-RSA-AES128-SHA256",
			},
		},
		{
			name:       "SHA-256 CBC only fails the HTTP/2 check",
			ciphers:    []string{"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256"},
			wantErrSub: "cannot serve HTTP/2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       tt.ciphers,
			}
			args, err := resolveKubeRBACProxyTLS(profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
			if tt.wantErrSub != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSub)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, args.CipherSuites, "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256")
		})
	}
}

func TestResolveKubeRBACProxyTLS_OldProfilePreservesDocumentedLegacyCiphers(t *testing.T) {
	oldProfile := *configv1.TLSProfiles[configv1.TLSProfileOldType]
	args, err := resolveKubeRBACProxyTLS(oldProfile, configv1.TLSProfileOldType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	assert.Contains(t, args.CipherSuites, "TLS_RSA_WITH_3DES_EDE_CBC_SHA")
	assert.Contains(t, args.CipherSuites, "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA")
}

// --- profileToRBACProxyArgs ---

func TestProfileToRBACProxyArgs_IntermediateProfile(t *testing.T) {
	args, err := profileToRBACProxyArgs(intermediateProfile)
	require.NoError(t, err)
	assert.Equal(t, "VersionTLS12", args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites, "Intermediate profile should produce cipher suites")
	// All cipher names should be comma-separated IANA names (contain no spaces).
	for _, c := range strings.Split(args.CipherSuites, ",") {
		assert.NotContains(t, c, " ", "cipher name should not contain spaces: %q", c)
	}
}

func TestProfileToRBACProxyArgs_ModernProfile_NoCiphers(t *testing.T) {
	// TLS 1.3: cipher flag must be omitted (CipherSuites empty).
	args, err := profileToRBACProxyArgs(modernProfile)
	require.NoError(t, err)
	assert.Equal(t, "VersionTLS13", args.MinVersion)
	assert.Empty(t, args.CipherSuites, "TLS 1.3 must not set cipher suites")
}

func TestProfileToRBACProxyArgs_AllCiphersUnsupported_ReturnsError(t *testing.T) {
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"BOGUS_CIPHER_1", "BOGUS_CIPHER_2"},
	}
	_, err := profileToRBACProxyArgs(profile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 2 ciphers")
}

func TestProfileToRBACProxyArgs_SomeCiphersUnsupported_ReturnsError(t *testing.T) {
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "BOGUS_CIPHER"},
	}
	// Fail-closed: rejects the entire profile if any cipher is unsupported
	_, err := profileToRBACProxyArgs(profile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ciphers")
	assert.Contains(t, err.Error(), "BOGUS_CIPHER")
}

func TestProfileToRBACProxyArgs_EmptyCiphers_NoFlag(t *testing.T) {
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{},
	}
	args, err := profileToRBACProxyArgs(profile)
	require.NoError(t, err)
	assert.Empty(t, args.CipherSuites)
}

func TestProfileToRBACProxyArgs_HTTP2CipherValidation(t *testing.T) {
	tests := []struct {
		name       string
		minVersion configv1.TLSProtocolVersion
		ciphers    []string
		wantError  bool
	}{
		{
			name:       "TLS 1.2 AES-256-only ciphers fail",
			minVersion: configv1.VersionTLS12,
			ciphers:    []string{"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
			wantError:  true,
		},
		{
			name:       "TLS 1.2 AES-128-GCM cipher passes",
			minVersion: configv1.VersionTLS12,
			ciphers:    []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
		},
		{
			name:       "empty ciphers pass",
			minVersion: configv1.VersionTLS12,
		},
		{
			name:       "TLS 1.3 passes",
			minVersion: configv1.VersionTLS13,
			ciphers:    []string{"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := configv1.TLSProfileSpec{
				MinTLSVersion: tt.minVersion,
				Ciphers:       tt.ciphers,
			}
			args, err := profileToRBACProxyArgs(profile)
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "TLS profile with minimum version \"VersionTLS12\"")
				assert.Contains(t, err.Error(), "kube-rbac-proxy cannot serve HTTP/2 with that cipher list")
				assert.Nil(t, args)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, args)
		})
	}
}

func TestProfileToRBACProxyArgs_GroupsBecomeNumericIDs(t *testing.T) {
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Groups: []configv1.TLSGroup{
			configv1.TLSGroupSecP256r1,
			configv1.TLSGroupSecP384r1,
			configv1.TLSGroupSecP521r1,
			configv1.TLSGroupX25519,
			configv1.TLSGroupX25519MLKEM768,
		},
	}
	args, err := profileToRBACProxyArgs(profile)
	require.NoError(t, err)
	assert.Equal(t, "23,24,25,29,4588", args.CurvePreferences)
}

func TestProfileToRBACProxyArgs_AllGroupsUnsupported_ReturnsError(t *testing.T) {
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Groups:        []configv1.TLSGroup{"NOT_A_GROUP"},
	}
	_, err := profileToRBACProxyArgs(profile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 1 groups in TLS profile are unsupported by Go")
}

// --- cipherNamesToIANA ---

func TestCipherNamesToIANA_IANANamePassthrough(t *testing.T) {
	// Structurally IANA names (TLS_* + underscores) recognized by library-go pass through.
	ianaNames, unsupported := cipherNamesToIANA([]string{"TLS_AES_128_GCM_SHA256"})
	assert.Equal(t, []string{"TLS_AES_128_GCM_SHA256"}, ianaNames)
	assert.Empty(t, unsupported)
}

func TestCipherNamesToIANA_OpenSSLConversion(t *testing.T) {
	// OpenSSL names (hyphenated) are converted via OpenSSLToIANACipherSuites.
	ianaNames, unsupported := cipherNamesToIANA([]string{"ECDHE-RSA-AES128-GCM-SHA256"})
	require.Len(t, ianaNames, 1)
	assert.Equal(t, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", ianaNames[0])
	assert.Empty(t, unsupported)
}

func TestCipherNamesToIANA_Unsupported(t *testing.T) {
	// Unrecognized names are returned as unsupported.
	ianaNames, unsupported := cipherNamesToIANA([]string{"BOGUS_CIPHER"})
	assert.Empty(t, ianaNames)
	assert.Equal(t, []string{"BOGUS_CIPHER"}, unsupported)
}

func TestCipherNamesToIANA_MixedInput(t *testing.T) {
	// Mix of IANA passthrough, OpenSSL conversion, and unsupported.
	ianaNames, unsupported := cipherNamesToIANA([]string{
		"TLS_AES_128_GCM_SHA256",      // IANA name → passthrough
		"ECDHE-RSA-AES128-GCM-SHA256", // OpenSSL name → convert
		"BOGUS_CIPHER",                // Unrecognized → unsupported
	})
	require.Len(t, ianaNames, 2)
	assert.Equal(t, "TLS_AES_128_GCM_SHA256", ianaNames[0])
	assert.Equal(t, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", ianaNames[1])
	assert.Equal(t, []string{"BOGUS_CIPHER"}, unsupported)
}

// --- DSPAParams.ExtractParams integration ---

func TestExtractParams_StrictInvalidProfile_FailsClosed(t *testing.T) {
	// Invalid strict profile must fail reconcile (fail-closed) with wrapped error.
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := testutil.CreateEmptyDSPA()
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true}

	params.TLSProfileSpec = configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"BOGUS_CIPHER_1"}, // All unsupported → resolver error
	}
	params.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents

	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	require.Error(t, err, "ExtractParams must return error for invalid strict profile")
	assert.Contains(t, err.Error(), "failed to resolve kube-rbac-proxy TLS configuration")
	assert.Nil(t, params.KubeRBACProxyTLS, "KubeRBACProxyTLS must remain nil on error")
}

func TestExtractParams_NoOpinion_FallsBackToIntermediate(t *testing.T) {
	// NoOpinion policy should ignore configured profile and use Intermediate fallback.
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := testutil.CreateEmptyDSPA()
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true}

	params.TLSProfileSpec = modernProfile // Modern has different TLS 1.3-only ciphers
	params.TLSAdherence = configv1.TLSAdherencePolicyNoOpinion

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.KubeRBACProxyTLS)
	// NoOpinion falls back to Intermediate: TLS 1.2 minimum
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), params.KubeRBACProxyTLS.MinVersion)
}

// --- Template rendering ---

// getContainerArgs returns the args list for a named container in a Deployment unstructured object.
func getContainerArgs(t *testing.T, obj unstructured.Unstructured, containerName string) []string {
	t.Helper()
	containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	for _, c := range containers {
		cm, ok := c.(map[string]interface{})
		require.True(t, ok)
		if cm["name"] == containerName {
			argSlice, _ := cm["args"].([]interface{})
			var args []string
			for _, a := range argSlice {
				if s, ok := a.(string); ok {
					args = append(args, s)
				}
			}
			return args
		}
	}
	t.Fatalf("container %q not found", containerName)
	return nil
}

// dspaWithRoute returns a minimal DSPA with the API server route enabled so the
// kube-rbac-proxy container is rendered in the deployment template.
func dspaWithRoute() *dspav1.DataSciencePipelinesApplication {
	dspa := testutil.CreateEmptyDSPA()
	dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true, EnableRoute: true}
	return dspa
}

func TestAPIServerDeploymentTemplate_TLSFlagsPresent(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := dspaWithRoute()
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	// Pre-set TLS profile before ExtractParams so the resolver runs.
	params.TLSProfileSpec = intermediateProfile
	params.TLSProfileSpec.Groups = []configv1.TLSGroup{configv1.TLSGroupSecP256r1, configv1.TLSGroupX25519}
	params.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.KubeRBACProxyTLS, "KubeRBACProxyTLS must be set when TLSProfileSpec is provided")

	src, err := config.PathTemplateSource(reconciler.TemplatesPath+"apiserver/default/deployment.yaml.tmpl", params)
	require.NoError(t, err)
	objs, err := src.Parse()
	require.NoError(t, err)
	require.Len(t, objs, 1)

	for _, containerName := range []string{"kube-rbac-proxy", "kube-rbac-proxy-metrics"} {
		args := getContainerArgs(t, objs[0], containerName)
		assert.Contains(t, args, "--tls-min-version=VersionTLS12",
			"container %q must have --tls-min-version flag", containerName)
		// Intermediate profile has TLS 1.2 → cipher suites must be present.
		found := false
		for _, a := range args {
			if strings.HasPrefix(a, "--tls-cipher-suites=") {
				found = true
				assert.NotEqual(t, "--tls-cipher-suites=", a, "cipher suites must not be empty")
			}
		}
		assert.True(t, found, "container %q must have --tls-cipher-suites flag", containerName)
		assert.Contains(t, args, "--tls-curve-preferences=23,29",
			"container %q must have numeric curve preferences", containerName)
	}
}

func TestAPIServerDeploymentTemplate_TLS13_NoCipherFlag(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := dspaWithRoute()
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	// Modern profile = TLS 1.3 only → cipher flag must be absent.
	params.TLSProfileSpec = modernProfile
	params.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.KubeRBACProxyTLS)
	assert.Empty(t, params.KubeRBACProxyTLS.CipherSuites)

	src, err := config.PathTemplateSource(reconciler.TemplatesPath+"apiserver/default/deployment.yaml.tmpl", params)
	require.NoError(t, err)
	objs, err := src.Parse()
	require.NoError(t, err)
	require.Len(t, objs, 1)

	for _, containerName := range []string{"kube-rbac-proxy", "kube-rbac-proxy-metrics"} {
		args := getContainerArgs(t, objs[0], containerName)
		assert.Contains(t, args, "--tls-min-version=VersionTLS13",
			"container %q must have --tls-min-version=VersionTLS13", containerName)
		for _, a := range args {
			assert.False(t, strings.HasPrefix(a, "--tls-cipher-suites="),
				"container %q must NOT have --tls-cipher-suites flag for TLS 1.3, got: %s", containerName, a)
		}
	}
}

func TestMLMetadataEnvoyDeploymentTemplate_TLSFlagsPresent(t *testing.T) {
	// Verify the ML Metadata Envoy deployment applies TLS flags to kube-rbac-proxy.
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := testutil.CreateEmptyDSPA()
	dspa.Spec.MLMD = &dspav1.MLMD{Deploy: true}
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	// Pre-set TLS profile before ExtractParams so the resolver runs.
	params.TLSProfileSpec = intermediateProfile
	params.TLSProfileSpec.Groups = []configv1.TLSGroup{configv1.TLSGroupSecP256r1, configv1.TLSGroupX25519}
	params.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.KubeRBACProxyTLS)

	src, err := config.PathTemplateSource(reconciler.TemplatesPath+"ml-metadata/metadata-envoy.deployment.yaml.tmpl", params)
	require.NoError(t, err)
	objs, err := src.Parse()
	require.NoError(t, err)
	require.Len(t, objs, 1)

	// The metadata-envoy deployment has a single kube-rbac-proxy sidecar.
	args := getContainerArgs(t, objs[0], "kube-rbac-proxy")
	assert.Contains(t, args, "--tls-min-version=VersionTLS12",
		"kube-rbac-proxy must have --tls-min-version flag")
	// Intermediate profile has TLS 1.2 ciphers.
	found := false
	for _, a := range args {
		if strings.HasPrefix(a, "--tls-cipher-suites=") {
			found = true
			assert.NotEqual(t, "--tls-cipher-suites=", a, "cipher suites must not be empty")
		}
	}
	assert.True(t, found, "kube-rbac-proxy must have --tls-cipher-suites flag")
	assert.Contains(t, args, "--tls-curve-preferences=23,29")
}

func TestAPIServerDeploymentTemplate_NoTLSProfile_FlagsAbsent(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := dspaWithRoute()
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	// No TLS profile set → KubeRBACProxyTLS is nil → no TLS flags in template.
	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	assert.Nil(t, params.KubeRBACProxyTLS)

	src, err := config.PathTemplateSource(reconciler.TemplatesPath+"apiserver/default/deployment.yaml.tmpl", params)
	require.NoError(t, err)
	objs, err := src.Parse()
	require.NoError(t, err)
	require.Len(t, objs, 1)

	for _, containerName := range []string{"kube-rbac-proxy", "kube-rbac-proxy-metrics"} {
		args := getContainerArgs(t, objs[0], containerName)
		for _, a := range args {
			assert.False(t, strings.HasPrefix(a, "--tls-min-version"),
				"container %q must NOT have --tls-min-version when no profile set, got: %s", containerName, a)
		}
	}
}

// --- fetchLiveTLSProfile caller-level tests ---

func TestResolveKubeRBACProxyTLS_ZeroProfileFromFetchFallback_UsesIntermediate(t *testing.T) {
	// Zero profile falls back to Intermediate regardless of adherence.
	zeroProfile := configv1.TLSProfileSpec{}
	args, err := resolveKubeRBACProxyTLS(zeroProfile, configv1.TLSProfileIntermediateType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites)
}

func TestResolveKubeRBACProxyTLS_EmptyAdherence_TreatsAsNoOpinion(t *testing.T) {
	// Empty adherence is NoOpinion, falls back to Intermediate (not strict).
	tls13Profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS13,
		Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
	}
	args, err := resolveKubeRBACProxyTLS(tls13Profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicy(""))
	require.NoError(t, err)
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites)
}

// --- fetchLiveTLSProfile: shared-helper mapping ---

// newFetchLiveTLSClient builds a fake client serving config.openshift.io with
// the optional APIServer object and Get interceptor.
func newFetchLiveTLSClient(t *testing.T, interceptors interceptor.Funcs, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(interceptors).Build()
}

func TestFetchLiveTLSProfile_Ready(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	reconciler := &DSPAReconciler{
		Client:                newFetchLiveTLSClient(t, interceptor.Funcs{}, apiServer),
		HasOpenShiftConfigAPI: true,
	}

	profile, profileType, adherence, err := reconciler.fetchLiveTLSProfile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, *configv1.TLSProfiles[configv1.TLSProfileModernType], profile)
	assert.Equal(t, configv1.TLSProfileModernType, profileType)
	assert.Equal(t, configv1.TLSAdherencePolicyStrictAllComponents, adherence)
}

func TestFetchLiveTLSProfile_APIServerMissing_FallsBackToIntermediate(t *testing.T) {
	reconciler := &DSPAReconciler{
		Client:                newFetchLiveTLSClient(t, interceptor.Funcs{}),
		HasOpenShiftConfigAPI: true,
	}

	profile, profileType, adherence, err := reconciler.fetchLiveTLSProfile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, *configv1.TLSProfiles[configv1.TLSProfileIntermediateType], profile)
	assert.Equal(t, configv1.TLSProfileIntermediateType, profileType)
	assert.Equal(t, configv1.TLSAdherencePolicyNoOpinion, adherence)
}

func TestFetchLiveTLSProfile_TransientError_RequeuesWithBackoff(t *testing.T) {
	// Transient API errors must surface as errors so Reconcile returns them
	// and controller-runtime requeues with exponential backoff.
	reconciler := &DSPAReconciler{
		Client: newFetchLiveTLSClient(t, interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return apierrors.NewServiceUnavailable("apiserver restarting")
			},
		}),
		HasOpenShiftConfigAPI: true,
	}

	profile, profileType, adherence, err := reconciler.fetchLiveTLSProfile(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transient API error fetching APIServer")
	assert.Empty(t, profile.MinTLSVersion, "no profile may be guessed on transient errors")
	assert.Empty(t, profileType)
	assert.Empty(t, adherence)
}

func TestFetchLiveTLSProfile_HardError_RequeuesWithBackoff(t *testing.T) {
	reconciler := &DSPAReconciler{
		Client: newFetchLiveTLSClient(t, interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return apierrors.NewForbidden(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster", errors.New("denied"))
			},
		}),
		HasOpenShiftConfigAPI: true,
	}

	_, _, _, err := reconciler.fetchLiveTLSProfile(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `failed to get APIServer "cluster"`)
}

func TestFetchLiveTLSProfile_NonOpenShift_SkipsFetch(t *testing.T) {
	// With HasOpenShiftConfigAPI=false the reconciler must not hit the API at all.
	reconciler := &DSPAReconciler{
		Client: newFetchLiveTLSClient(t, interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return errors.New("fetch must be skipped on non-OpenShift clusters")
			},
		}),
		HasOpenShiftConfigAPI: false,
	}

	profile, profileType, adherence, err := reconciler.fetchLiveTLSProfile(context.Background())
	require.NoError(t, err)
	assert.Empty(t, profile.MinTLSVersion)
	assert.Empty(t, profileType)
	assert.Empty(t, adherence)
}

// Live APIServer fetch → both Deployment PodTemplates is covered by
// TestReconcile_AppliesTLSProfileAndAdherenceToBothDeployments in tls_workload_test.go.

// --- resolveAPIServerTLS ---

func TestResolveAPIServerTLS_NoOpinion_PreservesOperandDefaults(t *testing.T) {
	args, err := resolveAPIServerTLS(modernProfile, configv1.TLSProfileModernType, configv1.TLSAdherencePolicyNoOpinion)
	require.NoError(t, err)
	assert.Nil(t, args, "NoOpinion must leave the operand unconfigured")
}

func TestResolveAPIServerTLS_Legacy_PreservesOperandDefaults(t *testing.T) {
	args, err := resolveAPIServerTLS(intermediateProfile, configv1.TLSProfileIntermediateType, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly)
	require.NoError(t, err)
	assert.Nil(t, args, "Legacy adherence must leave the operand unconfigured")
}

func TestResolveAPIServerTLS_Strict_UsesProfile(t *testing.T) {
	args, err := resolveAPIServerTLS(intermediateProfile, configv1.TLSProfileIntermediateType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	require.NotNil(t, args)
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites)
}

func TestResolveAPIServerTLS_UnknownAdherence_TreatedAsStrict(t *testing.T) {
	args, err := resolveAPIServerTLS(modernProfile, configv1.TLSProfileModernType, configv1.TLSAdherencePolicy("SomeFuturePolicy"))
	require.NoError(t, err)
	require.NotNil(t, args)
	assert.Equal(t, "VersionTLS13", args.MinVersion)
	assert.Empty(t, args.CipherSuites)
}

func TestResolveAPIServerTLS_ZeroProfile_UsesIntermediate(t *testing.T) {
	args, err := resolveAPIServerTLS(configv1.TLSProfileSpec{}, configv1.TLSProfileIntermediateType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	require.NotNil(t, args)
	assert.Equal(t, string(intermediateProfile.MinTLSVersion), args.MinVersion)
	assert.NotEmpty(t, args.CipherSuites)
}

func TestResolveAPIServerTLS_Strict_TLS13_NoCiphers(t *testing.T) {
	args, err := resolveAPIServerTLS(modernProfile, configv1.TLSProfileModernType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	require.NotNil(t, args)
	assert.Equal(t, "VersionTLS13", args.MinVersion)
	assert.Empty(t, args.CipherSuites, "TLS 1.3 must not emit a cipher flag")
}

func TestResolveAPIServerTLS_Strict_OldProfile_FailsClosed(t *testing.T) {
	// The operand only accepts VersionTLS12/VersionTLS13. Old (TLS 1.0) under
	// Strict must fail the reconcile rather than silently downgrade to TLS 1.2.
	oldProfile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS10,
		Ciphers:       []string{"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA"},
	}
	_, err := resolveAPIServerTLS(oldProfile, configv1.TLSProfileOldType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support minimum TLS version")
}

func TestResolveAPIServerTLS_Strict_AllCiphersUnsupported_ReturnsError(t *testing.T) {
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"ECDHE-INVALID-CIPHER-1"},
	}
	_, err := resolveAPIServerTLS(profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestResolveAPIServerTLS_CustomProfileAllowsSHA256CBC(t *testing.T) {
	// SHA-256 CBC suites are runtime-supported and not prohibited; only RC4,
	// 3DES, and SHA-1 CBC suites are rejected in Custom profiles.
	for _, cipher := range []string{
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
		"ECDHE-RSA-AES128-SHA256",
	} {
		t.Run(cipher, func(t *testing.T) {
			profile := configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", cipher},
			}
			args, err := resolveAPIServerTLS(profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
			require.NoError(t, err)
			assert.Contains(t, args.CipherSuites, "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256")
		})
	}
}

func TestProfileToRBACProxyArgs_HybridOnlyGroupsBelowTLS13Rejected(t *testing.T) {
	// Hybrid ML-KEM groups only negotiate TLS 1.3: below a TLS 1.3 minimum
	// the projection must reject a group list with no TLS 1.2-capable curve.
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
		Groups:        []configv1.TLSGroup{"X25519MLKEM768"},
	}
	args, err := resolveKubeRBACProxyTLS(profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.Error(t, err)
	assert.Nil(t, args)
	assert.Contains(t, err.Error(), "no groups usable for TLS 1.2")

	// A static RSA key exchange cipher needs no curve, so the same group
	// list is allowed when one is configured.
	profile.Ciphers = []string{"ECDHE-RSA-AES128-GCM-SHA256", "TLS_RSA_WITH_AES_128_GCM_SHA256"}
	args, err = resolveKubeRBACProxyTLS(profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.NoError(t, err)
	assert.Contains(t, args.CurvePreferences, "4588")

	// The check must also apply to profiles without ciphers: no ciphers
	// means no static RSA exception, so the hybrid-only list still rejects.
	profile.Ciphers = nil
	args, err = resolveKubeRBACProxyTLS(profile, configv1.TLSProfileCustomType, configv1.TLSAdherencePolicyStrictAllComponents)
	require.Error(t, err)
	assert.Nil(t, args)
	assert.Contains(t, err.Error(), "no groups usable for TLS 1.2")
}

func TestProfileToAPIServerArgs_EmptyCiphers_NoFlag(t *testing.T) {
	// A TLS 1.2 profile with no ciphers must leave CipherSuites empty so the
	// flag is omitted and the operand applies Go's default suites.
	args, err := profileToAPIServerArgs(configv1.TLSProfileSpec{MinTLSVersion: configv1.VersionTLS12})
	require.NoError(t, err)
	assert.Equal(t, "VersionTLS12", args.MinVersion)
	assert.Empty(t, args.CipherSuites)
}

// --- operand template projection ---

func renderTemplate(t *testing.T, reconciler *DSPAReconciler, params *DSPAParams, tmplPath string) unstructured.Unstructured {
	t.Helper()
	src, err := config.PathTemplateSource(reconciler.TemplatesPath+tmplPath, params)
	require.NoError(t, err)
	objs, err := src.Parse()
	require.NoError(t, err)
	require.Len(t, objs, 1)
	return objs[0]
}

func assertOperandTLSArgs(t *testing.T, obj unstructured.Unstructured, containerName string, wantMinVersion string, wantCiphers string) {
	t.Helper()
	args := getContainerArgs(t, obj, containerName)
	if wantMinVersion == "" {
		for _, a := range args {
			assert.False(t, strings.HasPrefix(a, "--tlsMinVersion"),
				"container %q must NOT have TLS version flag, got: %s", containerName, a)
			assert.False(t, strings.HasPrefix(a, "--tlsCipherSuites"),
				"container %q must NOT have cipher suites flag, got: %s", containerName, a)
		}
		return
	}
	assert.Contains(t, args, "--tlsMinVersion="+wantMinVersion,
		"container %q must have --tlsMinVersion=%s", containerName, wantMinVersion)
	foundCipher := false
	for _, a := range args {
		if strings.HasPrefix(a, "--tlsCipherSuites=") {
			foundCipher = true
			if wantCiphers == "" {
				t.Errorf("container %q must NOT have --tlsCipherSuites, got: %s", containerName, a)
			} else {
				assert.Equal(t, "--tlsCipherSuites="+wantCiphers, a)
			}
		}
	}
	if wantCiphers != "" {
		assert.True(t, foundCipher, "container %q must have --tlsCipherSuites", containerName)
	}
}

func TestOperandTemplates_StrictIntermediate_FlagsPresent(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := dspaWithRoute()
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	params.TLSProfileSpec = intermediateProfile
	params.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.APIServerTLS)
	assert.Equal(t, "VersionTLS12", params.APIServerTLS.MinVersion)

	apiDep := renderTemplate(t, reconciler, params, "apiserver/default/deployment.yaml.tmpl")
	assertOperandTLSArgs(t, apiDep, "ds-pipeline-api-server", "VersionTLS12", params.APIServerTLS.CipherSuites)

	webhookDep := renderTemplate(t, reconciler, params, "webhook/deployment.yaml.tmpl")
	assertOperandTLSArgs(t, webhookDep, "ds-pipeline-webhook", "VersionTLS12", params.APIServerTLS.CipherSuites)
}

func TestOperandTemplates_NoOpinion_FlagsAbsent(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := dspaWithRoute()
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	params.TLSProfileSpec = intermediateProfile
	params.TLSAdherence = configv1.TLSAdherencePolicyNoOpinion

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.Nil(t, params.APIServerTLS, "NoOpinion must leave the operand unconfigured")

	apiDep := renderTemplate(t, reconciler, params, "apiserver/default/deployment.yaml.tmpl")
	assertOperandTLSArgs(t, apiDep, "ds-pipeline-api-server", "", "")

	webhookDep := renderTemplate(t, reconciler, params, "webhook/deployment.yaml.tmpl")
	assertOperandTLSArgs(t, webhookDep, "ds-pipeline-webhook", "", "")
}

func TestOperandTemplates_StrictTLS13_NoCipherFlag(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := dspaWithRoute()
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	params.TLSProfileSpec = modernProfile
	params.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.APIServerTLS)
	assert.Empty(t, params.APIServerTLS.CipherSuites)

	apiDep := renderTemplate(t, reconciler, params, "apiserver/default/deployment.yaml.tmpl")
	assertOperandTLSArgs(t, apiDep, "ds-pipeline-api-server", "VersionTLS13", "")

	webhookDep := renderTemplate(t, reconciler, params, "webhook/deployment.yaml.tmpl")
	assertOperandTLSArgs(t, webhookDep, "ds-pipeline-webhook", "VersionTLS13", "")
}

func TestOperandTemplates_StrictCustom_OpenSSLCiphersConverted(t *testing.T) {
	ctx, params, reconciler := CreateNewTestObjects()
	dspa := dspaWithRoute()
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	params.TLSProfileSpec = configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
	}
	params.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents

	require.NoError(t, params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log))
	require.NotNil(t, params.APIServerTLS)
	assert.Equal(t, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", params.APIServerTLS.CipherSuites)

	apiDep := renderTemplate(t, reconciler, params, "apiserver/default/deployment.yaml.tmpl")
	assertOperandTLSArgs(t, apiDep, "ds-pipeline-api-server", "VersionTLS12", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256")
}

func TestProfileToAPIServerArgs_NoHTTP2Cipher_ReturnsError(t *testing.T) {
	// The operand advertises h2 on its HTTP and webhook surfaces; a TLS 1.2
	// list without an AES-128-GCM suite cannot serve HTTP/2 and must be
	// rejected rather than deployed broken.
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384"},
	}
	_, err := profileToAPIServerArgs(profile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP/2-required AES-128-GCM cipher suite")
}

func TestProfileToAPIServerArgs_HTTP2CipherAccepted(t *testing.T) {
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-RSA-AES128-GCM-SHA256"},
	}
	args, err := profileToAPIServerArgs(profile)
	require.NoError(t, err)
	assert.Equal(t, "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", args.CipherSuites)
}
