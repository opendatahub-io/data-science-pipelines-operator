package main

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/util"
	configv1 "github.com/openshift/api/config/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type tlsResult struct {
	TLSOpts []func(*tls.Config)
	// NextProtosOpt is the standalone ALPN callback. Under adherence
	// policies that do not enforce the cluster profile, it is the only
	// callback applied so the manager listener defaults are preserved.
	NextProtosOpt      func(*tls.Config)
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
//     error: Intermediate fallback and NoOpinion adherence; the watcher still
//     registers so the operator self-heals (graceful restart) once the real
//     profile is seen.
//   - any other API error, an invalid profile, or a profile unusable by the
//     running Go runtime: hard error, startup fails.
func fetchTLSProfile(ctx context.Context, cli client.Client) (*tlsResult, error) {
	l := logf.FromContext(ctx).WithName("tls-profile")
	result := &tlsResult{NextProtosOpt: setNextProtos}

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

	tlsConfigFn, unsupportedCiphers, err := newTLSConfigFromProfile(profile)
	if err != nil {
		return nil, err
	}
	if len(unsupportedCiphers) > 0 {
		l.Info("some ciphers from TLS profile are unusable by this Go runtime", "unsupported", unsupportedCiphers)
	}
	result.TLSOpts = append(result.TLSOpts, tlsConfigFn)
	result.ProfileSpec = profile
	result.TLSOpts = append(result.TLSOpts, result.NextProtosOpt)

	return result, nil
}

// intermediateFallbackProfile returns a copy of the Intermediate TLS profile,
// the safe fallback whenever the cluster TLS posture cannot be determined.
func intermediateFallbackProfile() configv1.TLSProfileSpec {
	return *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
}

func setNextProtos(c *tls.Config) {
	c.NextProtos = []string{"h2", "http/1.1"}
}

// newTLSConfigFromProfile validates the profile against the running Go TLS
// implementation before building a callback. It avoids the shared helper's
// TLSVersionOrDie so unknown Custom minimum versions fail with an error
// instead of panicking, and filters cipher suites that this runtime cannot
// negotiate for TLS 1.2 and below.
func newTLSConfigFromProfile(profile configv1.TLSProfileSpec) (func(*tls.Config), []string, error) {
	minVersion, err := libgocrypto.TLSVersion(string(profile.MinTLSVersion))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid TLS profile minimum version %q: %w", profile.MinTLSVersion, err)
	}

	var cipherSuites []uint16
	var unsupported []string
	if minVersion == tls.VersionTLS13 {
		// TLS 1.3 cipher suites are not configurable in Go. Validate the
		// names to preserve the existing rejection of unsupported entries,
		// but do not apply them.
		_, unsupported = runtimeCipherSuites(profile.Ciphers, false)
		if len(profile.Ciphers) > 0 && len(unsupported) == len(profile.Ciphers) {
			return nil, unsupported, fmt.Errorf("all %d ciphers in TLS profile are unusable by this Go runtime", len(profile.Ciphers))
		}
	} else {
		cipherSuites, unsupported = runtimeCipherSuites(profile.Ciphers, true)
		if len(profile.Ciphers) > 0 && len(cipherSuites) == 0 {
			return nil, unsupported, fmt.Errorf("all %d ciphers in TLS profile are unusable by this Go runtime", len(profile.Ciphers))
		}
	}

	return func(c *tls.Config) {
		c.MinVersion = minVersion
		if minVersion != tls.VersionTLS13 && len(cipherSuites) > 0 {
			c.CipherSuites = cipherSuites
		}
	}, unsupported, nil
}

// runtimeCipherSuites maps cipher names to the IDs this Go runtime can
// negotiate. Unknown names and suites the runtime cannot use (including
// TLS 1.3-only suites when requireTLS12 is set) are reported as unsupported.
func runtimeCipherSuites(ciphers []string, requireTLS12 bool) ([]uint16, []string) {
	runtimeSuites := make(map[uint16]*tls.CipherSuite)
	for _, suite := range tls.CipherSuites() {
		runtimeSuites[suite.ID] = suite
	}
	for _, suite := range tls.InsecureCipherSuites() {
		runtimeSuites[suite.ID] = suite
	}

	cipherSuites := make([]uint16, 0, len(ciphers))
	seen := make(map[uint16]struct{}, len(ciphers))
	var unsupported []string
	for _, name := range ciphers {
		id, ok := cipherSuiteID(name)
		if !ok {
			unsupported = append(unsupported, name)
			continue
		}
		suite, ok := runtimeSuites[id]
		if !ok || (requireTLS12 && !supportsTLS12OrBelow(suite)) {
			unsupported = append(unsupported, name)
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cipherSuites = append(cipherSuites, id)
	}

	return cipherSuites, unsupported
}

// cipherSuiteID resolves a cipher name, accepting both IANA and OpenSSL
// (library-go) spellings.
func cipherSuiteID(name string) (uint16, bool) {
	if id, err := libgocrypto.CipherSuite(name); err == nil {
		return id, true
	}

	ianaNames := libgocrypto.OpenSSLToIANACipherSuites([]string{name})
	if len(ianaNames) == 1 {
		if id, err := libgocrypto.CipherSuite(ianaNames[0]); err == nil {
			return id, true
		}
	}

	return 0, false
}

func supportsTLS12OrBelow(suite *tls.CipherSuite) bool {
	for _, version := range suite.SupportedVersions {
		if version <= tls.VersionTLS12 {
			return true
		}
	}
	return false
}

// tlsAdherenceApplies reports whether the cluster profile should be enforced
// and whether the adherence value is known. NoOpinion and
// LegacyAdheringComponentsOnly preserve the manager listener defaults; Strict
// enforces the profile; unknown values enforce the profile and are logged by
// the caller.
func tlsAdherenceApplies(adherence configv1.TLSAdherencePolicy) (bool, bool) {
	switch adherence {
	case configv1.TLSAdherencePolicyNoOpinion, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly:
		return false, true
	case configv1.TLSAdherencePolicyStrictAllComponents:
		return true, true
	default:
		return true, false
	}
}

// tlsOptsForAdherence returns the TLS callbacks to apply for the adherence
// policy: the full profile callbacks when the profile is enforced, otherwise
// only the ALPN callback so manager listener defaults are preserved.
func tlsOptsForAdherence(result *tlsResult, adherence configv1.TLSAdherencePolicy) ([]func(*tls.Config), bool) {
	applies, known := tlsAdherenceApplies(adherence)
	if applies {
		return result.TLSOpts, known
	}
	if result.NextProtosOpt == nil {
		return nil, known
	}
	return []func(*tls.Config){result.NextProtosOpt}, known
}
