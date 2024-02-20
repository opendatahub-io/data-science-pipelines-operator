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
	"k8s.io/apimachinery/pkg/util/json"
	"math/rand"
	"time"

	"github.com/go-logr/logr"
	mf "github.com/manifestival/manifestival"
	dspa "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/util"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DSPAParams struct {
	Name                                 string
	Namespace                            string
	Owner                                mf.Owner
	APIServer                            *dspa.APIServer
	APIServerPiplinesCABundleMountPath   string
	PiplinesCABundleMountPath            string
	APIServerDefaultResourceName         string
	APIServerServiceName                 string
	APICustomPemCerts                    []byte
	OAuthProxy                           string
	ScheduledWorkflow                    *dspa.ScheduledWorkflow
	ScheduledWorkflowDefaultResourceName string
	PersistenceAgent                     *dspa.PersistenceAgent
	PersistentAgentDefaultResourceName   string
	MlPipelineUI                         *dspa.MlPipelineUI
	MariaDB                              *dspa.MariaDB
	Minio                                *dspa.Minio
	MLMD                                 *dspa.MLMD
	DBConnection
	ObjectStorageConnection
}

type DBConnection struct {
	Host              string
	Port              string
	Username          string
	DBName            string
	CredentialsSecret *dspa.SecretKeyValue
	Password          string
	ExtraParams       string
}

type ObjectStorageConnection struct {
	Bucket            string
	CredentialsSecret *dspa.S3CredentialSecret
	Host              string
	Port              string
	Scheme            string
	Secure            *bool
	Endpoint          string // scheme://host:port
	AccessKeyID       string
	SecretAccessKey   string
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

func (p *DSPAParams) UsingMLMD(dsp *dspa.DataSciencePipelinesApplication) bool {
	if dsp.Spec.MLMD != nil {
		return dsp.Spec.MLMD.Deploy
	}
	return false
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

// SetupDBParams Populates the DB connection Parameters.
// If an external secret is specified, SetupDBParams will retrieve DB credentials from it.
// If DSPO is managing a dynamically created secret, then SetupDBParams generates the creds.
func (p *DSPAParams) SetupDBParams(ctx context.Context, dsp *dspa.DataSciencePipelinesApplication, client client.Client, log logr.Logger) error {

	usingExternalDB := p.UsingExternalDB(dsp)

	var customCreds *dspa.SecretKeyValue

	// Even if a secret is specified DSPO will deploy its own secret owned by DSPO
	p.DBConnection.CredentialsSecret = &dspa.SecretKeyValue{
		Name: config.DBSecretNamePrefix + p.Name,
		Key:  config.DBSecretKey,
	}

	if usingExternalDB {
		// Assume validation for CR ensures these values exist
		p.DBConnection.Host = dsp.Spec.Database.ExternalDB.Host
		p.DBConnection.Port = dsp.Spec.Database.ExternalDB.Port
		p.DBConnection.Username = dsp.Spec.Database.ExternalDB.Username
		p.DBConnection.DBName = dsp.Spec.Database.ExternalDB.DBName
		// Assume default external connection is tls enabled
		// user can override this via CustomExtraParams field
		p.DBConnection.ExtraParams = fmt.Sprintf(config.DBDefaultExtraParams, true)
		customCreds = dsp.Spec.Database.ExternalDB.PasswordSecret
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
		p.DBConnection.ExtraParams = fmt.Sprintf(config.DBDefaultExtraParams, false)
		if p.MariaDB.PasswordSecret != nil {
			customCreds = p.MariaDB.PasswordSecret
		}
	}

	// User specified custom Extra parameters will always take precedence
	if dsp.Spec.Database.CustomExtraParams != nil {
		// Validate CustomExtraParams is a valid params json
		var validParamsJson map[string]string
		err := json.Unmarshal([]byte(*dsp.Spec.Database.CustomExtraParams), &validParamsJson)
		if err != nil {
			log.Info(fmt.Sprintf("Encountered error when validationg CustomExtraParams field in DSPA, please ensure the params are well-formed: Error: %v", err))
			return err
		}
		p.DBConnection.ExtraParams = *dsp.Spec.Database.CustomExtraParams
	}

	// Secret where DB credentials reside on cluster
	var credsSecretName string
	var credsPasswordKey string

	customCredentialsSpecified := customCreds != nil
	if customCredentialsSpecified {
		credsSecretName = customCreds.Name
		credsPasswordKey = customCreds.Key
	} else {
		credsSecretName = p.DBConnection.CredentialsSecret.Name
		credsPasswordKey = p.DBConnection.CredentialsSecret.Key
	}

	dbSecret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      credsSecretName,
		Namespace: p.Namespace,
	}

	createNewSecret := false

	// Attempt to fetch the specified DB secret
	err := client.Get(ctx, namespacedName, dbSecret)
	if err != nil && apierrs.IsNotFound(err) {
		if !customCredentialsSpecified {
			generatedPass := passwordGen(12)
			p.DBConnection.Password = base64.StdEncoding.EncodeToString([]byte(generatedPass))
			createNewSecret = true
		} else {
			log.Error(err, fmt.Sprintf("DB secret [%s] was specified in CR but does not exist.",
				credsSecretName))
			return err
		}
	} else if err != nil {
		log.Error(err, "Unable to fetch DB secret...")
		return err
	}

	// Password was dynamically generated, no need to retrieve it from fetched secret
	if createNewSecret {
		return nil
	}

	p.DBConnection.Password = base64.StdEncoding.EncodeToString(dbSecret.Data[credsPasswordKey])

	if p.DBConnection.Password == "" {
		return fmt.Errorf(fmt.Sprintf("DB Password from secret [%s] for key [%s] was not successfully retrieved, "+
			"ensure that the secret with this key exist.", credsSecretName, credsPasswordKey))
	}
	return nil
}

// SetupObjectParams Populates the Object Storage connection Parameters.
// If an external secret is specified, SetupObjectParams will retrieve storage credentials from it.
// If DSPO is managing a dynamically created secret, then SetupObjectParams generates the creds.
func (p *DSPAParams) SetupObjectParams(ctx context.Context, dsp *dspa.DataSciencePipelinesApplication, client client.Client, log logr.Logger) error {

	usingExternalObjectStorage := p.UsingExternalStorage(dsp)

	var customCreds *dspa.S3CredentialSecret

	// Even if a secret is specified DSPO will deploy its own secret owned by DSPO
	p.ObjectStorageConnection.CredentialsSecret = &dspa.S3CredentialSecret{
		SecretName: config.ObjectStorageSecretName,
		AccessKey:  config.ObjectStorageAccessKey,
		SecretKey:  config.ObjectStorageSecretKey,
	}

	if usingExternalObjectStorage {
		// Assume validation for CR ensures these values exist
		p.ObjectStorageConnection.Bucket = dsp.Spec.ObjectStorage.ExternalStorage.Bucket
		p.ObjectStorageConnection.Host = dsp.Spec.ObjectStorage.ExternalStorage.Host
		p.ObjectStorageConnection.Scheme = dsp.Spec.ObjectStorage.ExternalStorage.Scheme

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
		customCreds = dsp.Spec.ObjectStorage.ExternalStorage.S3CredentialSecret
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

		if p.Minio.S3CredentialSecret != nil {
			customCreds = p.Minio.S3CredentialSecret
		}
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

	// Secret where credentials reside on cluster
	var credsSecretName string
	var credsAccessKey string
	var credsSecretKey string

	customCredentialsSpecified := customCreds != nil
	if customCredentialsSpecified {
		credsSecretName = customCreds.SecretName
		credsAccessKey = customCreds.AccessKey
		credsSecretKey = customCreds.SecretKey
	} else {
		credsSecretName = p.ObjectStorageConnection.CredentialsSecret.SecretName
		credsAccessKey = p.ObjectStorageConnection.CredentialsSecret.AccessKey
		credsSecretKey = p.ObjectStorageConnection.CredentialsSecret.SecretKey
	}

	storageSecret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      credsSecretName,
		Namespace: p.Namespace,
	}

	createNewSecret := false

	// Attempt to fetch the specified storage secret
	err := client.Get(ctx, namespacedName, storageSecret)
	if err != nil && apierrs.IsNotFound(err) {
		if !customCredentialsSpecified {
			generatedPass := passwordGen(16)
			p.ObjectStorageConnection.AccessKeyID = base64.StdEncoding.EncodeToString([]byte(generatedPass))
			generatedPass = passwordGen(24)
			p.ObjectStorageConnection.SecretAccessKey = base64.StdEncoding.EncodeToString([]byte(generatedPass))
			createNewSecret = true
		} else {
			log.Error(err, fmt.Sprintf("Storage secret [%s] was specified in CR but does not exist.",
				credsSecretName))
			return err
		}
	} else if err != nil {
		log.Error(err, "Unable to fetch Storage secret...")
		return err
	}

	// Password was dynamically generated, no need to retrieve it from fetched secret
	if createNewSecret {
		return nil
	}

	p.ObjectStorageConnection.AccessKeyID = base64.StdEncoding.EncodeToString(storageSecret.Data[credsAccessKey])
	p.ObjectStorageConnection.SecretAccessKey = base64.StdEncoding.EncodeToString(storageSecret.Data[credsSecretKey])

	if p.ObjectStorageConnection.AccessKeyID == "" || p.ObjectStorageConnection.SecretAccessKey == "" {
		return fmt.Errorf(fmt.Sprintf("Object Storage Password from secret [%s] for keys [%s, %s] was not "+
			"successfully retrieved, ensure that the secret with this key exist.", credsSecretName, credsAccessKey, credsSecretKey))
	}

	return nil

}

func (p *DSPAParams) SetupMLMD(ctx context.Context, dsp *dspa.DataSciencePipelinesApplication, client client.Client, log logr.Logger) error {
	if p.MLMD != nil {
		if p.MLMD.Envoy == nil {
			p.MLMD.Envoy = &dspa.Envoy{
				Image: config.GetStringConfigWithDefault(config.MlmdEnvoyImagePath, config.DefaultImageValue),
			}
		}
		if p.MLMD.GRPC == nil {
			p.MLMD.GRPC = &dspa.GRPC{
				Image: config.GetStringConfigWithDefault(config.MlmdGRPCImagePath, config.DefaultImageValue),
			}
		}
		if p.MLMD.Writer == nil {
			p.MLMD.Writer = &dspa.Writer{
				Image: config.GetStringConfigWithDefault(config.MlmdWriterImagePath, config.DefaultImageValue),
			}
		}

		mlmdEnvoyImageFromConfig := config.GetStringConfigWithDefault(config.MlmdEnvoyImagePath, config.DefaultImageValue)
		mlmdGRPCImageFromConfig := config.GetStringConfigWithDefault(config.MlmdGRPCImagePath, config.DefaultImageValue)
		mlmdWriterImageFromConfig := config.GetStringConfigWithDefault(config.MlmdWriterImagePath, config.DefaultImageValue)

		setStringDefault(mlmdEnvoyImageFromConfig, &p.MLMD.Envoy.Image)
		setStringDefault(mlmdGRPCImageFromConfig, &p.MLMD.GRPC.Image)
		setStringDefault(mlmdWriterImageFromConfig, &p.MLMD.Writer.Image)

		setResourcesDefault(config.MlmdEnvoyResourceRequirements, &p.MLMD.Envoy.Resources)
		setResourcesDefault(config.MlmdGRPCResourceRequirements, &p.MLMD.GRPC.Resources)
		setResourcesDefault(config.MlmdWriterResourceRequirements, &p.MLMD.Writer.Resources)

		setStringDefault(config.MlmdGrpcPort, &p.MLMD.GRPC.Port)
	}
	return nil
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

func (p *DSPAParams) ExtractParams(ctx context.Context, dsp *dspa.DataSciencePipelinesApplication, client client.Client, loggr logr.Logger) error {
	p.Name = dsp.Name
	p.Namespace = dsp.Namespace
	p.Owner = dsp
	p.APIServer = dsp.Spec.APIServer.DeepCopy()
	p.APIServerDefaultResourceName = apiServerDefaultResourceNamePrefix + dsp.Name
	p.APIServerServiceName = fmt.Sprintf("%s-%s", config.DSPServicePrefix, p.Name)
	p.ScheduledWorkflow = dsp.Spec.ScheduledWorkflow.DeepCopy()
	p.ScheduledWorkflowDefaultResourceName = scheduledWorkflowDefaultResourceNamePrefix + dsp.Name
	p.PersistenceAgent = dsp.Spec.PersistenceAgent.DeepCopy()
	p.PersistentAgentDefaultResourceName = persistenceAgentDefaultResourceNamePrefix + dsp.Name
	p.MlPipelineUI = dsp.Spec.MlPipelineUI.DeepCopy()
	p.MariaDB = dsp.Spec.Database.MariaDB.DeepCopy()
	p.Minio = dsp.Spec.ObjectStorage.Minio.DeepCopy()
	p.OAuthProxy = config.GetStringConfigWithDefault(config.OAuthProxyImagePath, config.DefaultImageValue)
	p.MLMD = dsp.Spec.MLMD.DeepCopy()
	p.APIServerPiplinesCABundleMountPath = config.APIServerPiplinesCABundleMountPath
	p.PiplinesCABundleMountPath = config.PiplinesCABundleMountPath

	log := loggr.WithValues("namespace", p.Namespace).WithValues("dspa_name", p.Name)

	if p.APIServer != nil {

		serverImageFromConfig := config.GetStringConfigWithDefault(config.APIServerImagePath, config.DefaultImageValue)
		artifactImageFromConfig := config.GetStringConfigWithDefault(config.APIServerArtifactImagePath, config.DefaultImageValue)
		cacheImageFromConfig := config.GetStringConfigWithDefault(config.APIServerCacheImagePath, config.DefaultImageValue)
		moveResultsImageFromConfig := config.GetStringConfigWithDefault(config.APIServerMoveResultsImagePath, config.DefaultImageValue)

		setStringDefault(serverImageFromConfig, &p.APIServer.Image)
		setStringDefault(artifactImageFromConfig, &p.APIServer.ArtifactImage)
		setStringDefault(cacheImageFromConfig, &p.APIServer.CacheImage)
		setStringDefault(moveResultsImageFromConfig, &p.APIServer.MoveResultsImage)

		setResourcesDefault(config.APIServerResourceRequirements, &p.APIServer.Resources)

		if p.APIServer.ArtifactScriptConfigMap == nil {
			p.APIServer.ArtifactScriptConfigMap = &dspa.ArtifactScriptConfigMap{
				Name: config.ArtifactScriptConfigMapNamePrefix + dsp.Name,
				Key:  config.ArtifactScriptConfigMapKey,
			}
		}

		// Check for Global certs
		// If it exists, we use this cert
		// If no global cert provided, check if a custom bundle is provided via the DSPA
		globalCABundleCFGMapKey, globalCABundleCFGMapName := config.GlobalCaBundleConfigMapKey, config.GlobalCaBundleConfigMapName
		err, globalCerVal := util.GetConfigMapValue(ctx, globalCABundleCFGMapKey, globalCABundleCFGMapName, p.Namespace, client, log)
		if err != nil && apierrs.IsNotFound(err) {
			// If the global cert configmap is not available, that is OK
			// proceed to check if the user has provided their
			// own configmap via DSPA config
			if p.APIServer.CABundle != nil {
				dspaCaBundleCfgKey, dspaCaBundleCfgName := p.APIServer.CABundle.ConfigMapKey, p.APIServer.CABundle.ConfigMapName
				dspaCACfgErr, dspaProvidedCABundle := util.GetConfigMapValue(ctx, dspaCaBundleCfgKey, dspaCaBundleCfgName, p.Namespace, client, log)
				if dspaCACfgErr != nil && apierrs.IsNotFound(dspaCACfgErr) {
					log.Info(fmt.Sprintf("ConfigMap [%s] was not found in namespace [%s]", dspaCaBundleCfgKey, p.Namespace))
					return dspaCACfgErr
				} else if dspaCACfgErr != nil {
					log.Info(fmt.Sprintf("Encountered error when attempting to fetch ConfigMap: [%s], Error: %v", dspaCaBundleCfgName, dspaCACfgErr))
					return dspaCACfgErr
				}
				p.APICustomPemCerts = []byte(dspaProvidedCABundle)
			}
		} else if err != nil {
			log.Info(fmt.Sprintf("Encountered error when attempting to fetch ConfigMap: [%s], Error: %v", globalCABundleCFGMapKey, err))
			return err
		} else {
			// Found a global cert, consume this cert, takes precedence over "cABundle" provided via DSPA
			log.Info(fmt.Sprintf("Found global CA Bundle %s present in this namespace %s, this cert will be "+
				"included to verify external tls connections in this DSPA.", config.GlobalCaBundleConfigMapName, p.Namespace))
			p.APICustomPemCerts = []byte(globalCerVal)
			p.APIServer.CABundle = &dspa.CABundle{
				ConfigMapName: config.GlobalCaBundleConfigMapName,
				ConfigMapKey:  config.GlobalCaBundleConfigMapKey,
			}
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
	if p.MlPipelineUI != nil {
		if dsp.Spec.MlPipelineUI.Image == "" {
			return fmt.Errorf("mlPipelineUI specified, but no image provided in the DSPA CR Spec")
		}
		p.MlPipelineUI.Image = dsp.Spec.MlPipelineUI.Image
		setStringDefault(config.MLPipelineUIConfigMapPrefix+dsp.Name, &p.MlPipelineUI.ConfigMapName)
		setResourcesDefault(config.MlPipelineUIResourceRequirements, &p.MlPipelineUI.Resources)
	}

	err := p.SetupMLMD(ctx, dsp, client, log)
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

	return nil
}
