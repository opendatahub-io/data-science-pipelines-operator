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
	mf "github.com/manifestival/manifestival"
	_ "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 2
)

type UtilContext struct {
	Ctx    context.Context
	Ns     string
	Opts   mf.Option
	Client client.Client
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

	rest := expected.Object["kind"].(string)
	result, err := CompareResourceProcs[rest](expected, actual)
	Expect(err).NotTo(HaveOccurred())
	Expect(result).Should(BeTrue())
}
