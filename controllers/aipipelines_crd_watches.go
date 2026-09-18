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
	"fmt"
	"sort"

	argoassets "github.com/opendatahub-io/data-science-pipelines-operator/config/argo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CRDWatchCaches contains one exact-name cache per CRD observed by the
// AIPipelines controllers. A cache can only apply one field selector per GVK,
// so separate caches avoid a cluster-wide CRD informer.
type CRDWatchCaches map[string]cache.Cache

// SetupAIPipelinesCRDWatchCaches creates and registers exact-name CRD caches
// with the manager. The returned caches can be shared by multiple controllers.
func SetupAIPipelinesCRDWatchCaches(mgr ctrl.Manager, namespace string) (CRDWatchCaches, error) {
	names, err := aipipelinesWatchedCRDNames(namespace)
	if err != nil {
		return nil, err
	}

	caches := make(CRDWatchCaches, len(names))
	for _, name := range names {
		crd := customResourceDefinition()
		watchCache, err := cache.New(mgr.GetConfig(), cache.Options{
			Scheme: mgr.GetScheme(),
			ByObject: map[client.Object]cache.ByObject{
				crd: {Field: fields.OneTermEqualSelector("metadata.name", name)},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create watch cache for CRD %s: %w", name, err)
		}
		if err := mgr.Add(watchCache); err != nil {
			return nil, fmt.Errorf("add watch cache for CRD %s: %w", name, err)
		}
		caches[name] = watchCache
	}
	return caches, nil
}

func aipipelinesWatchedCRDNames(namespace string) ([]string, error) {
	assets, err := argoassets.Objects(namespace)
	if err != nil {
		return nil, fmt.Errorf("load shared Argo assets for CRD watches: %w", err)
	}

	names := map[string]struct{}{prometheusRuleCRDName: {}}
	for _, asset := range assets {
		if asset.GetKind() == "CustomResourceDefinition" {
			names[asset.GetName()] = struct{}{}
		}
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func customResourceDefinition() *unstructured.Unstructured {
	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1")
	crd.SetKind("CustomResourceDefinition")
	return crd
}
