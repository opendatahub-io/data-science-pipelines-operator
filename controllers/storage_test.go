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
	"testing"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7/pkg/credentials"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDeployStorage(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with deployed Minio Object Storage
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			Database: &dspav1alpha1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1alpha1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1alpha1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1alpha1.Minio{
					Deploy: true,
					Image:  "someimage",
					Resources: &dspav1alpha1.ResourceRequirements{ //TODO: fails without this block.  Why?
						Requests: &dspav1alpha1.Resources{
							CPU:    resource.MustParse("250m"),
							Memory: resource.MustParse("500Mi"),
						},
						Limits: &dspav1alpha1.Resources{
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
}
func TestDontDeployStorage(t *testing.T) {
	testNamespace := "testnamespace"
	testDSPAName := "testdspa"
	expectedStorageName := "minio-testdspa"

	// Construct DSPA Spec with non-deployed Minio Object Storage
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			Database: &dspav1alpha1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1alpha1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1alpha1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1alpha1.Minio{
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
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			Database: &dspav1alpha1.Database{
				DisableHealthCheck: false,
				MariaDB: &dspav1alpha1.MariaDB{
					Deploy: true,
				},
			},
			ObjectStorage: &dspav1alpha1.ObjectStorage{
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
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool) bool {
		return true
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			ObjectStorage: &dspav1alpha1.ObjectStorage{
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

	verified := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	assert.True(t, verified)
}

func TestIsDatabaseNotAccessibleFalse(t *testing.T) {
	// Override the live connection function with a mock version
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool) bool {
		return false
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			ObjectStorage: &dspav1alpha1.ObjectStorage{
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

	verified := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	assert.False(t, verified)
}

func TestDisabledHealthCheckReturnsTrue(t *testing.T) {
	// Override the live connection function with a mock version that would always return false if called
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool) bool {
		return false
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			ObjectStorage: &dspav1alpha1.ObjectStorage{
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

	verified := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	// if health check is disabled this should always return True
	// even thought the mock connection function would return false if called
	assert.True(t, verified)
}

func TestIsDatabaseAccessibleBadAccessKey(t *testing.T) {
	// Override the live connection function with a mock version
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool) bool {
		return true
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			ObjectStorage: &dspav1alpha1.ObjectStorage{
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

	verified := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	assert.False(t, verified)
}

func TestIsDatabaseAccessibleBadSecretKey(t *testing.T) {
	// Override the live connection function with a mock version
	ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool) bool {
		return true
	}

	testNamespace := "testnamespace"
	testDSPAName := "testdspa"

	// Minimal Inputs
	dspa := &dspav1alpha1.DataSciencePipelinesApplication{
		Spec: dspav1alpha1.DSPASpec{
			ObjectStorage: &dspav1alpha1.ObjectStorage{
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

	verified := reconciler.isObjectStorageAccessible(ctx, dspa, params)
	assert.False(t, verified)
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
