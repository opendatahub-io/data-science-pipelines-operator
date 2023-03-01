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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"reflect"
)

func notEqualMsg(value string) {
	print(fmt.Sprintf("%s are not equal.", value))
}

func ConfigMapsAreEqual(expected v1.ConfigMap, actual v1.ConfigMap) bool {
	if expected.Name != actual.Name {
		notEqualMsg("Configmap Names are not equal.")
		return false
	}

	if !reflect.DeepEqual(expected.Data, actual.Data) {
		notEqualMsg("Configmap's Data values")
		return false
	}
	return true
}

func DeploymentsAreEqual(expectedDep appsv1.Deployment, actualDep appsv1.Deployment) bool {

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
