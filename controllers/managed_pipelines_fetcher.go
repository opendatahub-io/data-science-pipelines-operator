package controllers

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const managedPipelinesJSONPath = "app/managed-pipelines.json"

type cachedManifest struct {
	names    map[string]bool
	imageRef string
}

// OCIManifestFetcher pulls managed-pipelines.json from a container image
// using the OCI registry API. Results are cached per image reference.
type OCIManifestFetcher struct {
	mu    sync.Mutex
	cache *cachedManifest
	log   logr.Logger
}

func NewOCIManifestFetcher(log logr.Logger) *OCIManifestFetcher {
	return &OCIManifestFetcher{log: log}
}

func (f *OCIManifestFetcher) FetchManifest(ctx context.Context, imageRef string) ([]byte, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference %q: %w", imageRef, err)
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %q: %w", imageRef, err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers for image %q: %w", imageRef, err)
	}

	for i := len(layers) - 1; i >= 0; i-- {
		layerReader, err := layers[i].Uncompressed()
		if err != nil {
			continue
		}

		data, found, tarErr := extractFileFromTar(layerReader, managedPipelinesJSONPath)
		if closeErr := layerReader.Close(); closeErr != nil {
			f.log.V(1).Info("Error closing layer reader", "layer", i, "error", closeErr)
		}
		if tarErr != nil {
			f.log.V(1).Info("Error reading layer", "layer", i, "error", tarErr)
			continue
		}
		if found {
			return data, nil
		}
	}

	return nil, fmt.Errorf("managed-pipelines.json not found in image %q", imageRef)
}

// FetchPipelineNames fetches and parses managed-pipelines.json from the image,
// returning the set of valid pipeline names. Results are cached per image reference.
func (f *OCIManifestFetcher) FetchPipelineNames(ctx context.Context, imageRef string) (map[string]bool, error) {
	f.mu.Lock()
	if f.cache != nil && f.cache.imageRef == imageRef {
		cached := f.cache.names
		f.mu.Unlock()
		return cached, nil
	}
	f.mu.Unlock()

	data, err := f.FetchManifest(ctx, imageRef)
	if err != nil {
		return nil, err
	}

	names, err := ParseManagedPipelinesManifest(data)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.cache = &cachedManifest{names: names, imageRef: imageRef}
	f.mu.Unlock()
	return names, nil
}

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
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, false, fmt.Errorf("failed to read %s from tar: %w", targetPath, err)
			}
			return data, true, nil
		}
	}
}
