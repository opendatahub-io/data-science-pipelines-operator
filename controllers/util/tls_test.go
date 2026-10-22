//go:build test_all || test_unit

/*
Copyright 2026.

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

package util

import (
	"context"
	"errors"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestIsTransientOpenShiftAPIError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{name: "internal error", err: apierrors.NewInternalError(errors.New("apiserver unavailable")), transient: false},
		{name: "deadline exceeded", err: context.DeadlineExceeded, transient: true},
		{name: "timeout", err: apierrors.NewTimeoutError("timed out", 0), transient: true},
		{name: "service unavailable", err: apierrors.NewServiceUnavailable("apiserver restarting"), transient: true},
		{name: "too many requests", err: apierrors.NewTooManyRequestsError("throttled"), transient: true},
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster", errors.New("denied")), transient: false},
		{name: "not found", err: apierrors.NewNotFound(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster"), transient: false},
		{name: "nil error", err: nil, transient: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.transient, IsTransientOpenShiftAPIError(tt.err))
		})
	}
}

// newTLSFakeClient builds a fake client serving config.openshift.io with the
// optional APIServer object and Get interceptor.
func newTLSFakeClient(t *testing.T, objs ...client.Object) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, configv1.Install(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
}

func TestFetchClusterTLSConfig_Ready(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	cli := newTLSFakeClient(t, apiServer).Build()

	fetch, err := FetchClusterTLSConfig(context.Background(), cli)
	require.NoError(t, err)
	require.NotNil(t, fetch)

	assert.Equal(t, TLSFetchReady, fetch.State)
	assert.Equal(t, *configv1.TLSProfiles[configv1.TLSProfileModernType], fetch.ProfileSpec)
	assert.Equal(t, configv1.TLSAdherencePolicyStrictAllComponents, fetch.Adherence)
	assert.NoError(t, fetch.Err)
}

func TestFetchClusterTLSConfig_Ready_DefaultsToIntermediateWhenProfileUnset(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
	}
	cli := newTLSFakeClient(t, apiServer).Build()

	fetch, err := FetchClusterTLSConfig(context.Background(), cli)
	require.NoError(t, err)
	require.NotNil(t, fetch)

	assert.Equal(t, TLSFetchReady, fetch.State)
	assert.Equal(t, *configv1.TLSProfiles[configv1.TLSProfileIntermediateType], fetch.ProfileSpec)
	assert.Equal(t, configv1.TLSAdherencePolicyNoOpinion, fetch.Adherence)
}

func TestFetchClusterTLSConfig_APIServerMissing(t *testing.T) {
	cli := newTLSFakeClient(t).Build()

	fetch, err := FetchClusterTLSConfig(context.Background(), cli)
	require.NoError(t, err)
	require.NotNil(t, fetch)

	assert.Equal(t, TLSFetchMissing, fetch.State)
	assert.True(t, apierrors.IsNotFound(fetch.Err), "underlying error should be NotFound, got: %v", fetch.Err)
	assert.Empty(t, fetch.ProfileSpec.MinTLSVersion)
	assert.Empty(t, fetch.Adherence)
}

func TestFetchClusterTLSConfig_ConfigUnavailable(t *testing.T) {
	cli := newTLSFakeClient(t).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return &apimeta.NoResourceMatchError{
				PartialResource: schema.GroupVersionResource{Group: "config.openshift.io", Resource: "apiservers"},
			}
		},
	}).Build()

	fetch, err := FetchClusterTLSConfig(context.Background(), cli)
	require.NoError(t, err)
	require.NotNil(t, fetch)

	assert.Equal(t, TLSFetchConfigUnavailable, fetch.State)
	assert.True(t, apimeta.IsNoMatchError(fetch.Err), "underlying error should be a NoMatch error, got: %v", fetch.Err)
}

func TestFetchClusterTLSConfig_TransientError(t *testing.T) {
	cli := newTLSFakeClient(t).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return apierrors.NewServiceUnavailable("apiserver restarting")
		},
	}).Build()

	fetch, err := FetchClusterTLSConfig(context.Background(), cli)
	require.NoError(t, err)
	require.NotNil(t, fetch)

	assert.Equal(t, TLSFetchTransient, fetch.State)
	require.Error(t, fetch.Err)
	assert.True(t, IsTransientOpenShiftAPIError(fetch.Err))
}

func TestFetchClusterTLSConfig_HardError(t *testing.T) {
	cli := newTLSFakeClient(t).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return apierrors.NewForbidden(schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"}, "cluster", errors.New("denied"))
		},
	}).Build()

	fetch, err := FetchClusterTLSConfig(context.Background(), cli)
	require.Error(t, err)
	assert.Nil(t, fetch)
	assert.Contains(t, err.Error(), `failed to get APIServer "cluster"`)
	assert.True(t, apierrors.IsForbidden(err), "hard errors must preserve the API error for callers, got: %v", err)
}

func TestFetchClusterTLSConfig_InternalErrorIsHard(t *testing.T) {
	// InternalError is intentionally NOT transient: HTTP 500 on the profile
	// GET means the posture is unknown, so Intermediate fallback would be a guess.
	cli := newTLSFakeClient(t).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return apierrors.NewInternalError(errors.New("apiserver unavailable"))
		},
	}).Build()

	fetch, err := FetchClusterTLSConfig(context.Background(), cli)
	require.Error(t, err)
	assert.Nil(t, fetch)
	assert.True(t, apierrors.IsInternalError(err))
}

func TestFetchClusterTLSConfig_InvalidProfile(t *testing.T) {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				// Custom profile with a nil Custom spec cannot be resolved.
				Type: configv1.TLSProfileCustomType,
			},
		},
	}
	cli := newTLSFakeClient(t, apiServer).Build()

	fetch, err := FetchClusterTLSConfig(context.Background(), cli)
	require.Error(t, err)
	assert.Nil(t, fetch)
	assert.Contains(t, err.Error(), "failed to get TLS profile from APIServer")
}
