//go:build test_all || test_unit

/*

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
	"encoding/json"
	"testing"

	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Client struct {
	Clientset kubernetes.Interface
}

func TestExtractParams_WithEmptyDSPA(t *testing.T) {
	dspa := testutil.CreateEmptyDSPA()
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)
}

func TestExtractParams_CABundle(t *testing.T) {

	ctx, _, client := CreateNewTestObjects()

	tt := []struct {
		msg                         string
		dsp                         *dspav1alpha1.DataSciencePipelinesApplication
		CustomCABundleRootMountPath string
		CustomSSLCertDir            *string
		PiplinesCABundleMountPath   string
		SSLCertFileEnv              string
		APICustomPemCerts           [][]byte
		CustomCABundle              *dspav1alpha1.CABundle
		ConfigMapPreReq             []*v1.ConfigMap
		errorMsg                    string
	}{
		{
			msg:                         "no bundle provided",
			dsp:                         testutil.CreateEmptyDSPA(),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            nil,
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           nil,
			CustomCABundle:              nil,
		},
		{
			msg:                         "user bundle provided, but no configmap",
			dsp:                         testutil.CreateDSPAWithAPIServerCABundle("testcakey", "testcaname"),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            nil,
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           nil,
			CustomCABundle:              nil,
			ConfigMapPreReq:             []*v1.ConfigMap{},
			errorMsg:                    "configmaps \"testcaname\" not found",
		},
		{
			msg:                         "user bundle provided",
			dsp:                         testutil.CreateDSPAWithAPIServerCABundle("testcakey", "testcaname"),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            strPtr("/dsp-custom-certs:/etc/ssl/certs:/etc/pki/tls/certs"),
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           [][]byte{[]byte("bundle-contents")},
			CustomCABundle:              &dspav1alpha1.CABundle{ConfigMapKey: "dsp-ca.crt", ConfigMapName: "dsp-trusted-ca-testdspa"},
			ConfigMapPreReq: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "testcaname", Namespace: "testnamespace"},
					Data:       map[string]string{"testcakey": "bundle-contents"},
				},
			},
		},
		{
			msg:                         "odh-trusted-ca bundle provided",
			dsp:                         testutil.CreateEmptyDSPA(),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            strPtr("/dsp-custom-certs:/etc/ssl/certs:/etc/pki/tls/certs"),
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           [][]byte{[]byte("odh-bundle-contents")},
			CustomCABundle:              &dspav1alpha1.CABundle{ConfigMapKey: "dsp-ca.crt", ConfigMapName: "dsp-trusted-ca-testdspa"},
			ConfigMapPreReq: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "odh-trusted-ca-bundle", Namespace: "testnamespace"},
					Data:       map[string]string{"testcakey": "odh-bundle-contents"},
				},
			},
		},
		{
			msg:                         "some empty values in odh-trusted-ca bundle provided",
			dsp:                         testutil.CreateEmptyDSPA(),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            strPtr("/dsp-custom-certs:/etc/ssl/certs:/etc/pki/tls/certs"),
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           [][]byte{[]byte("odh-bundle-contents-2")},
			CustomCABundle:              &dspav1alpha1.CABundle{ConfigMapKey: "dsp-ca.crt", ConfigMapName: "dsp-trusted-ca-testdspa"},
			ConfigMapPreReq: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "odh-trusted-ca-bundle", Namespace: "testnamespace"},
					Data:       map[string]string{"ca-bundle.crt": "", "odh-ca-bundle.crt": "odh-bundle-contents-2"},
				},
			},
		},
		{
			msg:                         "some empty values in odh-trusted-ca bundle provided",
			dsp:                         testutil.CreateEmptyDSPA(),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            nil,
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           nil,
			CustomCABundle:              nil,
			ConfigMapPreReq: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "odh-trusted-ca-bundle", Namespace: "testnamespace"},
					Data:       map[string]string{"ca-bundle.crt": "", "odh-ca-bundle.crt": ""},
				},
			},
		},
		{
			msg:                         "both user and odh-trusted-ca bundle provided",
			dsp:                         testutil.CreateDSPAWithAPIServerCABundle("testcakey", "testcaname"),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            strPtr("/dsp-custom-certs:/etc/ssl/certs:/etc/pki/tls/certs"),
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           [][]byte{[]byte("odh-bundle-contents"), []byte("bundle-contents")},
			CustomCABundle:              &dspav1alpha1.CABundle{ConfigMapKey: "dsp-ca.crt", ConfigMapName: "dsp-trusted-ca-testdspa"},
			ConfigMapPreReq: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "odh-trusted-ca-bundle", Namespace: "testnamespace"},
					Data:       map[string]string{"testcakey": "odh-bundle-contents"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "testcaname", Namespace: "testnamespace"},
					Data:       map[string]string{"testcakey": "bundle-contents"},
				},
			},
		},
		{
			msg:                         "both user and odh-trusted-ca bundle provided with non empty SSL_CERT_FILE",
			dsp:                         testutil.CreateDSPAWithAPIServerCABundle("testcakey", "testcaname"),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            strPtr("/dsp-custom-certs:/etc/ssl/certs:/etc/pki/tls/certs"),
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           [][]byte{[]byte("odh-bundle-contents"), []byte("bundle-contents"), []byte("dummycontent")},
			CustomCABundle:              &dspav1alpha1.CABundle{ConfigMapKey: "dsp-ca.crt", ConfigMapName: "dsp-trusted-ca-testdspa"},
			ConfigMapPreReq: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "odh-trusted-ca-bundle", Namespace: "testnamespace"},
					Data:       map[string]string{"testcakey": "odh-bundle-contents"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "testcaname", Namespace: "testnamespace"},
					Data:       map[string]string{"testcakey": "bundle-contents"},
				},
			},
			SSLCertFileEnv: "testdata/tls/dummy-ca-bundle.crt",
		},

		{
			msg:                         "pod to pod tls enabled",
			dsp:                         testutil.CreateDSPAWithAPIServerPodtoPodTlsEnabled(),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            strPtr("/dsp-custom-certs:/etc/ssl/certs:/etc/pki/tls/certs"),
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           [][]byte{[]byte("service-ca-contents")},
			CustomCABundle:              &dspav1alpha1.CABundle{ConfigMapKey: "dsp-ca.crt", ConfigMapName: "dsp-trusted-ca-testdspa"},
			ConfigMapPreReq: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "openshift-service-ca.crt", Namespace: "testnamespace"},
					Data:       map[string]string{"service-ca.crt": "service-ca-contents"},
				},
			},
		},
		{
			msg:                         "pod to pod tls enabled with sys certs",
			dsp:                         testutil.CreateDSPAWithAPIServerPodtoPodTlsEnabled(),
			CustomCABundleRootMountPath: "/dsp-custom-certs",
			CustomSSLCertDir:            strPtr("/dsp-custom-certs:/etc/ssl/certs:/etc/pki/tls/certs"),
			PiplinesCABundleMountPath:   "/dsp-custom-certs/dsp-ca.crt",
			APICustomPemCerts:           [][]byte{[]byte("service-ca-contents"), []byte("dummycontent")},
			CustomCABundle:              &dspav1alpha1.CABundle{ConfigMapKey: "dsp-ca.crt", ConfigMapName: "dsp-trusted-ca-testdspa"},
			ConfigMapPreReq: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "openshift-service-ca.crt", Namespace: "testnamespace"},
					Data:       map[string]string{"service-ca.crt": "service-ca-contents"},
				},
			},
			SSLCertFileEnv: "testdata/tls/dummy-ca-bundle.crt",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			if test.SSLCertFileEnv == "" {
				t.Setenv("SSL_CERT_FILE", "testdata/tls/empty-ca-bundle.crt")
			} else {
				t.Setenv("SSL_CERT_FILE", test.SSLCertFileEnv)
			}

			if test.ConfigMapPreReq != nil && len(test.ConfigMapPreReq) > 0 {
				for _, cfg := range test.ConfigMapPreReq {
					err := client.Create(ctx, cfg)
					assert.Nil(t, err)
				}
			}

			actualParams := &DSPAParams{}
			extractError := actualParams.ExtractParams(ctx, test.dsp, client.Client, client.Log)
			if test.errorMsg != "" {
				assert.Contains(t, extractError.Error(), test.errorMsg)
			} else {
				assert.Nil(t, extractError)
			}

			actualCustomCABundleRootMountPath := actualParams.CustomCABundleRootMountPath
			assert.Equal(t, test.CustomCABundleRootMountPath, actualCustomCABundleRootMountPath)

			actualCustomSSLCertDir := actualParams.CustomSSLCertDir
			assert.Equal(t, test.CustomSSLCertDir, actualCustomSSLCertDir)

			actualPipelinesCABundleMountPath := actualParams.PiplinesCABundleMountPath
			assert.Equal(t, test.PiplinesCABundleMountPath, actualPipelinesCABundleMountPath)

			actualAPICustomPemCerts := actualParams.APICustomPemCerts
			assert.Equal(t, test.APICustomPemCerts, actualAPICustomPemCerts)

			actualCustomCABundle := actualParams.CustomCABundle
			assert.Equal(t, test.CustomCABundle, actualCustomCABundle)

			if test.ConfigMapPreReq != nil && len(test.ConfigMapPreReq) > 0 {
				for _, cfg := range test.ConfigMapPreReq {
					err := client.Delete(ctx, cfg)
					assert.Nil(t, err)
				}
			}
		})
	}
}

func strPtr(v string) *string {
	return &v
}

func TestExtractParams_WithCustomKfpLauncherConfigMap(t *testing.T) {
	ctx, params, client := CreateNewTestObjects()
	cmDataExpected := map[string]string{
		"this-is-the-only-thing": "that-should-be-in-kfp-launcher-now",
	}
	cm := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-custom-kfp-launcher",
			Namespace: "testnamespace",
		},
		Data: cmDataExpected,
	}
	err := client.Create(ctx, &cm)
	require.Nil(t, err)

	dspa := testutil.CreateDSPAWithCustomKfpLauncherConfigMap("my-custom-kfp-launcher")
	err = params.ExtractParams(ctx, dspa, client.Client, client.Log)
	require.Nil(t, err)

	cmDataExpectedJson, err := json.Marshal(cmDataExpected)
	require.Equal(t, string(cmDataExpectedJson), params.CustomKfpLauncherConfigMapData)
}
