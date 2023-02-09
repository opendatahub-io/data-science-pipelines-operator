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
	"reflect"
)

func notEqualMsg(value string) {
	print(fmt.Sprintf("%s are not equal.", value))
}

func DeploymentsAreEqual(dp1 appsv1.Deployment, dp2 appsv1.Deployment) bool {

	if !reflect.DeepEqual(dp1.ObjectMeta.Labels, dp2.ObjectMeta.Labels) {
		notEqualMsg("labels")
		return false
	}

	if !reflect.DeepEqual(dp1.Spec.Selector, dp2.Spec.Selector) {
		notEqualMsg("selector")
		return false
	}

	if !reflect.DeepEqual(dp1.Spec.Template.ObjectMeta, dp2.Spec.Template.ObjectMeta) {
		notEqualMsg("selector")
		return false
	}

	if !reflect.DeepEqual(dp1.Spec.Template.Spec.Volumes, dp2.Spec.Template.Spec.Volumes) {
		notEqualMsg("Volumes")
		return false
	}

	if len(dp1.Spec.Template.Spec.Containers) != len(dp2.Spec.Template.Spec.Containers) {
		notEqualMsg("Containers")
		return false
	}
	for i := range dp1.Spec.Template.Spec.Containers {
		c1 := dp1.Spec.Template.Spec.Containers[i]
		c2 := dp2.Spec.Template.Spec.Containers[i]
		if !reflect.DeepEqual(c1.Env, c2.Env) {
			notEqualMsg("Container Env")
			return false
		}
		if !reflect.DeepEqual(c1.Ports, c2.Ports) {
			notEqualMsg("Container Ports")
			return false
		}
		if !reflect.DeepEqual(c1.Resources, c2.Resources) {
			notEqualMsg("Container Resources")
			return false
		}
		if !reflect.DeepEqual(c1.VolumeMounts, c2.VolumeMounts) {
			notEqualMsg("Container VolumeMounts")
			return false
		}
		if !reflect.DeepEqual(c1.Args, c2.Args) {
			notEqualMsg("Container Args")
			return false
		}
		if c1.Name != c2.Name {
			notEqualMsg("Container Name")
			return false
		}
		if c1.Image != c2.Image {
			notEqualMsg("Container Image")
			return false
		}
	}

	return true
}
