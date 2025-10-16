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
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7/pkg/credentials"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDeployStorage(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with deployed Minio Object Storage
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: true,
					Image:  "someimage",
					Resources: &dspav1.ResourceRequirements{ //TODO: fails without this block.  Why?
						Requests: &dspav1.Resources{
							CPU:    resource.MustParse("250m"),
							Memory: resource.MustParse("500Mi"),
						},
						Limits: &dspav1.Resources{
							CPU:    resource.MustParse("500m"),
							Memory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileStorage(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Assert ObjectStorage Route doesn't exist
	route := &routev1.Route{}
	created, err = reconciler.IsResourceCreated(ctx, route, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}

func TestDeployStorageWithExternalRouteEnabled(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with deployed Minio Object Storage
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck:  false,
				EnableExternalRoute: true,
				Minio: &dspav1.Minio{
					Deploy: true,
					Image:  "someimage",
					Resources: &dspav1.ResourceRequirements{ //TODO: fails without this block.  Why?
						Requests: &dspav1.Resources{
							CPU:    resource.MustParse("250m"),
							Memory: resource.MustParse("500Mi"),
						},
						Limits: &dspav1.Resources{
							CPU:    resource.MustParse("500m"),
							Memory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Assert ObjectStorage Route doesn't yet exist
	route := &routev1.Route{}
	created, err = reconciler.IsResourceCreated(ctx, route, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileStorage(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment now exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)

	// Assert ObjectStorage Route now exists
	route = &routev1.Route{}
	created, err = reconciler.IsResourceCreated(ctx, route, expectedStorageName, testNamespace)
	assert.True(t, created)
	assert.Nil(t, err)
}

func TestDontDeployStorage(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with non-deployed Minio Object Storage
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "someimage",
				},
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileStorage(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment still doesn't exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}

func TestDefaultDeployBehaviorStorage(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with deployed Minio Object Storage
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			Database: &dspav1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
			},
		},
	}

	// Enrich DSPA with name+namespace
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params
	ctx, params, reconciler := CreateNewTestObjects()
	err := params.ExtractParams(ctx, dspa, reconciler.Client, reconciler.Log)
	assert.NotNil(t, err) // DSPAParams should throw an error if no objstore is provided

	// Assert ObjectStorage Deployment doesn't yet exist
	deployment := &appsv1.Deployment{}
	created, err := reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)

	// Run test reconciliation
	err = reconciler.ReconcileStorage(ctx, dspa, params)
	assert.Nil(t, err)

	// Assert ObjectStorage Deployment still doesn't exists
	deployment = &appsv1.Deployment{}
	created, err = reconciler.IsResourceCreated(ctx, deployment, expectedStorageName, testNamespace)
	assert.False(t, created)
	assert.Nil(t, err)
}

func TestIsDatabaseAccessibleTrue(t *testing.T) {
	// Override the live connection function with a mock version
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool, pemCerts [][]byte, proxyConfig *dspav1.ProxyConfig, objStoreConnectionTimeout time.Duration) (bool, error) {
		return true, nil
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
			},
		},
	}
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params (unused)
	ctx, _, reconciler := CreateNewTestObjects()

	SecureConnection := false
	params := &DSPAParams{
		ObjectStorageConnection: ObjectStorageConnection{
			Host:            "foo",
			Port:            "1337",
			Secure:          &SecureConnection,
			AccessKeyID:     base64.StdEncoding.EncodeToString([]byte("fooaccesskey")),
			SecretAccessKey: base64.StdEncoding.EncodeToString([]byte("foosecretkey")),
		},
	}

	verified, err := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	assert.True(t, verified, err)
}

func TestIsDatabaseNotAccessibleFalse(t *testing.T) {
	// Override the live connection function with a mock version
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool, pemCerts [][]byte, proxyConfig *dspav1.ProxyConfig, objStoreConnectionTimeout time.Duration) (bool, error) {
		return false, errors.New("Object Store is not Accessible")
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
			},
		},
	}
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params (unused)
	ctx, _, reconciler := CreateNewTestObjects()

	SecureConnection := false
	params := &DSPAParams{
		ObjectStorageConnection: ObjectStorageConnection{
			Host:            "foo",
			Port:            "1337",
			Secure:          &SecureConnection,
			AccessKeyID:     base64.StdEncoding.EncodeToString([]byte("fooaccesskey")),
			SecretAccessKey: base64.StdEncoding.EncodeToString([]byte("foosecretkey")),
		},
	}

	verified, err := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	assert.False(t, verified, err)
}

func TestDisabledHealthCheckReturnsTrue(t *testing.T) {
	// Override the live connection function with a mock version that would always return false if called
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool, pemCerts [][]byte, proxyConfig *dspav1.ProxyConfig, objStoreConnectionTimeout time.Duration) (bool, error) {
		return false, errors.New("Object Store is not Accessible")
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: true,
			},
		},
	}
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params (unused)
	ctx, _, reconciler := CreateNewTestObjects()

	SecureConnection := false
	params := &DSPAParams{
		ObjectStorageConnection: ObjectStorageConnection{
			Host:            "foo",
			Port:            "1337",
			Secure:          &SecureConnection,
			AccessKeyID:     base64.StdEncoding.EncodeToString([]byte("fooaccesskey")),
			SecretAccessKey: base64.StdEncoding.EncodeToString([]byte("foosecretkey")),
		},
	}

	verified, err := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	// if health check is disabled this should always return True
	// even thought the mock connection function would return false if called
	assert.True(t, verified, err)
}

func TestIsDatabaseAccessibleBadAccessKey(t *testing.T) {
	// Override the live connection function with a mock version
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool, pemCerts [][]byte, proxyConfig *dspav1.ProxyConfig, objStoreConnectionTimeout time.Duration) (bool, error) {
		return true, nil
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
			},
		},
	}
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params (unused)
	ctx, _, reconciler := CreateNewTestObjects()

	SecureConnection := false
	params := &DSPAParams{
		ObjectStorageConnection: ObjectStorageConnection{
			Host:            "foo",
			Port:            "1337",
			Secure:          &SecureConnection,
			AccessKeyID:     "this-is-not-a-base64-encoded-string",
			SecretAccessKey: base64.StdEncoding.EncodeToString([]byte("foosecretkey")),
		},
	}

	verified, err := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	assert.False(t, verified, err)
}

func TestIsDatabaseAccessibleBadSecretKey(t *testing.T) {
	// Override the live connection function with a mock version
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool, pemCerts [][]byte, proxyConfig *dspav1.ProxyConfig, objStoreConnectionTimeout time.Duration) (bool, error) {
		return true, nil
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
			},
		},
	}
	dspa.Name = testDSPAName
	dspa.Namespace = testNamespace

	// Create Context, Fake Controller and Params (unused)
	ctx, _, reconciler := CreateNewTestObjects()

	SecureConnection := false
	params := &DSPAParams{
		ObjectStorageConnection: ObjectStorageConnection{
			Host:            "foo",
			Port:            "1337",
			Secure:          &SecureConnection,
			AccessKeyID:     base64.StdEncoding.EncodeToString([]byte("fooaccesskey")),
			SecretAccessKey: "this-is-not-a-base64-encoded-string",
		},
	}

	verified, err := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	assert.False(t, verified, err)
}

func TestJoinHostPort(t *testing.T) {
	tests := map[string]struct {
		host           string
		port           string
		expectedResult string
		expectedError  bool
	}{
		"host and port defined": {host: "somehost", port: "1234", expectedResult: "somehost:1234", expectedError: false},
		"empty port":            {host: "somehost", port: "", expectedResult: "somehost", expectedError: false},
		"empty host":            {host: "", port: "1234", expectedResult: "", expectedError: true},
		"both empty":            {host: "", port: "", expectedResult: "", expectedError: true},
	}

	for _, test := range tests {
		actualResult, actualError := joinHostPort(test.host, test.port)
		if test.expectedError {
			assert.NotNil(t, actualError)
		} else {
			assert.Equal(t, test.expectedResult, actualResult)
			assert.Nil(t, actualError)
		}
	}
}

func TestCreateCredentialProvidersChain(t *testing.T) {
	tests := map[string]struct {
		accesskey       string
		secretkey       string
		expectedSigType credentials.SignatureType
	}{
		"both keys defined": {
			accesskey:       "fakeaccesskey",
			secretkey:       "fakesecretkey",
			expectedSigType: credentials.SignatureV4,
		},
		"no access key": {
			accesskey:       "",
			secretkey:       "fakesecretkey",
			expectedSigType: credentials.SignatureAnonymous,
		},
		"no secret key": {
			accesskey:       "fakeaccesskey",
			secretkey:       "",
			expectedSigType: credentials.SignatureAnonymous,
		},
		"both keys empty": {
			accesskey:       "",
			secretkey:       "",
			expectedSigType: credentials.SignatureAnonymous,
		},
	}

	// Run Tests
	for _, test := range tests {
		actual := createCredentialProvidersChain(test.accesskey, test.secretkey)
		actualCreds, err := actual.Get()
		assert.Nil(t, err)

		actualSigType := actualCreds.SignerType
		assert.Equal(t, test.expectedSigType, actualSigType)
	}
}

func TestGetTransportWithCACert(t *testing.T) {
	validCert := `
-----BEGIN CERTIFICATE-----
MIIDUTCCAjmgAwIBAgIINk8kYK1jtAYwDQYJKoZIhvcNAQELBQAwNjE0MDIGA1UE
Awwrb3BlbnNoaWZ0LXNlcnZpY2Utc2VydmluZy1zaWduZXJAMTY5NzQ4MDY4NjAe
Fw0yMzEwMTYxODI0NDVaFw0yNTEyMTQxODI0NDZaMDYxNDAyBgNVBAMMK29wZW5z
aGlmdC1zZXJ2aWNlLXNlcnZpbmctc2lnbmVyQDE2OTc0ODA2ODYwggEiMA0GCSqG
SIb3DQEBAQUAA4IBDwAwggEKAoIBAQDzSg9LmRucYyv9OUbMjbTGlvLFXl9+vsKd
rdZEq+jR5jr+lhxvU06rezHcTn7hXmm9g66YQhjfJ239VSh/YkQFqlaGY89lEtfr
fJzAkxpX0xmPhjAQ4fpsBs6LfkgC2v846oR2+gsI5hh5VuWNRS6BJlgRIQYUHBqM
p/d8QghkST1mheZKQZh4V9L1aB4Hgo4SCPNVGa/t0Q5sBZmlvC+6JqxsZW8miF/v
rs0oqm9dwhyAsTuLdDAD4bnLPXBQD7z+aq87uBNWcOrl0p/TdJy85lhE0dmbVKS6
c21lQ4Va5JNje25fJmtEviFDAVXc/akMWSHf94ZfbWN8eah29oHNAgMBAAGjYzBh
MA4GA1UdDwEB/wQEAwICpDAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRxBglm
SbrHzzijOCr6EQ2LOTZi5jAfBgNVHSMEGDAWgBRxBglmSbrHzzijOCr6EQ2LOTZi
5jANBgkqhkiG9w0BAQsFAAOCAQEAeENDkKOUebgjb5Jg3d0WLHjqF+xMofXo1Gvg
wkfrZ35hQTMOiFUAffyPRoMfZOJ5x4zUVPkXN1qjVe/oIc19EFgb7ppDXUTJDndu
4RfZCF/yim5C6vUFmPPHjbFxnJIo85pKWGLwGg79iTnExDYMUUg5pRfK1uNgfro9
jEtEoP3F3YVZ8g75TF70Ad9AHPWD2c1D8xOI4XwFvyi5BJJ+jsChl1e3v8D07ohj
Em/2fyF49JL+vAPFMWRFpaExUr3gMbELo4YABQGg024d623LK0ienEF0p4jMVNbP
S9IA40yOaVHMI51Fr1i1EIWvP8oJY8rAPWq45JnfFen3tOqKfw==
-----END CERTIFICATE-----
`
	_, _, reconciler := CreateNewTestObjects()

	validCerts := [][]byte{[]byte(validCert)}
	transport, err := getTransportWithProxyAndCACert(reconciler.Log, validCerts, nil, true)
	assert.Nil(t, err)
	assert.NotNil(t, transport)

	invalidCert := "invalidCert"
	invalidCerts := [][]byte{[]byte(invalidCert)}
	transport, err = getTransportWithProxyAndCACert(reconciler.Log, invalidCerts, nil, true)
	assert.NotNil(t, err)
	assert.Nil(t, transport)
}
