//go:build test_all || test_unit

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
	"testing"

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolvePlatformVersionFromLiveConfigMap(t *testing.T) {
	viper.Set(config.EnableAIPipelinesModuleControllerConfigName, true)
	t.Cleanup(viper.Reset)
	t.Setenv(applicationsNamespaceEnv, "redhat-ods-applications")

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: platformConfigMapName, Namespace: "redhat-ods-applications"},
		Data:       map[string]string{platformVersionKey: " 3.6.0 "},
	}).Build()

	version, err := resolvePlatformVersion(context.Background(), reader)
	require.NoError(t, err)
	require.Equal(t, "3.6.0", version)
}

func TestResolvePlatformVersionRejectsMissingValue(t *testing.T) {
	viper.Set(config.EnableAIPipelinesModuleControllerConfigName, true)
	t.Cleanup(viper.Reset)
	t.Setenv(applicationsNamespaceEnv, "redhat-ods-applications")

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: platformConfigMapName, Namespace: "redhat-ods-applications"},
	}).Build()

	_, err := resolvePlatformVersion(context.Background(), reader)
	require.ErrorContains(t, err, platformVersionKey)
}

func TestResolvePlatformVersionStandaloneFallback(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("DSPO.PlatformVersion", "v3.6.0-standalone")

	version, err := resolvePlatformVersion(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "v3.6.0-standalone", version)
}
