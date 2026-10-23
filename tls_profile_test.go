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
	assert.Contains(t, err.Error(), "all 2 ciphers in TLS profile are unsupported by Go")
}
