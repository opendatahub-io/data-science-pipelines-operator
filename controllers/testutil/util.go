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

package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	mf "github.com/manifestival/manifestival"
	appsv1 "k8s.io/api/apps/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 2
	CasesDir = "./testdata/declarative"
)

type UtilContext struct {
	Ctx    context.Context
	Ns     string
	Opts   mf.Option
	Client client.Client
}

type Case struct {
	Description string
	Config      string
	Deploy      []string
	Expected    Expectation
}

type Expectation struct {
	Created    []string
	NotCreated []string
}

// ResourceDoesNotExists will check against the client provided
// by uc.Opts whether resource at path exists.
func ResourceDoesNotExists(uc UtilContext, path string, t *testing.T) {
	manifest, err := mf.NewManifest(path, uc.Opts)
	assert.NoError(t, err)
	manifest, err = manifest.Transform(mf.InjectNamespace(uc.Ns))
	assert.NoError(t, err)
	u := manifest.Resources()[0]

	assert.Eventually(t, func() bool {
		_, err := manifest.Client.Get(&u)
		if err != nil {
			return apierrs.IsNotFound(err)
		}
		return false
	}, timeout, interval)
}

// DeployResource will deploy resource found in path by requesting
// a generic apply request to the client provided via uc.Opts
func DeployResource(uc UtilContext, path string, t *testing.T) {
	manifest, err := mf.NewManifest(path, uc.Opts)
	assert.NoError(t, err)
	manifest, err = manifest.Transform(mf.InjectNamespace(uc.Ns))
	assert.NoError(t, err)
	err = manifest.Apply()
	assert.NoError(t, err)
	u := manifest.Resources()[0]
	assert.Eventually(t, func() bool {
		_, err := manifest.Client.Get(&u)
		return err == nil
	}, timeout, interval)
}

// DeleteResource will delete resource found in path by requesting
// a generic delete request to the client provided via uc.Opts
func DeleteResource(uc UtilContext, path string, t *testing.T) {

	manifest, err := mf.NewManifest(path, uc.Opts)
	assert.NoError(t, err)
	manifest, err = manifest.Transform(mf.InjectNamespace(uc.Ns))
	assert.NoError(t, err)
	err = manifest.Delete()
	assert.NoError(t, err)
	u := manifest.Resources()[0]

	assert.Eventually(t, func() bool {
		_, err := manifest.Client.Get(&u)
		if err != nil {
			return apierrs.IsNotFound(err)
		}
		return false
	}, timeout, interval)
}

// CompareResources compares expected resource found locally
// in path and compares it against the resource found in the
// k8s cluster accessed via client defined in uc.Opts.
//
// Resource type is inferred dynamically. The resource found
// in path musth ave a supporting comparison procedure implemented.
//
// See testutil.CompareResourceProcs for supported procedures.
func CompareResources(uc UtilContext, path string, t *testing.T) {
	manifest, err := mf.NewManifest(path, uc.Opts)
	require.NoError(t, err)
	manifest, err = manifest.Transform(mf.InjectNamespace(uc.Ns))
	require.NoError(t, err)
	expected := &manifest.Resources()[0]
	var actual *unstructured.Unstructured

	require.Eventually(t, func() bool {
		var err error
		actual, err = manifest.Client.Get(expected)
		if err != nil {
			ctrl.Log.Info("Error when trying to get " + expected.GetName() + " (will keep trying until reaching timeout): " + err.Error())
		}
		return err == nil
	}, timeout, interval)

	rest := expected.Object["kind"].(string)
	result, err := CompareResourceProcs[rest](expected, actual)
	require.NoError(t, err)
	require.True(t, result)
}

// DirExists checks whether dir at path exists
func DirExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// GenerateDeclarativeTestCases dynamically generate
// testcases based on resources located in the testdata
// directory.
func GenerateDeclarativeTestCases(t *testing.T) []Case {
	var testcases []Case

	cases, err := os.ReadDir(CasesDir)
	assert.NoError(t, err, "Failed to fetch cases in case dir.")
	for _, testcase := range cases {
		caseName := testcase.Name()
		caseDir := fmt.Sprintf("%s/%s", CasesDir, caseName)
		newCase := Case{}
		caseDeployDir := fmt.Sprintf("%s/deploy", caseDir)
		deploys, err := os.ReadDir(caseDeployDir)
		assert.NoError(t, err, "Failed to read case.")
		for _, f := range deploys {
			newCase.Deploy = append(newCase.Deploy, fmt.Sprintf("%s/%s", caseDeployDir, f.Name()))
		}

		caseCreateDir := fmt.Sprintf("%s/expected/created", caseDir)
		caseCreationsFound, err := DirExists(caseCreateDir)
		assert.NoError(t, err, "Failed to read 'create' dir.")
		if caseCreationsFound {
			toCreate, err := os.ReadDir(caseCreateDir)
			assert.NoError(t, err, "Failed to read 'create' dir.")
			for _, f := range toCreate {
				newCase.Expected.Created = append(newCase.Expected.Created, fmt.Sprintf("%s/%s", caseCreateDir, f.Name()))
			}
		}

		caseNotCreateDir := fmt.Sprintf("%s/expected/not_created", caseDir)
		caseNoCreationsFound, err := DirExists(caseNotCreateDir)
		assert.NotEqual(t, err, "Failed to read 'not_create' dir.")
		if caseNoCreationsFound {
			toNotCreate, err := os.ReadDir(caseNotCreateDir)
			assert.NotEqual(t, err, "Failed to read 'not_create' dir.")
			for _, f := range toNotCreate {
				newCase.Expected.NotCreated = append(newCase.Expected.NotCreated, fmt.Sprintf("%s/%s", caseNotCreateDir, f.Name()))
			}
		}

		newCase.Description = fmt.Sprintf("[%s] - When a DSPA is deployed", caseName)

		newCase.Config = fmt.Sprintf("%s/config.yaml", caseDir)

		testcases = append(testcases, newCase)
	}

	return testcases
}

func CreateEmptyDSPA() *dspav1.DataSciencePipelinesApplication {
	dspa := &dspav1.DataSciencePipelinesApplication{
		Spec: dspav1.DSPASpec{
			PodToPodTLS:       boolPtr(false),
			APIServer:         &dspav1.APIServer{Deploy: false},
			MLMD:              &dspav1.MLMD{Deploy: true}, // MLMD is required
			PersistenceAgent:  &dspav1.PersistenceAgent{Deploy: false},
			ScheduledWorkflow: &dspav1.ScheduledWorkflow{Deploy: false},
			MlPipelineUI: &dspav1.MlPipelineUI{
				Deploy: false,
				Image:  "testimage-MlPipelineUI:test",
			},
			WorkflowController: &dspav1.WorkflowController{Deploy: false},
			Database:           &dspav1.Database{DisableHealthCheck: false, MariaDB: &dspav1.MariaDB{Deploy: false}},
			ObjectStorage: &dspav1.ObjectStorage{
				DisableHealthCheck: false,
				Minio: &dspav1.Minio{
					Deploy: false,
					Image:  "testimage-Minio:test",
				},
			},
		},
	}
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"
	return dspa
}

func CreateDSPAWithAPIServerCABundle(key string, cfgmapName string) *dspav1.DataSciencePipelinesApplication {
	dspa := CreateEmptyDSPA()
	dspa.Spec.APIServer = &dspav1.APIServer{
		Deploy: true,
		CABundle: &dspav1.CABundle{
			ConfigMapKey:  key,
			ConfigMapName: cfgmapName,
		},
	}
	return dspa
}

func CreateDSPAWithAPIServerPodtoPodTlsEnabled() *dspav1.DataSciencePipelinesApplication {
	dspa := CreateEmptyDSPA()
	dspa.Spec.DSPVersion = "v2"
	dspa.Spec.APIServer = &dspav1.APIServer{
		Deploy: true,
	}
	dspa.Spec.MLMD.Deploy = true
	dspa.Spec.PodToPodTLS = boolPtr(true)

	return dspa
}

func boolPtr(b bool) *bool {
	return &b
}

func CreateDSPAWithCustomKfpLauncherConfigMap(configMapName string) *dspav1.DataSciencePipelinesApplication {
	dspa := CreateEmptyDSPA()
	dspa.Spec.DSPVersion = "v2"
	// required, or we get an error because OCP certs aren't found
	dspa.Spec.PodToPodTLS = boolPtr(false)
	// required, or we get an error because this is required in v2
	dspa.Spec.MLMD.Deploy = true
	dspa.Spec.APIServer = &dspav1.APIServer{
		Deploy:                     true,
		CustomKfpLauncherConfigMap: configMapName,
	}
	return dspa
}

func CreateTestDSPA() *dspav1.DataSciencePipelinesApplication {
	dspa := CreateEmptyDSPA()
	dspa.Name = "testdspa"
	dspa.Namespace = "testnamespace"

	dspa.Spec = dspav1.DSPASpec{
		PodToPodTLS: boolPtr(false),
		APIServer: &dspav1.APIServer{
			Deploy:        true,
			PipelineStore: "kubernetes",
		},
		MLMD: &dspav1.MLMD{
			Deploy: true,
		},
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
		DSPVersion: "v2",
	}

	return dspa
}

func CreateTestDSPODeployment(namespace string) *appsv1.Deployment {
	operatorName := "data-science-pipelines-operator-controller-manager"
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "dspo"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "dspo"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "manager", Image: "testimage"},
					},
				},
			},
		},
	}
}
