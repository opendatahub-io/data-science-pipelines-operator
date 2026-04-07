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

package controllers

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

var (
	// errNotFound is returned by extractFileFromTar when the target file
	// is not present in the layer. Callers may continue to the next layer.
	errNotFound = errors.New("file not found in tar layer")

	// errWhiteout is returned by extractFileFromTar when the target file
	// was deleted by an OCI whiteout marker (.wh.<name> or .wh..wh..opq).
	// Callers must stop scanning lower layers to avoid surfacing stale content.
	errWhiteout = errors.New("file deleted by OCI whiteout")
)

// permanentError wraps errors representing definitive misconfigurations that
// will never self-resolve (invalid image ref, allowlist denial, file missing
// from image, malformed manifest). The controller uses errors.As to distinguish
// these from transient errors (network, registry unavailable) and blocks API
// server deployment only for permanent ones.
type permanentError struct {
	err error
}

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

const managedPipelinesJSONPath = "app/managed-pipelines.json"

const cacheTTL = 10 * time.Minute
const registryFetchTimeout = 30 * time.Second

type cacheEntry struct {
	names     map[string]bool
	fetchedAt time.Time
}

// OCIManifestFetcher pulls managed-pipelines.json from a container image
// using the OCI registry API. Results are cached per resolved image digest
// and expire after cacheTTL.
type OCIManifestFetcher struct {
	mu                sync.Mutex
	cache             map[string]cacheEntry
	nowFunc           func() time.Time
	log               logr.Logger
	AllowedRegistries []string
}

func NewOCIManifestFetcher(log logr.Logger, allowedRegistries []string) *OCIManifestFetcher {
	return &OCIManifestFetcher{
		cache:             make(map[string]cacheEntry),
		nowFunc:           time.Now,
		log:               log,
		AllowedRegistries: allowedRegistries,
	}
}

func (f *OCIManifestFetcher) isRegistryAllowed(registry string) bool {
	for _, allowed := range f.AllowedRegistries {
		if strings.EqualFold(registry, allowed) {
			return true
		}
	}
	return false
}

// resolveImageDigest parses the reference, validates the registry allowlist,
// fetches the image manifest (lightweight), and returns the image handle plus
// the resolved content digest string for cache lookups.
func (f *OCIManifestFetcher) resolveImageDigest(ctx context.Context, imageRef string) (oci.Image, string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, "", &permanentError{fmt.Errorf("invalid image reference %q: %w", imageRef, err)}
	}

	if len(f.AllowedRegistries) > 0 {
		registry := ref.Context().RegistryStr()
		if !f.isRegistryAllowed(registry) {
			return nil, "", &permanentError{fmt.Errorf("registry %q is not in the allowed list for managed pipelines", registry)}
		}
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(ctx))
	if err != nil {
		return nil, "", fmt.Errorf("failed to pull image %q: %w", imageRef, err)
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve digest for image %q: %w", imageRef, err)
	}

	return img, digest.String(), nil
}

// extractManifestFromImage iterates image layers from newest to oldest and
// returns the raw bytes of managed-pipelines.json. Layer read failures are
// returned immediately instead of falling through to older layers that may
// contain stale content.
func (f *OCIManifestFetcher) extractManifestFromImage(img oci.Image, imageRef string) ([]byte, error) {
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers for image %q: %w", imageRef, err)
	}

	for i := len(layers) - 1; i >= 0; i-- {
		layerReader, err := layers[i].Uncompressed()
		if err != nil {
			return nil, fmt.Errorf("failed to open layer %d for image %q: %w", i, imageRef, err)
		}

		data, tarErr := extractFileFromTar(layerReader, managedPipelinesJSONPath)
		if closeErr := layerReader.Close(); closeErr != nil {
			f.log.V(1).Info("Error closing layer reader", "layer", i, "error", closeErr)
		}
		switch {
		case tarErr == nil:
			return data, nil
		case errors.Is(tarErr, errNotFound):
			continue
		case errors.Is(tarErr, errWhiteout):
			return nil, &permanentError{fmt.Errorf("managed-pipelines.json not found in image %q (deleted by whiteout in layer %d)", imageRef, i)}
		default:
			return nil, fmt.Errorf(
				"failed to scan layer %d for %s in image %q: %w",
				i, managedPipelinesJSONPath, imageRef, tarErr,
			)
		}
	}

	return nil, &permanentError{fmt.Errorf("managed-pipelines.json not found in image %q", imageRef)}
}

func (f *OCIManifestFetcher) getCached(digestStr string) map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.cache[digestStr]
	if !ok {
		return nil
	}
	if f.nowFunc().Sub(entry.fetchedAt) > cacheTTL {
		delete(f.cache, digestStr)
		return nil
	}
	return copyStringBoolMap(entry.names)
}

// pruneExpiredLocked removes expired entries from f.cache.
// Caller must hold f.mu.
func (f *OCIManifestFetcher) pruneExpiredLocked(now time.Time) {
	for digest, entry := range f.cache {
		if now.Sub(entry.fetchedAt) > cacheTTL {
			delete(f.cache, digest)
		}
	}
}

func (f *OCIManifestFetcher) putCache(digestStr string, names map[string]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.nowFunc()
	f.pruneExpiredLocked(now)
	f.cache[digestStr] = cacheEntry{names: copyStringBoolMap(names), fetchedAt: now}
}

// FetchPipelineNames resolves the image digest (lightweight manifest fetch),
// checks the per-digest cache (entries expire after cacheTTL), and only on a
// cache miss downloads layers and extracts managed-pipelines.json. The
// returned map is a defensive copy safe for caller mutation.
func (f *OCIManifestFetcher) FetchPipelineNames(ctx context.Context, imageRef string) (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, registryFetchTimeout)
	defer cancel()

	img, digestStr, err := f.resolveImageDigest(ctx, imageRef)
	if err != nil {
		return nil, err
	}

	if cached := f.getCached(digestStr); cached != nil {
		return cached, nil
	}

	data, err := f.extractManifestFromImage(img, imageRef)
	if err != nil {
		return nil, err
	}

	names, err := ParseManagedPipelinesManifest(data)
	if err != nil {
		return nil, &permanentError{err}
	}

	f.putCache(digestStr, names)
	return copyStringBoolMap(names), nil
}

func copyStringBoolMap(m map[string]bool) map[string]bool {
	cp := make(map[string]bool, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

const maxManagedPipelinesManifestSize int64 = 1 << 20 // 1 MiB

// isWhiteoutFor reports whether entryName is an OCI whiteout marker that
// deletes targetPath (.wh.<name>) or makes its parent directory opaque
// (.wh..wh..opq), hiding all lower-layer content.
func isWhiteoutFor(entryName, targetPath string) bool {
	dir := path.Dir(targetPath)
	base := path.Base(targetPath)
	return entryName == dir+"/.wh."+base || entryName == dir+"/.wh..wh..opq"
}

// readBoundedTarEntry reads the current tar entry, rejecting files larger
// than maxManagedPipelinesManifestSize.
func readBoundedTarEntry(tr *tar.Reader, hdr *tar.Header) ([]byte, error) {
	if hdr.Size < 0 || hdr.Size > maxManagedPipelinesManifestSize {
		return nil, fmt.Errorf("%s size %d exceeds limit of %d bytes", hdr.Name, hdr.Size, maxManagedPipelinesManifestSize)
	}
	data, err := io.ReadAll(io.LimitReader(tr, maxManagedPipelinesManifestSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s from tar: %w", hdr.Name, err)
	}
	return data, nil
}

// extractFileFromTar scans a tar stream for targetPath. The entire stream is
// read because tar entry order is not guaranteed: a layer may contain both a
// whiteout marker and a replacement file. The file takes precedence (the
// whiteout only applies to lower layers). Returns:
//   - (data, nil)        — file found (even if a whiteout was also present)
//   - (nil, errNotFound) — file not present in this layer
//   - (nil, errWhiteout) — file deleted by OCI whiteout; stop scanning lower layers
//   - (nil, other error) — read failure
func extractFileFromTar(r io.Reader, targetPath string) ([]byte, error) {
	tr := tar.NewReader(r)
	var foundData []byte
	var foundWhiteout bool
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar read error: %w", err)
		}
		if isWhiteoutFor(hdr.Name, targetPath) {
			foundWhiteout = true
			continue
		}
		if hdr.Name == targetPath {
			data, readErr := readBoundedTarEntry(tr, hdr)
			if readErr != nil {
				return nil, readErr
			}
			foundData = data
		}
	}
	if foundData != nil {
		return foundData, nil
	}
	if foundWhiteout {
		return nil, errWhiteout
	}
	return nil, errNotFound
}
