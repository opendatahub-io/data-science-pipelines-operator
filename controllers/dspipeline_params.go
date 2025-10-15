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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	mf "github.com/manifestival/manifestival"
	dspa "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/util"
	routev1 "github.com/openshift/api/route/v1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const MlmdIsRequired = "MLMD explicitly disabled in DSPA, but is a required component for DSP"

type DSPAParams struct {
	IncludeOwnerReference                 bool
	UID                                   types.UID
	Name                                  string
	APIVersion                            string
	Kind                                  string
	Namespace                             string
	Owner                                 mf.Owner
	DSPVersion                            string
	APIServer                             *dspa.APIServer
	APIServerDefaultResourceName          string
	APIServerServiceName                  string
	APIServerConfigHash                   string
	KubeRBACProxy                         string
	SampleConfigJSON                      string
	ScheduledWorkflow                     *dspa.ScheduledWorkflow
	ScheduledWorkflowDefaultResourceName  string
	PersistenceAgent                      *dspa.PersistenceAgent
	PersistentAgentDefaultResourceName    string
	WorkflowControllerDefaultResourceName string
	MariaDB                               *dspa.MariaDB
	Minio                                 *dspa.Minio
	MLMD                                  *dspa.MLMD
	MlmdProxyDefaultResourceName          string
	MlmdGrpcCertificateContents           string
	MlmdGrpcPrivateKeyContents            string
	WebhookName                           string
	WorkflowController                    *dspa.WorkflowController
	CustomKfpLauncherConfigMapData        string
	APIServerWorkspaceJSON                string
	WebhookAnnotations                    map[string]string
	DBConnection
	ObjectStorageConnection

	// TLS
	// The CA bundle path used by API server
	CustomCABundleRootMountPath string
	// This path is used by API server to also look
	// for CustomCABundleRootMountPath when
	// verifying certs
	CustomSSLCertDir *string
	// The CA bundle path found in the pipeline pods
	PiplinesCABundleMountPath string
	// Collects all certs from user & global certs
	APICustomPemCerts [][]byte
	// Source of truth for the DSP cert configmap details
	// If this is defined, then we assume we have additional certs
	// we need to leverage for tls connections within dsp apiserver
	// pipeline pods
	CustomCABundle *dspa.CABundle
	DSPONamespace  string
	// Use to enable tls communication between component pods.
	PodToPodTLS bool

	APIServerServiceDNSName string
	FIPSEnabled             bool
}

type DBConnection struct {
	Host              string
	Port              string
	Username          string
	DBName            string
	CredentialsSecret *dspa.SecretKeyValue
	Password          string
	DecodedPassword   string
	ExtraParams       string
}
type ObjectStorageConnection struct {
	Bucket            string
	CredentialsSecret *dspa.S3CredentialSecret
	Host              string
	Port              string
	Scheme            string
	Region            string
	BasePath          string
	Secure            *bool
	Endpoint          string // scheme://host:port
	AccessKeyID       string
	SecretAccessKey   string
	ExternalRouteURL  string
}

// UsingExternalDB will return true if an external Database is specified in the CR, otherwise false.
func (p *DSPAParams) UsingExternalDB(dsp *dspa.DataSciencePipelinesApplication) bool {
	if dsp.Spec.Database != nil && dsp.Spec.Database.ExternalDB != nil {
		return true
	}
	return false
}

// DatabaseHealthCheckDisabled will return the value if the Database has disableHealthCheck specified in the CR, otherwise false.
func (p *DSPAParams) DatabaseHealthCheckDisabled(dsp *dspa.DataSciencePipelinesApplication) bool {
	if dsp.Spec.Database != nil {
		return dsp.Spec.Database.DisableHealthCheck
	}
	return false
}

// UsingExternalStorage will return true if an external Object Storage is specified in the CR, otherwise false.
func (p *DSPAParams) UsingExternalStorage(dsp *dspa.DataSciencePipelinesApplication) bool {
	if dsp.Spec.ObjectStorage != nil && dsp.Spec.ObjectStorage.ExternalStorage != nil {
		return true
	}
	return false
}

// ObjectStorageHealthCheckDisabled will return the value if the Object Storage has disableHealthCheck specified in the CR, otherwise false.
func (p *DSPAParams) ObjectStorageHealthCheckDisabled(dsp *dspa.DataSciencePipelinesApplication) bool {
	if dsp.Spec.ObjectStorage != nil {
		return dsp.Spec.ObjectStorage.DisableHealthCheck
	}
	return false
}

// ExternalRouteEnabled will return true if an external route is enabled in the CR, otherwise false.
func (p *DSPAParams) ExternalRouteEnabled(dsp *dspa.DataSciencePipelinesApplication) bool {
	if dsp.Spec.ObjectStorage != nil {
		return dsp.Spec.ObjectStorage.EnableExternalRoute
	}
	return false
}

func (p *DSPAParams) RetrieveAndSetExternalRoute(ctx context.Context, client client.Client, log logr.Logger) (*routev1.Route, error) {
	// Retrieve the external route
	route := &routev1.Route{}
	namespacedName := types.NamespacedName{
		Name:      "minio-" + p.Name,
		Namespace: p.Namespace,
	}
	err := client.Get(ctx, namespacedName, route)

	return route, err
}

func passwordGen(n int) string {
	rand.Seed(time.Now().UnixNano())
	var chars = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func (p *DSPAParams) RetrieveSecret(ctx context.Context, client client.Client, secretName, secretKey string, log logr.Logger) (string, error) {
	secret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: p.Namespace,
	}
	err := client.Get(ctx, namespacedName, secret)
	if err != nil {
		log.V(1).Info(fmt.Sprintf("Unable to retrieve secret [%s].", secretName))
		return "", err
	}
	return base64.StdEncoding.EncodeToString(secret.Data[secretKey]), nil
}

func (p *DSPAParams) RetrieveOrCreateSecret(ctx context.Context, client client.Client, secretName, secretKey string, generatedPasswordLength int, log logr.Logger) (string, error) {
	val, err := p.RetrieveSecret(ctx, client, secretName, secretKey, log)
	if err != nil && apierrs.IsNotFound(err) {
		generatedPass := passwordGen(generatedPasswordLength)
		return base64.StdEncoding.EncodeToString([]byte(generatedPass)), nil
	} else if err != nil {
		log.Error(err, "Unable to create DB secret...")
		return "", err
	}
	log.Info(fmt.Sprintf("Secret [%s] already exists, using stored value.", secretName))
	return val, nil
}

func (p *DSPAParams) RetrieveOrCreateDBSecret(ctx context.Context, client client.Client, secret *dspa.SecretKeyValue, log logr.Logger) (string, error) {
	dbPassword, err := p.RetrieveOrCreateSecret(ctx, client, secret.Name, secret.Key, config.GeneratedDBPasswordLength, log)
	if err != nil {
		return "", err
	}
	return dbPassword, nil

}

func (p *DSPAParams) RetrieveOrCreateObjectStoreSecret(ctx context.Context, client client.Client, secret *dspa.S3CredentialSecret, log logr.Logger) (string, string, error) {
	accessKey, err := p.RetrieveOrCreateSecret(ctx, client, secret.SecretName, secret.AccessKey, config.GeneratedObjectStorageAccessKeyLength, log)
	if err != nil {
		return "", "", err
	}
	secretKey, err := p.RetrieveOrCreateSecret(ctx, client, secret.SecretName, secret.SecretKey, config.GeneratedObjectStorageSecretKeyLength, log)
	if err != nil {
		return "", "", err
	}
	return accessKey, secretKey, nil
}

// SetupDBParams Populates the DB connection Parameters.
// If an external secret is specified, SetupDBParams will retrieve DB credentials from it.
// If DSPO is managing a dynamically created secret, then SetupDBParams generates the creds.
func (p *DSPAParams) SetupDBParams(ctx context.Context, dsp *dspa.DataSciencePipelinesApplication, client client.Client, log logr.Logger) error {

	usingExternalDB := p.UsingExternalDB(dsp)
	if usingExternalDB {
		// Assume validation for CR ensures these values exist
		p.DBConnection.Host = dsp.Spec.Database.ExternalDB.Host
		p.DBConnection.Port = dsp.Spec.Database.ExternalDB.Port
		p.DBConnection.Username = dsp.Spec.Database.ExternalDB.Username
		p.DBConnection.DBName = dsp.Spec.Database.ExternalDB.DBName
		p.DBConnection.CredentialsSecret = dsp.Spec.Database.ExternalDB.PasswordSecret

		// Assume default external connection is tls enabled
		// user can override this via CustomExtraParams field
		tlsParams := config.DBExtraParams{
			"tls": "true",
		}
		dbExtraParams, err := config.GetDefaultDBExtraParams(tlsParams, log)
		if err != nil {
			log.Error(err, "Unexpected error encountered while retrieving DBExtraparams")
			return err
		}
		p.DBConnection.ExtraParams = dbExtraParams

		// Retreive DB Password from specified secret.  Ignore error if the secret simply doesn't exist (will be created later)
		password, err := p.RetrieveSecret(ctx, client, p.DBConnection.CredentialsSecret.Name, p.DBConnection.CredentialsSecret.Key, log)
		if err != nil && !apierrs.IsNotFound(err) {
			log.Error(err, "Unexpected error encountered while fetching Database Secret")
			return err
		}
		p.DBConnection.Password = password
		decodedPasswordBytes, _ := base64.StdEncoding.DecodeString(password)
		p.DBConnection.DecodedPassword = string(decodedPasswordBytes)
	} else {
		// If no externalDB or mariaDB is specified, DSPO assumes
		// MariaDB deployment with defaults.
		if p.MariaDB == nil {
			p.MariaDB = &dspa.MariaDB{
				Deploy:    true,
				Image:     config.GetStringConfigWithDefault(config.MariaDBImagePath, config.DefaultImageValue),
				Resources: config.MariaDBResourceRequirements.DeepCopy(),
				Username:  config.MariaDBUser,
				DBName:    config.MariaDBName,
				PVCSize:   resource.MustParse(config.MariaDBNamePVCSize),
			}
		}

		// If MariaDB was specified, ensure missing fields are
		// populated with defaults.
		if p.MariaDB.Image == "" {
			p.MariaDB.Image = config.GetStringConfigWithDefault(config.MariaDBImagePath, config.DefaultImageValue)
		}
		setStringDefault(config.MariaDBUser, &p.MariaDB.Username)
		setStringDefault(config.MariaDBName, &p.MariaDB.DBName)
		setResourcesDefault(config.MariaDBResourceRequirements, &p.MariaDB.Resources)

		p.DBConnection.Host = fmt.Sprintf(
			"%s.%s.svc.cluster.local",
			config.MariaDBHostPrefix+"-"+p.Name,
			p.Namespace,
		)
		p.DBConnection.Port = config.MariaDBHostPort
		p.DBConnection.Username = p.MariaDB.Username
		p.DBConnection.DBName = p.MariaDB.DBName
		// By Default OOB mariadb is not tls enabled
		tlsParams := config.DBExtraParams{
			"tls": "false",
		}
		if p.PodToPodTLS {
			tlsParams["tls"] = "true"
		}
		dbExtraParams, err := config.GetDefaultDBExtraParams(tlsParams, log)
		if err != nil {
			log.Error(err, "Unexpected error encountered while retrieving DBExtraparams")
			return err
		}
		p.DBConnection.ExtraParams = dbExtraParams

		// If custom DB Secret provided, use its values.  Otherwise generate a default
		if p.MariaDB.PasswordSecret != nil {
			p.DBConnection.CredentialsSecret = p.MariaDB.PasswordSecret
		} else {
			p.DBConnection.CredentialsSecret = &dspa.SecretKeyValue{
				Name: config.DefaultDBSecretNamePrefix + p.Name,
				Key:  config.DefaultDBSecretKey,
			}
		}
		dbPassword, err := p.RetrieveOrCreateDBSecret(ctx, client, p.DBConnection.CredentialsSecret, log)
		if err != nil {
			return err
		}
		p.DBConnection.Password = dbPassword
		decodedPasswordBytes, _ := base64.StdEncoding.DecodeString(dbPassword)
		p.DBConnection.DecodedPassword = string(decodedPasswordBytes)
	}

	// User specified custom Extra parameters will always take precedence
	if dsp.Spec.Database.CustomExtraParams != nil {
		// Validate CustomExtraParams is a valid params json
		var validParamsJson map[string]string
		err := json.Unmarshal([]byte(*dsp.Spec.Database.CustomExtraParams), &validParamsJson)
		if err != nil {
			log.Info(fmt.Sprintf("Encountered error when validating CustomExtraParams field in DSPA, please ensure the params are well-formed: Error: %v", err))
			return err
		}
		p.DBConnection.ExtraParams = *dsp.Spec.Database.CustomExtraParams
	}

	if p.DBConnection.Password == "" {
		return fmt.Errorf("db password from secret [%s] for key [%s] was not successfully retrieved, ensure that the secret with this key exist",
			p.DBConnection.CredentialsSecret.Name, p.DBConnection.CredentialsSecret.Key)
	}
	return nil
}

// SetupObjectParams Populates the Object Storage connection Parameters.
// If an external secret is specified, SetupObjectParams will retrieve storage credentials from it.
// If DSPO is managing a dynamically created secret, then SetupObjectParams generates the creds.
func (p *DSPAParams) SetupObjectParams(ctx context.Context, dsp *dspa.DataSciencePipelinesApplication, client client.Client, log logr.Logger) error {

	usingExternalObjectStorage := p.UsingExternalStorage(dsp)
	if usingExternalObjectStorage {
		// Assume validation for CR ensures these values exist
		p.ObjectStorageConnection.Bucket = dsp.Spec.ObjectStorage.ExternalStorage.Bucket
		p.ObjectStorageConnection.Host = dsp.Spec.ObjectStorage.ExternalStorage.Host
		p.ObjectStorageConnection.Scheme = dsp.Spec.ObjectStorage.ExternalStorage.Scheme
		p.ObjectStorageConnection.BasePath = dsp.Spec.ObjectStorage.ExternalStorage.BasePath
		p.ObjectStorageConnection.Region = dsp.Spec.ObjectStorage.ExternalStorage.Region
		if p.ObjectStorageConnection.Region == "" {
			p.ObjectStorageConnection.Region = "auto"
		}

		if dsp.Spec.ObjectStorage.ExternalStorage.Secure == nil {
			if p.ObjectStorageConnection.Scheme == "https" {
				p.ObjectStorageConnection.Secure = util.BoolPointer(true)
			} else {
				p.ObjectStorageConnection.Secure = util.BoolPointer(false)
			}
		} else {
			p.ObjectStorageConnection.Secure = dsp.Spec.ObjectStorage.ExternalStorage.Secure
		}

		// Port can be empty, which is fine.
		p.ObjectStorageConnection.Port = dsp.Spec.ObjectStorage.ExternalStorage.Port
		p.ObjectStorageConnection.CredentialsSecret = dsp.Spec.ObjectStorage.ExternalStorage.S3CredentialSecret

		// Retrieve ObjStore Creds from specified secret.  Ignore error if the secret simply doesn't exist (will be created later)
		accesskey, err := p.RetrieveSecret(ctx, client, p.ObjectStorageConnection.CredentialsSecret.SecretName, p.ObjectStorageConnection.CredentialsSecret.AccessKey, log)
		if err != nil && !apierrs.IsNotFound(err) {
			log.Error(err, "Unexpected error encountered while fetching Object Storage Secret")
			return err
		}
		secretkey, err := p.RetrieveSecret(ctx, client, p.ObjectStorageConnection.CredentialsSecret.SecretName, p.ObjectStorageConnection.CredentialsSecret.SecretKey, log)
		if err != nil && !apierrs.IsNotFound(err) {
			log.Error(err, "Unexpected error encountered while fetching Object Storage Secret")
			return err
		}
		p.ObjectStorageConnection.AccessKeyID = accesskey
		p.ObjectStorageConnection.SecretAccessKey = secretkey
	} else {
		if p.Minio == nil {
			return fmt.Errorf("either [spec.objectStorage.minio] or [spec.objectStorage.externalStorage] " +
				"need to be specified in DSPA spec")
		}

		// If Minio was specified, ensure missing fields are
		// populated with defaults.

		if p.Minio.Image == "" {
			return fmt.Errorf("minio specified, but no image provided in the DSPA CR Spec")
		}

		p.Minio.Image = dsp.Spec.ObjectStorage.Minio.Image

		setStringDefault(config.MinioDefaultBucket, &p.Minio.Bucket)
		setResourcesDefault(config.MinioResourceRequirements, &p.Minio.Resources)

		p.ObjectStorageConnection.Bucket = config.MinioDefaultBucket
		p.ObjectStorageConnection.Host = fmt.Sprintf(
			"%s.%s.svc.cluster.local",
			config.MinioHostPrefix+"-"+p.Name,
			p.Namespace,
		)
		p.ObjectStorageConnection.Port = config.MinioPort
		p.ObjectStorageConnection.Scheme = config.MinioScheme
		p.ObjectStorageConnection.Secure = util.BoolPointer(false)
		p.ObjectStorageConnection.Region = "minio"

		if p.Minio.S3CredentialSecret != nil {
			p.ObjectStorageConnection.CredentialsSecret = p.Minio.S3CredentialSecret
		} else {
			p.ObjectStorageConnection.CredentialsSecret = &dspa.S3CredentialSecret{
				SecretName: config.DefaultObjectStorageSecretNamePrefix + p.Name,
				AccessKey:  config.DefaultObjectStorageAccessKey,
				SecretKey:  config.DefaultObjectStorageSecretKey,
			}
		}

		accessKey, secretKey, err := p.RetrieveOrCreateObjectStoreSecret(ctx, client, p.ObjectStorageConnection.CredentialsSecret, log)
		if err != nil {
			return err
		}
		p.ObjectStorageConnection.AccessKeyID = accessKey
		p.ObjectStorageConnection.SecretAccessKey = secretKey

	}

	if p.ExternalRouteEnabled(dsp) {
		route, err := p.RetrieveAndSetExternalRoute(ctx, client, log)
		if err != nil {
			log.Info("Unable to retrieve route", "error", err)
		}
		p.ObjectStorageConnection.ExternalRouteURL = route.Spec.Host
		p.ObjectStorageConnection.Endpoint = route.Spec.Host
		p.ObjectStorageConnection.Secure = util.BoolPointer(true)
		p.ObjectStorageConnection.Host = route.Spec.Host
		p.ObjectStorageConnection.Scheme = "https"
		//port should be empty when external route is specified
		p.ObjectStorageConnection.Port = ""
	}
	endpoint := fmt.Sprintf(
		"%s://%s",
		p.ObjectStorageConnection.Scheme,
		p.ObjectStorageConnection.Host,
	)

	if p.ObjectStorageConnection.Port != "" {
		endpoint = fmt.Sprintf(
			"%s:%s",
			endpoint,
			p.ObjectStorageConnection.Port,
		)
	}

	p.ObjectStorageConnection.Endpoint = endpoint

	if p.ObjectStorageConnection.AccessKeyID == "" || p.ObjectStorageConnection.SecretAccessKey == "" {
		return fmt.Errorf("object storage password from secret [%s] for keys [%s, %s] was not "+
			"successfully retrieved, ensure that the secret with this key exist",
			p.ObjectStorageConnection.CredentialsSecret.SecretName,
			p.ObjectStorageConnection.CredentialsSecret.AccessKey, p.ObjectStorageConnection.CredentialsSecret.SecretKey)
	}
	return nil

}

func (p *DSPAParams) SetupMLMD(dsp *dspa.DataSciencePipelinesApplication, log logr.Logger) error {
	if p.MLMD == nil {
		log.Info("MLMD not specified, but is a required component for Pipelines. Including MLMD with default specs.")
		p.MLMD = &dspa.MLMD{
			Deploy: true,
			Envoy: &dspa.Envoy{
				DeployRoute: true,
			},
		}
	} else if !p.MLMD.Deploy {
		return errors.New(MlmdIsRequired)
	}

	if p.MLMD != nil {
		if p.MLMD.Envoy == nil {
			p.MLMD.Envoy = &dspa.Envoy{
				Image:       config.GetStringConfigWithDefault(config.MlmdEnvoyImagePath, config.DefaultImageValue),
				DeployRoute: true,
			}
		}
		if p.MLMD.GRPC == nil {
			p.MLMD.GRPC = &dspa.GRPC{
				Image: config.GetStringConfigWithDefault(config.MlmdGRPCImagePath, config.DefaultImageValue),
			}
		}

		mlmdEnvoyImageFromConfig := config.GetStringConfigWithDefault(config.MlmdEnvoyImagePath, config.DefaultImageValue)
		mlmdGRPCImageFromConfig := config.GetStringConfigWithDefault(config.MlmdGRPCImagePath, config.DefaultImageValue)

		setStringDefault(mlmdEnvoyImageFromConfig, &p.MLMD.Envoy.Image)
		setStringDefault(mlmdGRPCImageFromConfig, &p.MLMD.GRPC.Image)

		setResourcesDefault(config.MlmdEnvoyResourceRequirements, &p.MLMD.Envoy.Resources)
		setResourcesDefault(config.MlmdGRPCResourceRequirements, &p.MLMD.GRPC.Resources)

		setStringDefault(config.MlmdGrpcPort, &p.MLMD.GRPC.Port)
	}
	return nil
}

func (p *DSPAParams) SetupOwner(dsp *dspa.DataSciencePipelinesApplication) {
	p.IncludeOwnerReference = config.GetBoolConfigWithDefault(config.ApiServerIncludeOwnerReferenceConfigName, config.DefaultApiServerIncludeOwnerReferenceConfigName)

	if p.IncludeOwnerReference {
		p.UID = dsp.UID
		p.Name = dsp.Name
		p.APIVersion = dsp.APIVersion
		p.Kind = dsp.Kind
	}
}

func setStringDefault(defaultValue string, value *string) {
	if *value == "" {
		*value = defaultValue
	}
}

func setResourcesDefault(defaultValue dspa.ResourceRequirements, value **dspa.ResourceRequirements) {
	if *value == nil {
		*value = defaultValue.DeepCopy()
	}
}

func (p *DSPAParams) LoadMlmdCertificates(ctx context.Context, client client.Client) (bool, error) {
	secret, err := util.GetSecret(ctx, "ds-pipeline-metadata-grpc-tls-certs-"+p.Name, p.Namespace, client)
	if err != nil {
		if apierrs.IsNotFound(err) {
			return false, nil
		} else {
			return false, err
		}
	}
	p.MlmdGrpcCertificateContents = strings.ReplaceAll(string(secret.Data["tls.crt"]), "\n", "\\n")
	p.MlmdGrpcPrivateKeyContents = strings.ReplaceAll(string(secret.Data["tls.key"]), "\n", "\\n")
	return true, nil
}

func (p *DSPAParams) ExtractParams(ctx context.Context, dsp *dspa.DataSciencePipelinesApplication, client client.Client, loggr logr.Logger) error {
	p.Name = dsp.Name
	p.Namespace = dsp.Namespace
	p.DSPONamespace = os.Getenv("DSPO_NAMESPACE")
	p.DSPVersion = dsp.Spec.DSPVersion
	p.Owner = dsp
	p.APIServer = dsp.Spec.APIServer.DeepCopy()
	p.APIServerDefaultResourceName = apiServerDefaultResourceNamePrefix + dsp.Name
	p.APIServerServiceName = fmt.Sprintf("%s-%s", config.DSPServicePrefix, p.Name)
	p.APIServerServiceDNSName = fmt.Sprintf("%s.%s.svc.cluster.local", p.APIServerServiceName, p.Namespace)
	p.ScheduledWorkflow = dsp.Spec.ScheduledWorkflow.DeepCopy()
	p.ScheduledWorkflowDefaultResourceName = scheduledWorkflowDefaultResourceNamePrefix + dsp.Name
	p.WorkflowController = dsp.Spec.WorkflowController.DeepCopy()
	p.WorkflowControllerDefaultResourceName = workflowControllerDefaultResourceNamePrefix + dsp.Name
	p.PersistenceAgent = dsp.Spec.PersistenceAgent.DeepCopy()
	p.PersistentAgentDefaultResourceName = persistenceAgentDefaultResourceNamePrefix + dsp.Name
	p.MariaDB = dsp.Spec.Database.MariaDB.DeepCopy()
	p.Minio = dsp.Spec.ObjectStorage.Minio.DeepCopy()
	p.KubeRBACProxy = config.GetStringConfigWithDefault(config.KubeRBACProxyImagePath, config.DefaultImageValue)
	p.MLMD = dsp.Spec.MLMD.DeepCopy()
	p.MlmdProxyDefaultResourceName = mlmdProxyDefaultResourceNamePrefix + dsp.Name
	p.CustomCABundleRootMountPath = config.CustomCABundleRootMountPath
	p.PiplinesCABundleMountPath = config.GetCABundleFileMountPath()
	p.PodToPodTLS = false
	p.FIPSEnabled = config.GetBoolConfigWithDefault(config.FIPSEnabledConfigName, config.DefaultFIPSEnabled)
	p.WebhookName = "ds-pipelines-webhook"
	dspTrustedCAConfigMapKey := config.CustomDSPTrustedCAConfigMapKey

	// by default it's enabled when omitted
	if dsp.Spec.PodToPodTLS == nil {
		p.PodToPodTLS = true
	} else {
		p.PodToPodTLS = *dsp.Spec.PodToPodTLS
	}

	log := loggr.WithValues("namespace", p.Namespace).WithValues("dspa_name", p.Name)

	if p.APIServer != nil {
		serverImageFromConfig := config.GetStringConfigWithDefault(config.APIServerImagePath, config.DefaultImageValue)
		argoLauncherImageFromConfig := config.GetStringConfigWithDefault(config.LauncherImagePath, config.DefaultImageValue)
		argoDriverImageFromConfig := config.GetStringConfigWithDefault(config.DriverImagePath, config.DefaultImageValue)

		setStringDefault(serverImageFromConfig, &p.APIServer.Image)
		setStringDefault(argoLauncherImageFromConfig, &p.APIServer.ArgoLauncherImage)
		setStringDefault(argoDriverImageFromConfig, &p.APIServer.ArgoDriverImage)

		setResourcesDefault(config.APIServerResourceRequirements, &p.APIServer.Resources)

		if p.APIServer.CustomServerConfig == nil {
			p.APIServer.CustomServerConfig = &dspa.ScriptConfigMap{
				Name: config.CustomServerConfigMapNamePrefix + dsp.Name,
				Key:  config.CustomServerConfigMapNameKey,
			}
		}

		if p.APIServer.CustomKfpLauncherConfigMap != "" {
			cm, err := util.GetConfigMap(ctx, p.APIServer.CustomKfpLauncherConfigMap, p.Namespace, client)
			if err != nil {
				if apierrs.IsNotFound(err) {
					log.Info(fmt.Sprintf("ConfigMap referenced by CustomKfpLauncherConfig not found: [%s], Error: %v", p.APIServer.CustomKfpLauncherConfigMap, err))
					return err
				} else {
					log.Info(fmt.Sprintf("Error fetching ConfigMap referenced by CustomKfpLauncherConfig: [%s], Error: %v", p.APIServer.CustomKfpLauncherConfigMap, err))
					return err
				}

			} else {
				// when setting a map into the `data` field of a ConfigMap, text/template works well with a json object
				jsonData, err := json.Marshal(cm.Data)
				if err != nil {
					log.Info(fmt.Sprintf("Error reading data of ConfigMap referenced by CustomKfpLauncherConfig: [%s], Error: %v", p.APIServer.CustomKfpLauncherConfigMap, err))
					return err
				} else {
					p.CustomKfpLauncherConfigMapData = string(jsonData)
				}
			}
		}

		// Track whether the "ca-bundle.crt" configmap key from odh-trusted-ca bundle
		// was found, this will be used to decide whether we need to account for this
		// ourselves later or not.
		wellKnownCABundleAdded := false

		// Check for cert bundle provided by the platform instead of by the DSPA user
		// If it exists, include this cert for tls verifications
		globalCABundleCFGMapName := config.GlobalODHCaBundleConfigMapName

		odhTrustedCABundleConfigMap, err := util.GetConfigMap(ctx, globalCABundleCFGMapName, p.Namespace, client)
		if err != nil {
			// If the global cert configmap is not available, that is OK
			if !apierrs.IsNotFound(err) {
				log.Info(fmt.Sprintf("Encountered error when attempting to fetch ConfigMap: [%s], Error: %v", globalCABundleCFGMapName, err))
				return err
			}
		} else {
			// Found a cert provided by odh-operator. Consume it.
			globalCerts := util.GetConfigMapValues(odhTrustedCABundleConfigMap)
			log.Info(fmt.Sprintf("Found global CA Bundle %s present in this namespace %s, this bundle will be included in external tls connections.", config.GlobalODHCaBundleConfigMapName, p.Namespace))
			// "odh-trusted-ca-bundle" can have fields: "odh-ca-bundle.crt" and "ca-bundle.crt", we need to utilize both
			for _, val := range globalCerts {
				// If the ca-bundle field is empty, ignore it
				if strings.TrimSpace(val) != "" {
					p.APICustomPemCerts = append(p.APICustomPemCerts, []byte(val))
				}
			}
			// If odh-trusted-ca-bundle is created via network operator then this is always going to be present
			// however if a user creates this, they may accidentally leave this out, so we need to account for this
			_, ok := odhTrustedCABundleConfigMap.Data[config.GlobalODHCaBundleConfigMapSystemBundleKey]
			if ok {
				wellKnownCABundleAdded = true
			}
		}

		// If user provided a CA bundle, include this in tls verification
		if p.APIServer.CABundle != nil {
			dspaCaBundleCfgKey, dspaCaBundleCfgName := p.APIServer.CABundle.ConfigMapKey, p.APIServer.CABundle.ConfigMapName
			dspaCAConfigMap, dspaCACfgErr := util.GetConfigMap(ctx, dspaCaBundleCfgName, p.Namespace, client)
			if dspaCACfgErr != nil && apierrs.IsNotFound(dspaCACfgErr) {
				log.Info(fmt.Sprintf("ConfigMap [%s] was not found in namespace [%s]", dspaCAConfigMap.Name, p.Namespace))
				return dspaCACfgErr
			} else if dspaCACfgErr != nil {
				log.Info(fmt.Sprintf("Encountered error when attempting to fetch ConfigMap: [%s], Error: %v", dspaCaBundleCfgName, dspaCACfgErr))
				return dspaCACfgErr
			}
			dspaProvidedCABundle := util.GetConfigMapValue(dspaCaBundleCfgKey, dspaCAConfigMap)
			// If the ca-bundle field is empty, ignore it
			if strings.TrimSpace(dspaProvidedCABundle) != "" {
				p.APICustomPemCerts = append(p.APICustomPemCerts, []byte(dspaProvidedCABundle))
			}
		}

		// If PodToPodTLS is enabled, we need to include service-ca ca-bundles to recognize the certs
		// that are signed by service-ca. These can be accessed via "openshift-service-ca.crt"
		// configmap.
		if p.PodToPodTLS {
			serviceCA, serviceCACfgErr := util.GetConfigMap(ctx, config.OpenshiftServiceCAConfigMapName, p.Namespace, client)
			if serviceCACfgErr != nil {
				log.Info(fmt.Sprintf("Encountered error when attempting to fetch ConfigMap: [%s]. Error: %v", config.OpenshiftServiceCAConfigMapName, serviceCA))
				return serviceCACfgErr
			}
			serviceCABundle := util.GetConfigMapValue(config.OpenshiftServiceCAConfigMapKey, serviceCA)
			if strings.TrimSpace(serviceCABundle) == "" {
				return fmt.Errorf("expected key %s from configmap %s not found", config.OpenshiftServiceCAConfigMapKey, config.OpenshiftServiceCAConfigMapName)
			}
			p.APICustomPemCerts = append(p.APICustomPemCerts, []byte(serviceCABundle))
		}

		if p.APIServer.CABundleFileMountPath != "" {
			p.CustomCABundleRootMountPath = p.APIServer.CABundleFileMountPath
		}
		if p.APIServer.CABundleFileName != "" {
			dspTrustedCAConfigMapKey = p.APIServer.CABundleFileName
		}
		p.PiplinesCABundleMountPath = fmt.Sprintf("%s/%s", p.CustomCABundleRootMountPath, dspTrustedCAConfigMapKey)

		// There are situations where global & user provided certs, or a provided ca trust configmap(s) have various trust bundles
		// (for example in the case of "odh-trusted-ca-bundle") there is "odh-ca-bundle.crt" and "ca-bundle.crt".
		// We create a separate configmap and concatenate all the certs into a single bundle, because passing a
		// full path into the pipeline doesn't seem to work with aws cli used for artifact passing
		// Ref: https://github.com/aws/aws-cli/issues/3425#issuecomment-402289636

		// If user or global CABundle has been provided
		// 1) create the dsp-trusted-ca configmap
		// 2) populate CustomCABundle SOT var for pipeline pods and artifact script to utilize during templating
		// 3) set ssl_cert_dir for api server
		if len(p.APICustomPemCerts) > 0 {

			// We need to ensure system certs are always part of this new configmap
			// We can either source this from odh-trusted-ca-bundle cfgmap if provided,
			// or fetch one from "config-trusted-cabundle" configmap, which is always present in an ocp ns
			if !wellKnownCABundleAdded {
				certs, sysCertsErr := util.GetSystemCerts()
				if sysCertsErr != nil {
					return sysCertsErr
				}

				if len(bytes.TrimSpace(certs)) != 0 {
					p.APICustomPemCerts = append(p.APICustomPemCerts, certs)
				}
			}

			p.CustomCABundle = &dspa.CABundle{
				ConfigMapKey:  dspTrustedCAConfigMapKey,
				ConfigMapName: fmt.Sprintf("%s-%s", config.CustomDSPTrustedCAConfigMapNamePrefix, p.Name),
			}

			// Combine certs into a single configmap field
			customCABundleCert := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      p.CustomCABundle.ConfigMapName,
					Namespace: p.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         dsp.APIVersion,
							Kind:               dsp.Kind,
							Name:               dsp.Name,
							UID:                dsp.UID,
							Controller:         util.BoolPointer(true),
							BlockOwnerDeletion: util.BoolPointer(true),
						},
					},
				},

				Data: map[string]string{
					p.CustomCABundle.ConfigMapKey: string(bytes.Join(p.APICustomPemCerts, []byte("\n"))),
				},
			}

			err := client.Create(ctx, customCABundleCert)
			if apierrs.IsAlreadyExists(err) {
				err := client.Update(ctx, customCABundleCert)
				if err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			// We need to update the default SSL_CERT_DIR to include
			// dsp custom cert path, used by DSP Api Server
			var certDirectories = []string{
				p.CustomCABundleRootMountPath,
				"/etc/ssl/certs",     // SLES10/SLES11, https://golang.org/issue/12139
				"/etc/pki/tls/certs", // Fedora/RHEL
			}
			// SSL_CERT_DIR accepts a colon separated list of directories
			sslCertDir := strings.Join(certDirectories, ":")
			p.CustomSSLCertDir = &sslCertDir
		}

		if p.APIServer.ArtifactSignedURLExpirySeconds == nil {
			expiry := config.DefaultSignedUrlExpiryTimeSeconds
			p.APIServer.ArtifactSignedURLExpirySeconds = &expiry
		}

		if dsp.Spec.APIServer.CacheEnabled != nil {
			p.APIServer.CacheEnabled = dsp.Spec.APIServer.CacheEnabled
		}

		if p.APIServer.Workspace != nil {
			workspaceConfig := map[string]interface{}{
				"VolumeClaimTemplateSpec": p.APIServer.Workspace.VolumeClaimTemplateSpec,
			}
			workspaceJSON, err := json.Marshal(workspaceConfig)
			if err != nil {
				return fmt.Errorf("unable to marshal workspace configuration: %w", err)
			}
			p.APIServerWorkspaceJSON = string(workspaceJSON)
		}
	}

	if p.PersistenceAgent != nil {
		persistenceAgentImageFromConfig := config.GetStringConfigWithDefault(config.PersistenceAgentImagePath, config.DefaultImageValue)
		setStringDefault(persistenceAgentImageFromConfig, &p.PersistenceAgent.Image)
		setResourcesDefault(config.PersistenceAgentResourceRequirements, &p.PersistenceAgent.Resources)
	}
	if p.ScheduledWorkflow != nil {
		scheduledWorkflowImageFromConfig := config.GetStringConfigWithDefault(config.ScheduledWorkflowImagePath, config.DefaultImageValue)
		setStringDefault(scheduledWorkflowImageFromConfig, &p.ScheduledWorkflow.Image)
		setResourcesDefault(config.ScheduledWorkflowResourceRequirements, &p.ScheduledWorkflow.Resources)
	}
	// If user did not specify WorkflowController
	if dsp.Spec.WorkflowController == nil {
		dsp.Spec.WorkflowController = &dspa.WorkflowController{
			Deploy: true,
		}
	}
	p.WorkflowController = dsp.Spec.WorkflowController.DeepCopy()

	if p.WorkflowController != nil {
		argoWorkflowImageFromConfig := config.GetStringConfigWithDefault(config.ArgoWorkflowControllerImagePath, config.DefaultImageValue)
		argoExecImageFromConfig := config.GetStringConfigWithDefault(config.ArgoExecImagePath, config.DefaultImageValue)
		setStringDefault(argoWorkflowImageFromConfig, &p.WorkflowController.Image)
		setStringDefault(argoExecImageFromConfig, &p.WorkflowController.ArgoExecImage)
		setResourcesDefault(config.WorkflowControllerResourceRequirements, &p.WorkflowController.Resources)
	}

	err := p.SetupMLMD(dsp, log)
	if err != nil {
		return err
	}

	err = p.SetupDBParams(ctx, dsp, client, log)
	if err != nil {
		return err
	}

	err = p.SetupObjectParams(ctx, dsp, client, log)
	if err != nil {
		return err
	}

	p.SetupOwner(dsp)

	return nil
}
