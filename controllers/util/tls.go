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
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	tlspkg "github.com/openshift/controller-runtime-common/pkg/tls"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
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
	ProfileType configv1.TLSProfileType
	// Adherence is the TLS adherence policy read from the same object.
	Adherence configv1.TLSAdherencePolicy
	// Err is the underlying API error for non-Ready states; nil when Ready.
	Err error
}

func ProhibitedTLSCipherNames(ciphers []string) []string {
	insecureIDs := make(map[uint16]struct{})
	runtimeSuites := append(tls.CipherSuites(), tls.InsecureCipherSuites()...)
	for _, suite := range runtimeSuites {
		if isProhibitedTLSCipherName(suite.Name) {
			insecureIDs[suite.ID] = struct{}{}
		}
	}

	var rejected []string
	for _, cipher := range ciphers {
		name := strings.TrimSpace(cipher)
		originalName := strings.ToUpper(name)
		switch originalName {
		case "RC4-SHA":
			name = "TLS_RSA_WITH_RC4_128_SHA"
		case "ECDHE-RSA-RC4-SHA":
			name = "TLS_ECDHE_RSA_WITH_RC4_128_SHA"
		case "ECDHE-ECDSA-RC4-SHA":
			name = "TLS_ECDHE_ECDSA_WITH_RC4_128_SHA"
		}
		id, err := libgocrypto.CipherSuite(name)
		if err != nil {
			ianaNames := libgocrypto.OpenSSLToIANACipherSuites([]string{name})
			if len(ianaNames) != 1 {
				if isProhibitedTLSCipherName(originalName) {
					rejected = append(rejected, cipher)
				}
				continue
			}
			id, err = libgocrypto.CipherSuite(ianaNames[0])
			if err != nil {
				if isProhibitedTLSCipherName(ianaNames[0]) {
					rejected = append(rejected, cipher)
				}
				continue
			}
		}
		if _, ok := insecureIDs[id]; ok {
			rejected = append(rejected, cipher)
		}
	}
	return rejected
}

func isProhibitedTLSCipherName(name string) bool {
	name = strings.ToUpper(name)
	return strings.Contains(name, "RC4") || strings.Contains(name, "3DES") ||
		strings.Contains(name, "CBC3") || strings.HasSuffix(name, "_CBC_SHA") ||
		strings.HasSuffix(name, "-SHA")
}

// IsTransientOpenShiftAPIError reports API and connection errors that are safe
// to retry while cluster TLS posture is still unknown.
func IsTransientOpenShiftAPIError(err error) bool {
	// Explicitly transient API statuses first.
	if apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		apierrors.IsInternalError(err) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Reject other API-status errors before checking transport predicates.
	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		return false
	}
	// Transport-level errors for non-API-status failures.
	var netErr net.Error
	return utilnet.IsConnectionRefused(err) ||
		utilnet.IsConnectionReset(err) ||
		utilnet.IsProbableEOF(err) ||
		utilnet.IsHTTP2ConnectionLost(err) ||
		(errors.As(err, &netErr) && netErr.Timeout())
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
	profileType := configv1.TLSProfileIntermediateType
	if apiServer.Spec.TLSSecurityProfile != nil && apiServer.Spec.TLSSecurityProfile.Type != "" {
		profileType = apiServer.Spec.TLSSecurityProfile.Type
	}

	return &TLSFetchResult{
		State:       TLSFetchReady,
		ProfileSpec: profileSpec,
		ProfileType: profileType,
		Adherence:   apiServer.Spec.TLSAdherence,
	}, nil
}
