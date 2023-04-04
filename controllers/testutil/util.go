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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"reflect"
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

func ResourceDoesNotExists(uc UtilContext, expected, actual client.Object, path string) {
	Expect(convertToStructuredResource(path, expected, uc.Opts, uc.Ns)).NotTo(HaveOccurred())
	namespacedNamed := types.NamespacedName{
		Name:      expected.GetName(),
		Namespace: uc.Ns,
	}
	Eventually(func() error {
		err := uc.Client.Get(uc.Ctx, namespacedNamed, actual)
		if err != nil {
			if apierrs.IsNotFound(err) {
				return nil
			}
			return err
		}
		return fmt.Errorf("resource still exists on cluster")

	}, timeout, interval).ShouldNot(HaveOccurred())

}

func DeployResource(uc UtilContext, res client.Object, path string) {
	err := convertToStructuredResource(path, res, uc.Opts, uc.Ns)
	Expect(err).NotTo(HaveOccurred())
	name := res.GetName()
	Expect(uc.Client.Create(uc.Ctx, res)).Should(Succeed())
	Eventually(func() error {
		namespacedNamed := types.NamespacedName{Name: name, Namespace: uc.Ns}
		return uc.Client.Get(uc.Ctx, namespacedNamed, res)
	}, timeout, interval).ShouldNot(HaveOccurred())
}

func DeleteResource(uc UtilContext, res client.Object, path string) {
	err := convertToStructuredResource(path, res, uc.Opts, uc.Ns)
	Expect(err).NotTo(HaveOccurred())
	name := res.GetName()

	Eventually(func() error {
		return uc.Client.Delete(uc.Ctx, res)
	}, timeout, interval).ShouldNot(HaveOccurred())

	Eventually(func() error {
		namespacedNamed := types.NamespacedName{Name: name, Namespace: uc.Ns}
		err = uc.Client.Get(uc.Ctx, namespacedNamed, res)
		if err != nil {
			if apierrs.IsNotFound(err) {
				return nil
			}
			return err
		}
		return fmt.Errorf("resource still exists on cluster")

	}, timeout, interval).ShouldNot(HaveOccurred())

}

func CompareResources(uc UtilContext, expected, actual client.Object, path string) {
	Expect(convertToStructuredResource(path, expected, uc.Opts, uc.Ns)).NotTo(HaveOccurred())
	Eventually(func() error {
		namespacedNamed := types.NamespacedName{Name: expected.GetName(), Namespace: uc.Ns}
		return uc.Client.Get(uc.Ctx, namespacedNamed, actual)
	}, timeout, interval).ShouldNot(HaveOccurred())

	resType := reflect.TypeOf(expected).Elem().Name()
	Expect(compareResourceProcs[resType](expected, actual)).Should(BeTrue())

}

func convertToStructuredResource(path string, out interface{}, opts mf.Option, namespace string) error {
	m, err := mf.ManifestFrom(mf.Recursive(path), opts)
	if err != nil {
		return err
	}
	m, err = m.Transform(mf.InjectNamespace(namespace))
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	err = scheme.Scheme.Convert(&m.Resources()[0], out, nil)
	if err != nil {
		return err
	}
	return nil
}

func notEqualMsg(value string) {
	print(fmt.Sprintf("%s are not equal.", value))
}

func configMapsAreEqual(expected, actual client.Object) bool {
	expectedConfigMap := expected.(*v1.ConfigMap)
	actualConfigMap := actual.(*v1.ConfigMap)
	if expectedConfigMap.Name != actualConfigMap.Name {
		notEqualMsg("Configmap Names are not equal.")
		return false
	}

	if !reflect.DeepEqual(expectedConfigMap.Data, actualConfigMap.Data) {
		notEqualMsg("Configmap's Data values")
		return false
	}
	return true
}

func secretsAreEqual(expected, actual client.Object) bool {
	expectedSecret := expected.(*v1.Secret)
	actualSecret := actual.(*v1.Secret)
	if expectedSecret.Name != actualSecret.Name {
		notEqualMsg("Secret Names are not equal.")
		return false
	}

	if !reflect.DeepEqual(expectedSecret.Data, actualSecret.Data) {
		notEqualMsg("Secret's Data values")
		return false
	}
	return true
}

func deploymentsAreEqual(expected, actual client.Object) bool {
	expectedDep := expected.(*appsv1.Deployment)
	actualDep := actual.(*appsv1.Deployment)
	if !reflect.DeepEqual(expectedDep.ObjectMeta.Labels, actualDep.ObjectMeta.Labels) {
		notEqualMsg("labels")
		return false
	}

	if !reflect.DeepEqual(expectedDep.Spec.Selector, actualDep.Spec.Selector) {
		notEqualMsg("selector")
		return false
	}

	if !reflect.DeepEqual(expectedDep.Spec.Template.ObjectMeta, actualDep.Spec.Template.ObjectMeta) {
		notEqualMsg("selector")
		return false
	}

	if !reflect.DeepEqual(expectedDep.Spec.Template.Spec.Volumes, actualDep.Spec.Template.Spec.Volumes) {
		notEqualMsg("Volumes")
		return false
	}

	if len(expectedDep.Spec.Template.Spec.Containers) != len(actualDep.Spec.Template.Spec.Containers) {
		notEqualMsg("Containers")
		return false
	}
	for i := range expectedDep.Spec.Template.Spec.Containers {
		expectedContainer := expectedDep.Spec.Template.Spec.Containers[i]
		actualContainer := actualDep.Spec.Template.Spec.Containers[i]
		if !reflect.DeepEqual(expectedContainer.Env, actualContainer.Env) {
			notEqualMsg("Container Env")
			return false
		}
		if !reflect.DeepEqual(expectedContainer.Ports, actualContainer.Ports) {
			notEqualMsg("Container Ports")
			return false
		}
		if !reflect.DeepEqual(expectedContainer.Resources, actualContainer.Resources) {
			notEqualMsg("Container Resources")
			return false
		}
		if !reflect.DeepEqual(expectedContainer.VolumeMounts, actualContainer.VolumeMounts) {
			notEqualMsg("Container VolumeMounts")
			return false
		}
		if !reflect.DeepEqual(expectedContainer.Args, actualContainer.Args) {
			notEqualMsg("Container Args")
			return false
		}
		if expectedContainer.Name != actualContainer.Name {
			notEqualMsg("Container Name")
			return false
		}
		if expectedContainer.Image != actualContainer.Image {
			notEqualMsg("Container Image")
			return false
		}
	}

	return true
}

var compareResourceProcs = map[string]func(expected, actual client.Object) bool{
	"Secret":     secretsAreEqual,
	"ConfigMap":  configMapsAreEqual,
	"Deployment": deploymentsAreEqual,
}
