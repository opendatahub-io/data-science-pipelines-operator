//go:build test_all || test_unit

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClassifyTLSAdherenceFetchError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		action tlsAdherenceFetchAction
	}{
		{
			name:   "not found",
			err:    apierrors.NewNotFound(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster"),
			action: tlsAdherenceFetchRetry,
		},
		{
			name:   "internal error",
			err:    apierrors.NewInternalError(errors.New("apiserver unavailable")),
			action: tlsAdherenceFetchRetry,
		},
		{
			name:   "deadline exceeded",
			err:    context.DeadlineExceeded,
			action: tlsAdherenceFetchRetry,
		},
		{
			name:   "timeout",
			err:    apierrors.NewTimeoutError("timed out", 0),
			action: tlsAdherenceFetchRetry,
		},
		{
			name:   "forbidden",
			err:    apierrors.NewForbidden(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster", errors.New("denied")),
			action: tlsAdherenceFetchFatal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.action, classifyTLSAdherenceFetchError(tt.err))
		})
	}
}

func TestIsTransientOpenShiftAPIError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{name: "internal error", err: apierrors.NewInternalError(errors.New("apiserver unavailable")), transient: true},
		{name: "deadline exceeded", err: context.DeadlineExceeded, transient: true},
		{name: "timeout", err: apierrors.NewTimeoutError("timed out", 0), transient: true},
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster", errors.New("denied")), transient: false},
		{name: "not found", err: apierrors.NewNotFound(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster"), transient: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.transient, isTransientOpenShiftAPIError(tt.err))
		})
	}
}

func TestFetchTLSProfile_IntermediateProfile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))

	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.True(t, result.HasOpenShiftConfig)
	assert.NotEmpty(t, result.TLSOpts)

	cfg := &tls.Config{}
	for _, fn := range result.TLSOpts {
		fn(cfg)
	}
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
}

func TestFetchTLSProfile_ModernProfile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))

	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.True(t, result.HasOpenShiftConfig)

	cfg := &tls.Config{}
	for _, fn := range result.TLSOpts {
		fn(cfg)
	}
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
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
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))

	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)
	assert.False(t, result.HasOpenShiftConfig)

	cfg := &tls.Config{}
	for _, fn := range result.TLSOpts {
		fn(cfg)
	}
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
}

func TestFetchTLSProfile_NextProtosAlwaysSet(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))

	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

	result, err := fetchTLSProfile(context.Background(), cli)
	require.NoError(t, err)

	cfg := &tls.Config{}
	for _, fn := range result.TLSOpts {
		fn(cfg)
	}
	assert.Equal(t, []string{"h2", "http/1.1"}, cfg.NextProtos)
}

func TestFetchTLSProfile_AllCiphersUnsupported(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))

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
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()

	_, err := fetchTLSProfile(context.Background(), cli)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 2 ciphers in TLS profile are unsupported by Go")
}
