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
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
)

const storageSecret = "minio/secret.yaml.tmpl"

var storageTemplates = []string{
	"minio/deployment.yaml.tmpl",
	"minio/pvc.yaml.tmpl",
	"minio/service.yaml.tmpl",
	"minio/minio-sa.yaml.tmpl",
	storageSecret,
}

func joinHostPort(host, port string) string {
	if port == "" {
		return host
	}
	return fmt.Sprintf("%s:%s", host, port)
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

var ConnectAndQueryObjStore = func(ctx context.Context, log logr.Logger, endpoint string, accesskey, secretkey []byte, secure bool) bool {
	cred := createCredentialProvidersChain(string(accesskey), string(secretkey))
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  cred,
		Secure: secure,
	})
	if err != nil {
		log.Info(fmt.Sprintf("Could not connect to object storage endpoint: %s", endpoint))
		return false
	}

	_, err = minioClient.ListBuckets(ctx)
	if err != nil {
		log.Info(fmt.Sprintf("Could not perform ListBuckets health check on object storage endpoint: %s", endpoint))
		return false
	}

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

	endpoint := joinHostPort(params.ObjectStorageConnection.Host, params.ObjectStorageConnection.Port)
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

	verified := ConnectAndQueryObjStore(ctx, log, endpoint, accesskey, secretkey, *params.ObjectStorageConnection.Secure)
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
