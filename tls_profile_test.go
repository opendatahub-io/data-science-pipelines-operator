//go:build test_all || test_unit

package main

import (
	"context"
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// newTLSTestClient builds a fake client serving config.openshift.io with the
// optional APIServer object and Get interceptor.
func newTLSTestClient(t *testing.T, objs ...client.Object) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
}

func applyTLSOpts(result *tlsResult) *tls.Config {
	cfg := &tls.Config{}
	for _, fn := range result.TLSOpts {
		fn(cfg)
	}
	return cfg
}

func TestFetchTLSProfile_IntermediateProfile(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.True(t, result.HasOpenShiftConfig)
	assert.True(t, result.AdherenceFetched)
	assert.NotEmpty(t, result.TLSOpts)

	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
}

func TestFetchTLSProfile_ModernProfile(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.True(t, result.HasOpenShiftConfig)

	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
	assert.Equal(t, []tls.CurveID{
		tls.X25519MLKEM768,
		tls.X25519,
		tls.CurveP256,
		tls.CurveP384,
	}, cfg.CurvePreferences)
}

func TestFetchTLSProfile_CustomSHA256CBCAllowedUnderStrict(t *testing.T) {
	// SHA-256 CBC suites are runtime-supported and not prohibited; only RC4,
	// 3DES, and SHA-1 CBC suites are rejected in Custom profiles.
	for _, cipher := range []string{
		"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
		"ECDHE-RSA-AES128-SHA256",
	} {
		t.Run(cipher, func(t *testing.T) {
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
							MinTLSVersion: configv1.VersionTLS12,
							Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", cipher},
						}},
					},
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			}
			cli := newTLSTestClient(t, apiServer).Build()

			result, err := fetchTLSProfile(context.Background(), cli)
			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

func TestFetchTLSProfile_CustomInsecureCiphersIgnoredWithoutStrict(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS12,
					Ciphers:       []string{"ECDHE-RSA-AES128-SHA256"},
				}},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.Equal(t, configv1.TLSProfileCustomType, result.ProfileType)

	opts, known := tlsOptsForAdherence(result, configv1.TLSAdherencePolicyNoOpinion)
	assert.True(t, known)
	cfg := &tls.Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
	assert.Empty(t, cfg.CipherSuites)
}

func TestFetchTLSProfile_OldProfileIsNotTreatedAsCustom(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType},
			TLSAdherence:       configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.Equal(t, configv1.TLSProfileOldType, result.ProfileType)
	assert.Equal(t, uint16(tls.VersionTLS10), applyTLSOpts(result).MinVersion)
}

func TestFetchTLSProfile_NoGroupsPreservesGoDefaults(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS12,
				}},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.Empty(t, applyTLSOpts(result).CurvePreferences)
}

func TestFetchTLSProfile_GroupsSetCurvePreferences(t *testing.T) {
	profile := configv1.TLSProfileSpec{
		MinTLSVersion: configv1.VersionTLS12,
		Groups:        []configv1.TLSGroup{configv1.TLSGroupSecP256r1, configv1.TLSGroupX25519},
	}
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type:   configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: profile},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	cfg := applyTLSOpts(result)
	assert.Equal(t, []tls.CurveID{tls.CurveP256, tls.X25519}, cfg.CurvePreferences)
}

func TestFetchTLSProfile_AllGroupsUnsupported(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS12,
					Groups:        []configv1.TLSGroup{"NOT_A_GROUP"},
				}},
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 1 groups in TLS profile are unsupported by Go")
}

func TestFetchTLSProfile_AdherenceReadFromSameObject(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.True(t, result.HasOpenShiftConfig)
	assert.True(t, result.AdherenceFetched)
	assert.Equal(t, configv1.TLSAdherencePolicyStrictAllComponents, result.Adherence)
}

func TestFetchTLSProfile_UnrecognizedError(t *testing.T) {
	// A fake client without configv1 registered returns a scheme error
	// (not IsNoMatchError, which only fires against a real API server).
	// This tests the fail-closed path for unrecognized errors.
	scheme := runtime.NewScheme()
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch TLS profile")
}

func TestFetchTLSProfile_APIServerNotFound(t *testing.T) {
	cli := newTLSTestClient(t).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	// NotFound on a served config API is a bootstrap race: the watcher must
	// still register so the real profile is picked up once the object exists.
	assert.True(t, result.HasOpenShiftConfig)
	assert.True(t, result.AdherenceFetched)
	assert.Equal(t, configv1.TLSAdherencePolicyNoOpinion, result.Adherence)

	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
}

func TestFetchTLSProfile_TransientError(t *testing.T) {
	// Transient API errors fall back to Intermediate and keep the watcher
	// registered so the operator self-heals when the API recovers.
	cli := newTLSTestClient(t).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return apierrors.NewServiceUnavailable("apiserver restarting")
		},
	}).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.True(t, result.HasOpenShiftConfig)
	assert.True(t, result.AdherenceFetched)
	assert.Equal(t, configv1.TLSAdherencePolicyNoOpinion, result.Adherence)

	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
}

func TestFetchTLSProfile_HardErrorFailsStartup(t *testing.T) {
	cli := newTLSTestClient(t).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return apierrors.NewForbidden(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster", assert.AnError)
		},
	}).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch TLS profile")
}

func TestFetchTLSProfile_NextProtosAlwaysSet(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)

	cfg := applyTLSOpts(result)
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
}

func TestFetchTLSProfile_AllCiphersUnsupported(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"BOGUS_CIPHER_1", "BOGUS_CIPHER_2"},
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 2 ciphers in TLS profile are unusable by this Go runtime")
}

func TestFetchTLSProfile_InvalidMinVersion(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: "VersionTLS99",
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid TLS profile minimum version "VersionTLS99"`)
}

func TestFetchTLSProfile_RestrictedCustomProfile(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)

	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	require.Len(t, cfg.CipherSuites, 1)
	assert.Equal(t, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, cfg.CipherSuites[0])
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
}

func TestFetchTLSProfile_NilProfileResolvesIntermediate(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.True(t, result.HasOpenShiftConfig)

	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
}

func TestFetchTLSProfile_TLS13CipherInTLS12Profile(t *testing.T) {
	// TLS 1.3-only suites in a TLS <=1.2 list are unusable for that minimum
	// version: they are filtered, and the usable suites still apply.
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256", "ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)

	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	require.Len(t, cfg.CipherSuites, 1)
	assert.Equal(t, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, cfg.CipherSuites[0])
}

func TestFetchTLSProfile_TLS13AllCiphersUnsupported(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384", "TLS_CHACHA20_POLY1305_SHA256"},
					},
				},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 3 ciphers in TLS profile are unusable by this Go runtime")
}

func TestFetchTLSProfile_EmptyCipherListKeepsGoDefaults(t *testing.T) {
	// A Custom TLS 1.2 profile with no ciphers must leave CipherSuites nil
	// so Go's default suites apply; an empty non-nil slice would disable
	// TLS 1.2 negotiation entirely.
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
					},
				},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)

	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.Nil(t, cfg.CipherSuites)
}

func TestTLSAdherenceApplies(t *testing.T) {
	cases := []struct {
		adherence configv1.TLSAdherencePolicy
		applies   bool
		known     bool
	}{
		{configv1.TLSAdherencePolicyNoOpinion, false, true},
		{configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly, false, true},
		{configv1.TLSAdherencePolicyStrictAllComponents, true, true},
		{configv1.TLSAdherencePolicy("Banana"), true, false},
	}
	for _, tc := range cases {
		applies, known := tlsAdherenceApplies(tc.adherence)
		assert.Equal(t, tc.applies, applies, "applies for %q", tc.adherence)
		assert.Equal(t, tc.known, known, "known for %q", tc.adherence)
	}
}

func TestTLSConfigForAdherence(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)

	apply := func(opts []func(*tls.Config)) *tls.Config {
		cfg := &tls.Config{}
		for _, fn := range opts {
			fn(cfg)
		}
		return cfg
	}

	// NoOpinion and Legacy preserve the manager defaults: ALPN only.
	for _, adherence := range []configv1.TLSAdherencePolicy{configv1.TLSAdherencePolicyNoOpinion, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly} {
		opts, known := tlsOptsForAdherence(result, adherence)
		assert.True(t, known)
		cfg := apply(opts)
		assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
		assert.Equal(t, uint16(0), cfg.MinVersion)
		assert.Empty(t, cfg.CipherSuites)
	}

	// Strict and unknown values apply the profile.
	for _, adherence := range []configv1.TLSAdherencePolicy{configv1.TLSAdherencePolicyStrictAllComponents, configv1.TLSAdherencePolicy("Banana")} {
		opts, known := tlsOptsForAdherence(result, adherence)
		if adherence == configv1.TLSAdherencePolicy("Banana") {
			assert.False(t, known)
		} else {
			assert.True(t, known)
		}
		cfg := apply(opts)
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		assert.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, cfg.CipherSuites)
		assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
	}
}

func TestFetchTLSProfile_NoOpinionSkipsAllUnsupportedGroups(t *testing.T) {
	// Non-enforcing adherence policies preserve the manager listener
	// defaults, so a group list that cannot be served must not prevent
	// the operator from starting — including an entirely unsupported one.
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS13,
					Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519MLKEM768},
				}},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
}

func TestFetchTLSProfile_GroupValidationSkippedUnderNoOpinion(t *testing.T) {
	// Non-enforcing adherence policies preserve the manager listener
	// defaults, so a group list that cannot be served must not prevent
	// the operator from starting.
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS12,
					Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519MLKEM768},
				}},
			},
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
}

func TestFetchTLSProfile_NoTLS12GroupUnderStrict(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS12,
					Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519MLKEM768},
					Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
				}},
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no groups usable for TLS 1.2")
}

func TestFetchTLSProfile_StaticRSACipherNeedsNoGroup(t *testing.T) {
	// Static RSA key exchange completes TLS 1.2 handshakes without an
	// elliptic curve, so the TLS 1.2 group requirement does not apply.
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
					MinTLSVersion: configv1.VersionTLS12,
					Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519MLKEM768},
					Ciphers:       []string{"TLS_RSA_WITH_AES_128_GCM_SHA256"},
				}},
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	cli := newTLSTestClient(t, apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	cfg := applyTLSOpts(result)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	require.Len(t, cfg.CipherSuites, 1)
	assert.Equal(t, tls.TLS_RSA_WITH_AES_128_GCM_SHA256, cfg.CipherSuites[0])
}
