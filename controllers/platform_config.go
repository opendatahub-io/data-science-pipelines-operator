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
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	applicationsNamespaceEnv = "APPLICATIONS_NAMESPACE"
	platformConfigMapName    = "odh-aipipelines-config"
	platformVersionKey       = "platformVersion"
)

// resolvePlatformVersion reads the live platform handshake ConfigMap in
// modular mode. Standalone deployments retain their existing Viper-backed
// default and do not require platform resources.
func resolvePlatformVersion(ctx context.Context, reader client.Reader) (string, error) {
	if !config.AIPipelinesModuleControllerEnabled() {
		return config.ResolvedPlatformVersion(), nil
	}
	if reader == nil {
		return "", fmt.Errorf("platform configuration reader is not configured")
	}

	namespace := strings.TrimSpace(os.Getenv(applicationsNamespaceEnv))
	if namespace == "" {
		return "", fmt.Errorf("%s must be set in modular mode", applicationsNamespaceEnv)
	}

	platformConfig := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: platformConfigMapName, Namespace: namespace}
	if err := reader.Get(ctx, key, platformConfig); err != nil {
		return "", fmt.Errorf("read platform configuration ConfigMap %s: %w", key, err)
	}

	version := strings.TrimSpace(platformConfig.Data[platformVersionKey])
	if version == "" {
		return "", fmt.Errorf("platform configuration ConfigMap %s has no non-empty %q value", key, platformVersionKey)
	}
	return version, nil
}

func isPlatformConfigMap(object client.Object) bool {
	return object.GetName() == platformConfigMapName &&
		object.GetNamespace() == strings.TrimSpace(os.Getenv(applicationsNamespaceEnv))
}
