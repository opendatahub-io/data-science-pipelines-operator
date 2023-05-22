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
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
)

// CompareResourceProcs maps object kind's to their associated comparison procedures.
// To add new comparison procedures, ensure they are added to compareResourceProcs
// and follow the same signature.
var CompareResourceProcs = map[string]func(expected, actual *unstructured.Unstructured) (bool, error){
	"Secret":     secretsAreEqual,
	"ConfigMap":  configMapsAreEqual,
	"Deployment": deploymentsAreEqual,
}

func configMapsAreEqual(expected, actual *unstructured.Unstructured) (bool, error) {
	expectedConfigMap := &v1.ConfigMap{}
	actualConfigMap := &v1.ConfigMap{}
	err := scheme.Scheme.Convert(expected, expectedConfigMap, nil)
	if err != nil {
		return false, err
	}
	err = scheme.Scheme.Convert(actual, actualConfigMap, nil)
	if err != nil {
		return false, err
	}

	if expectedConfigMap.Name != actualConfigMap.Name {
		return false, notEqualMsg("Configmap Names are not equal.")
	}

	if !reflect.DeepEqual(expectedConfigMap.Data, actualConfigMap.Data) {
		return false, notEqualMsg("Configmap's Data values")
	}
	return true, nil
}

func secretsAreEqual(expected, actual *unstructured.Unstructured) (bool, error) {
	expectedSecret := &v1.Secret{}
	actualSecret := &v1.Secret{}
	err := scheme.Scheme.Convert(expected, expectedSecret, nil)
	if err != nil {
		return false, err
	}
	err = scheme.Scheme.Convert(actual, actualSecret, nil)
	if err != nil {
		return false, err
	}

	if expectedSecret.Name != actualSecret.Name {
		return false, notEqualMsg("Secret Names are not equal.")
	}

	if !reflect.DeepEqual(expectedSecret.Data, actualSecret.Data) {
		return false, notEqualMsg("Secret's Data values")
	}
	return true, nil
}

func deploymentsAreEqual(expected, actual *unstructured.Unstructured) (bool, error) {
	expectedDep := &appsv1.Deployment{}
	actualDep := &appsv1.Deployment{}
	err := scheme.Scheme.Convert(expected, expectedDep, nil)
	if err != nil {
		return false, err
	}
	err = scheme.Scheme.Convert(actual, actualDep, nil)
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(expectedDep.ObjectMeta.Labels, actualDep.ObjectMeta.Labels) {
		return false, notEqualMsg("labels")
	}

	if !reflect.DeepEqual(expectedDep.Spec.Selector, actualDep.Spec.Selector) {
		return false, notEqualMsg("selector")
	}

	if !reflect.DeepEqual(expectedDep.Spec.Template.ObjectMeta, actualDep.Spec.Template.ObjectMeta) {
		return false, notEqualMsg("spec template")
	}

	if !reflect.DeepEqual(expectedDep.Spec.Template.Spec.Volumes, actualDep.Spec.Template.Spec.Volumes) {
		return false, notEqualMsg("Volumes")
	}

	if len(expectedDep.Spec.Template.Spec.Containers) != len(actualDep.Spec.Template.Spec.Containers) {
		return false, notEqualMsg("Containers")
	}
	for i := range expectedDep.Spec.Template.Spec.Containers {
		expectedContainer := expectedDep.Spec.Template.Spec.Containers[i]
		actualContainer := actualDep.Spec.Template.Spec.Containers[i]

		if len(expectedContainer.Env) != len(actualContainer.Env) {
			return false, notEqualMsg("Container Env Lengths ")
		}
		// Check each env individually for a more meaningful response upon failure.
		for i, expectedEnv := range expectedContainer.Env {
			actualEnv := actualContainer.Env[i]
			if !reflect.DeepEqual(expectedEnv, actualEnv) {
				return false, notEqualMsg(fmt.Sprintf("Container Env [expected: %s=%s, actual: %s=%s]", expectedEnv.Name, expectedEnv.Value, actualEnv.Name, actualEnv.Value))
			}
		}

		if !reflect.DeepEqual(expectedContainer.Ports, actualContainer.Ports) {
			return false, notEqualMsg("Container Ports")
		}
		if !reflect.DeepEqual(expectedContainer.Resources, actualContainer.Resources) {
			return false, notEqualMsg("Container Resources")
		}
		if !reflect.DeepEqual(expectedContainer.VolumeMounts, actualContainer.VolumeMounts) {
			return false, notEqualMsg("Container VolumeMounts")
		}
		if !reflect.DeepEqual(expectedContainer.Args, actualContainer.Args) {
			return false, notEqualMsg("Container Args")
		}
		if expectedContainer.Name != actualContainer.Name {
			return false, notEqualMsg("Container Name")
		}
		if expectedContainer.Image != actualContainer.Image {
			return false, notEqualMsg(fmt.Sprintf("Container Image [expected: %s, actual: %s]", expectedContainer.Image, actualContainer.Image))
		}
	}

	return true, nil
}

func notEqualMsg(value string) error {
	return fmt.Errorf("%s are not equal", value)
}
