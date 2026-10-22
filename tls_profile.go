package main

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/util"
	configv1 "github.com/openshift/api/config/v1"
	tlspkg "github.com/openshift/controller-runtime-common/pkg/tls"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type tlsResult struct {
	TLSOpts            []func(*tls.Config)
	ProfileSpec        configv1.TLSProfileSpec
	HasOpenShiftConfig bool
	// Adherence is the TLS adherence policy read from the same APIServer
	// object as the profile. It is NoOpinion (the zero value) whenever it
	// could not be determined.
	Adherence configv1.TLSAdherencePolicy
	// AdherenceFetched reports whether the SecurityProfileWatcher should
	// track adherence changes. It mirrors HasOpenShiftConfig: the adherence
	// value defaults to NoOpinion when unknown, and the watcher detects the
	// real policy once the APIServer read recovers.
	AdherenceFetched bool
}

// fetchTLSProfile resolves the TLS configuration for the operator's webhook
// and metrics servers from the cluster-wide TLS security profile, using the
// shared cluster TLS fetch. Fallback and watcher-retry semantics:
//   - config.openshift.io API not served (non-OpenShift cluster): Intermediate
//     fallback and no SecurityProfileWatcher.
//   - cluster APIServer object missing (bootstrap race) or transient API
//     error: Intermediate fallback; the watcher still registers so the
//     operator self-heals (graceful restart) once the real profile is seen.
//   - any other API error, or a profile unusable by Go: hard error, startup fails.
func fetchTLSProfile(ctx context.Context, cli client.Client) (*tlsResult, error) {
	l := logf.FromContext(ctx).WithName("tls-profile")
	result := &tlsResult{}

	fetch, err := util.FetchClusterTLSConfig(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TLS profile: %w", err)
	}

	var profile configv1.TLSProfileSpec
	switch fetch.State {
	case util.TLSFetchReady:
		result.HasOpenShiftConfig = true
		result.Adherence = fetch.Adherence
		result.AdherenceFetched = true
		profile = fetch.ProfileSpec
	case util.TLSFetchConfigUnavailable:
		l.Info("config.openshift.io API not available (non-OpenShift cluster), using Intermediate TLS profile as fallback")
		profile = intermediateFallbackProfile()
	case util.TLSFetchMissing:
		l.Info("APIServer resource not found, using Intermediate TLS profile as fallback")
		// Mark OpenShift config as present so SecurityProfileWatcher still registers.
		// Bootstrap races can return NotFound even on OpenShift clusters.
		result.HasOpenShiftConfig = true
		result.AdherenceFetched = true
		profile = intermediateFallbackProfile()
	case util.TLSFetchTransient:
		l.Info("Transient API error reading TLS profile, using Intermediate TLS profile as fallback", "error", fetch.Err)
		// Mark OpenShift config as present so SecurityProfileWatcher still registers.
		// When the API recovers, the watcher will detect the real profile and trigger a restart.
		result.HasOpenShiftConfig = true
		result.AdherenceFetched = true
		profile = intermediateFallbackProfile()
	}

	tlsConfigFn, unsupportedCiphers := tlspkg.NewTLSConfigFromProfile(profile)
	if len(unsupportedCiphers) > 0 {
		l.Info("some ciphers from TLS profile are not supported by Go", "unsupported", unsupportedCiphers)
	}
	if len(profile.Ciphers) > 0 && len(unsupportedCiphers) == len(profile.Ciphers) {
		return nil, fmt.Errorf("all %d ciphers in TLS profile are unsupported by Go", len(profile.Ciphers))
	}
	result.TLSOpts = append(result.TLSOpts, tlsConfigFn)
	result.ProfileSpec = profile
	result.TLSOpts = append(result.TLSOpts, func(c *tls.Config) {
		c.NextProtos = []string{"h2", "http/1.1"}
	})

	return result, nil
}

// intermediateFallbackProfile returns a copy of the Intermediate TLS profile,
// the safe fallback whenever the cluster TLS posture cannot be determined.
func intermediateFallbackProfile() configv1.TLSProfileSpec {
	return *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
}
