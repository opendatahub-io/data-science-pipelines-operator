package controllers

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const managedPipelinesJSONPath = "app/managed-pipelines.json"

// OCIManifestFetcher pulls managed-pipelines.json from a container image
// using the OCI registry API. Results are cached per resolved image digest.
type OCIManifestFetcher struct {
	mu                sync.Mutex
	cache             map[string]map[string]bool // digest string -> pipeline names
	log               logr.Logger
	AllowedRegistries []string
}

func NewOCIManifestFetcher(log logr.Logger, allowedRegistries []string) *OCIManifestFetcher {
	return &OCIManifestFetcher{
		cache:             make(map[string]map[string]bool),
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
		return nil, "", fmt.Errorf("invalid image reference %q: %w", imageRef, err)
	}

	if len(f.AllowedRegistries) > 0 {
		registry := ref.Context().RegistryStr()
		if !f.isRegistryAllowed(registry) {
			return nil, "", fmt.Errorf("registry %q is not in the allowed list for managed pipelines", registry)
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

		data, found, tarErr := extractFileFromTar(layerReader, managedPipelinesJSONPath)
		if closeErr := layerReader.Close(); closeErr != nil {
			f.log.V(1).Info("Error closing layer reader", "layer", i, "error", closeErr)
		}
		if tarErr != nil {
			return nil, fmt.Errorf(
				"failed to scan layer %d for %s in image %q: %w",
				i, managedPipelinesJSONPath, imageRef, tarErr,
			)
		}
		if found {
			return data, nil
		}
	}

	return nil, fmt.Errorf("managed-pipelines.json not found in image %q", imageRef)
}

// FetchPipelineNames resolves the image digest (lightweight manifest fetch),
// checks the per-digest cache, and only on a cache miss downloads layers and
// extracts managed-pipelines.json. The returned map is a defensive copy safe
// for caller mutation.
func (f *OCIManifestFetcher) FetchPipelineNames(ctx context.Context, imageRef string) (map[string]bool, error) {
	img, digestStr, err := f.resolveImageDigest(ctx, imageRef)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	if cached, ok := f.cache[digestStr]; ok {
		f.mu.Unlock()
		return copyStringBoolMap(cached), nil
	}
	f.mu.Unlock()

	data, err := f.extractManifestFromImage(img, imageRef)
	if err != nil {
		return nil, err
	}

	names, err := ParseManagedPipelinesManifest(data)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.cache[digestStr] = names
	f.mu.Unlock()
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

func extractFileFromTar(r io.Reader, targetPath string) ([]byte, bool, error) {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("tar read error: %w", err)
		}
		if hdr.Name == targetPath {
			if hdr.Size < 0 || hdr.Size > maxManagedPipelinesManifestSize {
				return nil, false, fmt.Errorf("%s size %d exceeds limit of %d bytes", targetPath, hdr.Size, maxManagedPipelinesManifestSize)
			}
			data, err := io.ReadAll(io.LimitReader(tr, maxManagedPipelinesManifestSize))
			if err != nil {
				return nil, false, fmt.Errorf("failed to read %s from tar: %w", targetPath, err)
			}
			return data, true, nil
		}
	}
}
