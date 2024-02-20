/*
Copyright 2023.

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
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/util"
	"net/http"
)

const storageSecret = "minio/secret.yaml.tmpl"

var storageTemplates = []string{
	"minio/deployment.yaml.tmpl",
	"minio/pvc.yaml.tmpl",
	"minio/service.yaml.tmpl",
	"minio/minio-sa.yaml.tmpl",
	storageSecret,
}

func joinHostPort(host, port string) (string, error) {
	if host == "" {
		return "", errors.New("Object Storage Connection missing host")
	}
	if port == "" {
		return host, nil
	}
	return fmt.Sprintf("%s:%s", host, port), nil
}

func createCredentialProvidersChain(accessKey, secretKey string) *credentials.Credentials {
	// first try with static api key
	if accessKey != "" && secretKey != "" {
		return credentials.NewStaticV4(accessKey, secretKey, "")
	}
	// otherwise use a chained provider: minioEnv -> awsEnv -> IAM
	providers := []credentials.Provider{
		&credentials.EnvMinio{},
		&credentials.EnvAWS{},
		&credentials.IAM{
			Client: &http.Client{
				Transport: http.DefaultTransport,
			},
		},
	}
	return credentials.New(&credentials.Chain{Providers: providers})
}

func getHttpsTransportWithCACert(log logr.Logger, pemCerts []byte) (*http.Transport, error) {
	transport, err := minio.DefaultTransport(true)
	if err != nil {
		return nil, fmt.Errorf("Error creating default transport : %s", err)
	}

	if transport.TLSClientConfig.RootCAs == nil {
		pool, err := x509.SystemCertPool()
		if err != nil {
			log.Error(err, "error initializing TLS Pool: %s")
			transport.TLSClientConfig.RootCAs = x509.NewCertPool()
		} else {
			transport.TLSClientConfig.RootCAs = pool
		}
	}

	if ok := transport.TLSClientConfig.RootCAs.AppendCertsFromPEM(pemCerts); !ok {
		return nil, fmt.Errorf("error parsing CA Certificate, ensure provided certs are in valid PEM format")
	}
	return transport, nil
}

var ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint, bucket string, accesskey, secretkey []byte, secure bool, pemCerts []byte) bool {
	cred := createCredentialProvidersChain(string(accesskey), string(secretkey))

	opts := &minio.Options{
		Creds:  cred,
		Secure: secure,
	}

	if len(pemCerts) != 0 {
		tr, err := getHttpsTransportWithCACert(log, pemCerts)
		if err != nil {
			log.Error(err, "Encountered error when processing custom ca bundle.")
			return false
		}
		opts.Transport = tr
	}

	minioClient, err := minio.New(endpoint, opts)
	if err != nil {
		log.Info(fmt.Sprintf("Could not connect to object storage endpoint: %s", endpoint))
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, config.DefaultObjStoreConnectionTimeout)
	defer cancel()

	// Attempt to run Stat on the Object.  It doesn't necessarily have to exist, we just want to verify we can successfully run an authenticated s3 command
	_, err = minioClient.StatObject(ctx, bucket, "some-random-object", minio.GetObjectOptions{})
	if err != nil {
		switch err := err.(type) {

		// In the case that the Error is NoSuchKey (or NoSuchBucket), we can verify that the endpoint worked and the object just doesn't exist
		case minio.ErrorResponse:
			if err.Code == "NoSuchKey" || err.Code == "NoSuchBucket" {
				return true
			}
		}

		if util.IsX509UnknownAuthorityError(err) {
			log.Info(fmt.Sprintf("Encountered x509 UnknownAuthorityError when connecting to ObjectStore. "+
				"If using an tls S3 connection with  self-signed certs, you may specify a custom CABundle "+
				"to mount on the DSP API Server via the DSPA cr under the spec.cABundle field. If you have already "+
				"provided a CABundle, verify the validity of the provided CABundle. Error: %v", err))
			return false
		}

		// Every other error means the endpoint in inaccessible, or the credentials provided do not have, at a minimum GetObject, permissions
		log.Info(fmt.Sprintf("Could not connect to (%s), Error: %s", endpoint, err.Error()))
		return false
	}

	// Getting here means the health check passed
	return true
}

func (r *DSPAReconciler) isObjectStorageAccessible(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) bool {
	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)
	if params.ObjectStorageHealthCheckDisabled(dsp) {
		log.V(1).Info("Object Storage health check disabled, assuming object store is available and ready.")
		return true
	}

	log.Info("Performing Object Storage Health Check")

	endpoint, err := joinHostPort(params.ObjectStorageConnection.Host, params.ObjectStorageConnection.Port)
	if err != nil {
		log.Error(err, "Could not determine Object Storage Endpoint")
		return false
	}

	accesskey, err := base64.StdEncoding.DecodeString(params.ObjectStorageConnection.AccessKeyID)
	if err != nil {
		log.Error(err, "Could not decode Object Storage Access Key ID")
		return false
	}

	secretkey, err := base64.StdEncoding.DecodeString(params.ObjectStorageConnection.SecretAccessKey)
	if err != nil {
		log.Error(err, "Could not decode Object Storage Secret Access Key")
		return false
	}

	verified := ConnectAndQueryObjStore(ctx, log, endpoint, params.ObjectStorageConnection.Bucket, accesskey, secretkey, *params.ObjectStorageConnection.Secure, params.APICustomPemCerts)
	if verified {
		log.Info("Object Storage Health Check Successful")
	} else {
		log.Info("Object Storage Health Check Failed")
	}
	return verified
}

// ReconcileStorage will set up Storage Connection.
func (r *DSPAReconciler) ReconcileStorage(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	storageSpecified := dsp.Spec.ObjectStorage != nil
	// Storage field can be specified as an empty obj, confirm that subfields are also specified
	externalStorageSpecified := params.UsingExternalStorage(dsp)
	minioSpecified := !storageSpecified || dsp.Spec.ObjectStorage.Minio != nil
	deployMinio := !storageSpecified || (minioSpecified && dsp.Spec.ObjectStorage.Minio.Deploy)

	// If external storage is specified, it takes precedence
	if externalStorageSpecified {
		log.Info("Deploying external storage secret.")
		// If using external storage, we just need to create the secret
		// for apiserver
		err := r.Apply(dsp, params, storageSecret)
		if err != nil {
			return err
		}
	} else if deployMinio {
		log.Info("Applying object storage resources.")
		for _, template := range storageTemplates {
			err := r.Apply(dsp, params, template)
			if err != nil {
				return err
			}
		}
		// If no storage was not specified, deploy minio by default.
		// Update the CR with the state of minio to accurately portray
		// desired state.
		if !storageSpecified {
			dsp.Spec.ObjectStorage = &dspav1alpha1.ObjectStorage{}
			dsp.Spec.ObjectStorage.Minio = params.Minio.DeepCopy()
			dsp.Spec.ObjectStorage.Minio.Deploy = true
			if err := r.Update(ctx, dsp); err != nil {
				return err
			}
		}
	} else {
		log.Info("No externalstorage detected, and minio disabled. " +
			"skipping application of storage Resources")
		return nil
	}
	log.Info("Finished applying storage Resources")

	return nil
}
