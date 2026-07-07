package main

import (
	"context"
	"crypto/tls"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	tlspkg "github.com/openshift/controller-runtime-common/pkg/tls"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type tlsResult struct {
	TLSOpts            []func(*tls.Config)
	ProfileSpec        configv1.TLSProfileSpec
	HasOpenShiftConfig bool
}

func fetchTLSProfile(ctx context.Context, cli client.Client) (*tlsResult, error) {
	l := logf.FromContext(ctx).WithName("tls-profile")
	result := &tlsResult{}

	profile, err := tlspkg.FetchAPIServerTLSProfile(ctx, cli)
	if err != nil {
		switch {
		case apimeta.IsNoMatchError(err):
			l.Info("config.openshift.io API not available (non-OpenShift cluster), using Intermediate TLS profile as fallback")
		case apierrors.IsNotFound(err):
			l.Info("APIServer resource not found, using Intermediate TLS profile as fallback")
		case apierrors.IsServiceUnavailable(err),
			apierrors.IsTimeout(err),
			apierrors.IsTooManyRequests(err):
			l.Info("Transient API error reading TLS profile, using Intermediate TLS profile as fallback", "error", err)
			// Mark OpenShift config as present so SecurityProfileWatcher still registers.
			// When the API recovers, the watcher will detect the real profile and trigger a restart.
			result.HasOpenShiftConfig = true
		default:
			return nil, fmt.Errorf("failed to fetch TLS profile: %w", err)
		}
		fallbackProfile := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		fallbackFn, _ := tlspkg.NewTLSConfigFromProfile(fallbackProfile)
		result.TLSOpts = append(result.TLSOpts, fallbackFn)
		result.ProfileSpec = fallbackProfile
	} else {
		result.HasOpenShiftConfig = true
		result.ProfileSpec = profile
		tlsConfigFn, unsupportedCiphers := tlspkg.NewTLSConfigFromProfile(profile)
		if len(unsupportedCiphers) > 0 {
			l.Info("some ciphers from TLS profile are not supported by Go", "unsupported", unsupportedCiphers)
		}
		if len(profile.Ciphers) > 0 && len(unsupportedCiphers) == len(profile.Ciphers) {
			return nil, fmt.Errorf("all %d ciphers in TLS profile are unsupported by Go", len(profile.Ciphers))
		}
		result.TLSOpts = append(result.TLSOpts, tlsConfigFn)
	}
	result.TLSOpts = append(result.TLSOpts, func(c *tls.Config) {
		c.NextProtos = []string{"h2", "http/1.1"}
	})

	return result, nil
}
