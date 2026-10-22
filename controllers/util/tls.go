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
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	tlspkg "github.com/openshift/controller-runtime-common/pkg/tls"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TLSFetchState classifies the outcome of reading the cluster-wide TLS
// configuration from the config.openshift.io APIServer object.
type TLSFetchState int

const (
	// TLSFetchReady means the cluster APIServer object was read and its TLS
	// profile and adherence policy were resolved.
	TLSFetchReady TLSFetchState = iota
	// TLSFetchConfigUnavailable means the config.openshift.io API is not
	// served (non-OpenShift cluster).
	TLSFetchConfigUnavailable
	// TLSFetchMissing means the API is served but the cluster APIServer
	// object does not exist (bootstrap race).
	TLSFetchMissing
	// TLSFetchTransient means a transient API error was encountered; the
	// cluster TLS posture is unknown but the read can be retried.
	TLSFetchTransient
)

// TLSFetchResult is the outcome of a cluster TLS configuration fetch.
// ProfileSpec and Adherence are only populated when State is TLSFetchReady.
type TLSFetchResult struct {
	// State classifies the fetch outcome.
	State TLSFetchState
	// ProfileSpec is the TLS profile resolved from the cluster APIServer object.
	ProfileSpec configv1.TLSProfileSpec
	// Adherence is the TLS adherence policy read from the same object.
	Adherence configv1.TLSAdherencePolicy
	// Err is the underlying API error for non-Ready states; nil when Ready.
	Err error
}

// IsTransientOpenShiftAPIError reports API errors that are safe to retry while
// cluster TLS posture is still unknown. Matches the TLS Error Handling Contract:
// ServiceUnavailable, Timeout, ServerTimeout, TooManyRequests, DeadlineExceeded.
// InternalError is intentionally omitted: HTTP 500 on the profile GET means the
// posture is unknown, so Intermediate fallback would be a guess.
func IsTransientOpenShiftAPIError(err error) bool {
	return apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		errors.Is(err, context.DeadlineExceeded)
}

// FetchClusterTLSConfig reads the cluster APIServer object once and resolves
// both its TLS security profile and its TLS adherence policy, so callers never
// need a second API round-trip for the adherence policy.
//
// Error classification is shared by all callers: a missing config.openshift.io
// API (non-OpenShift cluster), a missing cluster APIServer object, and
// transient API errors are reported through the result state instead of an
// error, so each caller can apply its own fallback policy. Any other API
// error, and any failure to resolve the TLS profile itself, is returned as an
// error.
func FetchClusterTLSConfig(ctx context.Context, cli client.Client) (*TLSFetchResult, error) {
	apiServer := &configv1.APIServer{}
	nn := types.NamespacedName{Name: tlspkg.APIServerName}
	if err := cli.Get(ctx, nn, apiServer); err != nil {
		switch {
		case apimeta.IsNoMatchError(err):
			return &TLSFetchResult{State: TLSFetchConfigUnavailable, Err: err}, nil
		case apierrors.IsNotFound(err):
			return &TLSFetchResult{State: TLSFetchMissing, Err: err}, nil
		case IsTransientOpenShiftAPIError(err):
			return &TLSFetchResult{State: TLSFetchTransient, Err: err}, nil
		default:
			return nil, fmt.Errorf("failed to get APIServer %q: %w", tlspkg.APIServerName, err)
		}
	}

	profileSpec, err := tlspkg.GetTLSProfileSpec(apiServer.Spec.TLSSecurityProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to get TLS profile from APIServer %q: %w", tlspkg.APIServerName, err)
	}

	return &TLSFetchResult{
		State:       TLSFetchReady,
		ProfileSpec: profileSpec,
		Adherence:   apiServer.Spec.TLSAdherence,
	}, nil
}
