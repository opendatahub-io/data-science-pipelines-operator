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
	"time"

	mf "github.com/manifestival/manifestival"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
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
func ResourceDoesNotExists(uc UtilContext, path string) {
	manifest, err := mf.NewManifest(path, uc.Opts)
	Expect(err).NotTo(HaveOccurred())
	manifest, err = manifest.Transform(mf.InjectNamespace(uc.Ns))
	Expect(err).NotTo(HaveOccurred())
	u := manifest.Resources()[0]

	Eventually(func() error {
		_, err := manifest.Client.Get(&u)
		if err != nil {
			if apierrs.IsNotFound(err) {
				return nil
			}
			return err
		}
		return fmt.Errorf("resource still exists on cluster")

	}, timeout, interval).ShouldNot(HaveOccurred())

}

// DeployResource will deploy resource found in path by requesting
// a generic apply request to the client provided via uc.Opts
func DeployResource(uc UtilContext, path string) {
	manifest, err := mf.NewManifest(path, uc.Opts)
	Expect(err).NotTo(HaveOccurred())
	manifest, err = manifest.Transform(mf.InjectNamespace(uc.Ns))
	Expect(err).NotTo(HaveOccurred())
	err = manifest.Apply()
	Expect(err).NotTo(HaveOccurred())
	u := manifest.Resources()[0]
	Eventually(func() error {
		_, err := manifest.Client.Get(&u)
		return err
	}, timeout, interval).ShouldNot(HaveOccurred())
}

// DeleteResource will delete resource found in path by requesting
// a generic delete request to the client provided via uc.Opts
func DeleteResource(uc UtilContext, path string) {

	manifest, err := mf.NewManifest(path, uc.Opts)
	Expect(err).NotTo(HaveOccurred())
	manifest, err = manifest.Transform(mf.InjectNamespace(uc.Ns))
	Expect(err).NotTo(HaveOccurred())
	err = manifest.Delete()
	Expect(err).NotTo(HaveOccurred())
	u := manifest.Resources()[0]

	Eventually(func() error {
		_, err := manifest.Client.Get(&u)
		if err != nil {
			if apierrs.IsNotFound(err) {
				return nil
			}
			return err
		}
		return fmt.Errorf("resource still exists on cluster")

	}, timeout, interval).ShouldNot(HaveOccurred())

}

func compareEnvs(expected, actual []v1.EnvVar) string {
	errlist := ""
	if len(expected) != len(actual) {
		errlist = fmt.Sprintf("Container Env Lengths [expected: %d, actual: %d]\n", len(expected), len(actual))
	}

	// Iterate across expected list and check for each value
	for e_i, e := range expected {
		found := false
		for a_i, a := range actual {
			if e.Name == a.Name {
				found = true
				if e_i != a_i {
					errlist = fmt.Sprintf("%sExpected Env out-of-order: [%s]\n", errlist, e.Name)
				}
				if e.Value != a.Value {
					errlist = fmt.Sprintf("%sExpected Env Values do not match: [expected: %s, actual: %s]\n", errlist, e.Value, a.Value)
				}
				continue
			}
		}
		if !found {
			errlist = fmt.Sprintf("%sCould not find expected env: [%s]\n", errlist, e.Name)
		}
	}

	// Iterate across expected list and check for each value
	for _, a := range actual {
		found := false
		for _, e := range expected {
			if a.Name == e.Name {
				found = true
				continue
			}
		}
		if !found {
			errlist = fmt.Sprintf("%sExtra Env Found: [%s]\n", errlist, a.Name)
		}
	}

	return errlist
}

func getEnvFromUnstructured(obj *unstructured.Unstructured) []v1.EnvVar {
	var envVars []v1.EnvVar

	// Assuming 'obj' represents a Kubernetes Pod, you can retrieve the container's environment variables
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "containers")
	if len(containers) > 0 {
		container := containers[0].(map[string]interface{})
		env, _, _ := unstructured.NestedSlice(container, "env")
		for _, e := range env {
			envVar := e.(map[string]interface{})
			name := envVar["name"].(string)
			value := envVar["value"].(string)
			envVars = append(envVars, v1.EnvVar{Name: name, Value: value})
		}
	}

	return envVars
}

// CompareResources compares expected resource found locally
// in path and compares it against the resource found in the
// k8s cluster accessed via client defined in uc.Opts.
//
// Resource type is inferred dynamically. The resource found
// in path musth ave a supporting comparison procedure implemented.
//
// See testutil.CompareResourceProcs for supported procedures.
func CompareResources(uc UtilContext, path string) {
	manifest, err := mf.NewManifest(path, uc.Opts)
	Expect(err).NotTo(HaveOccurred())
	manifest, err = manifest.Transform(mf.InjectNamespace(uc.Ns))
	Expect(err).NotTo(HaveOccurred())
	expected := &manifest.Resources()[0]
	var actual *unstructured.Unstructured

	Eventually(func() error {
		var err error
		actual, err = manifest.Client.Get(expected)
		return err
	}, timeout, interval).ShouldNot(HaveOccurred())

	expectedEnv := getEnvFromUnstructured(expected)
	actualEnv := getEnvFromUnstructured(actual)

	envMismatch := compareEnvs(expectedEnv, actualEnv)
	if envMismatch != "" {
		errorMsg := fmt.Sprintf("Container Env Lengths [expected: %d, actual: %d]\n%s", len(expectedEnv), len(actualEnv), envMismatch)
		Expect(false).To(BeTrue(), errorMsg)
		return
	}
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
func GenerateDeclarativeTestCases() []Case {
	var testcases []Case

	cases, err := os.ReadDir(CasesDir)
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch cases in case dir.")
	for _, testcase := range cases {
		caseName := testcase.Name()
		caseDir := fmt.Sprintf("%s/%s", CasesDir, caseName)
		newCase := Case{}
		caseDeployDir := fmt.Sprintf("%s/deploy", caseDir)
		deploys, err := os.ReadDir(caseDeployDir)
		Expect(err).ToNot(HaveOccurred(), "Failed to read case.")
		for _, f := range deploys {
			newCase.Deploy = append(newCase.Deploy, fmt.Sprintf("%s/%s", caseDeployDir, f.Name()))
		}

		caseCreateDir := fmt.Sprintf("%s/expected/created", caseDir)
		caseCreationsFound, err := DirExists(caseCreateDir)
		Expect(err).ToNot(HaveOccurred(), "Failed to read 'create' dir.")
		if caseCreationsFound {
			toCreate, err := os.ReadDir(caseCreateDir)
			Expect(err).ToNot(HaveOccurred(), "Failed to read 'create' dir.")
			for _, f := range toCreate {
				newCase.Expected.Created = append(newCase.Expected.Created, fmt.Sprintf("%s/%s", caseCreateDir, f.Name()))
			}
		}

		caseNotCreateDir := fmt.Sprintf("%s/expected/not_created", caseDir)
		caseNoCreationsFound, err := DirExists(caseNotCreateDir)
		Expect(err).ToNot(HaveOccurred(), "Failed to read 'not_create' dir.")
		if caseNoCreationsFound {
			toNotCreate, err := os.ReadDir(caseNotCreateDir)
			Expect(err).ToNot(HaveOccurred(), "Failed to read 'not_create' dir.")
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
