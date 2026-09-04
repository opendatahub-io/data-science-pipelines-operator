/*
Copyright 2026.

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

// Package v1alpha1 contains API schema definitions for the components.platform v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=components.platform.opendatahub.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion identifies the AIPipelines module API group and version.
	GroupVersion = schema.GroupVersion{Group: "components.platform.opendatahub.io", Version: "v1alpha1"}

	// SchemeBuilder registers the AIPipelines module API with a runtime scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the AIPipelines module API to a runtime scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
