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

// Package argo embeds the shared Argo resources that are reconciled by the
// modular AIPipelines controller. Keeping this list explicit prevents
// kustomization and parameter files from accidentally becoming API objects.
package argo

import (
	"bytes"
	"embed"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var assetNames = []string{
	"clusterrole.argo-aggregate-to-admin.yaml",
	"clusterrole.argo-aggregate-to-edit.yaml",
	"clusterrole.argo-aggregate-to-view.yaml",
	"clusterrole.argo-cluster-role.yaml",
	"clusterrolebinding.ds-pipeline-argo-binding.yaml",
	"configmap.workflow-controller-configmap.yaml",
	"role.argo.yaml",
	"rolebinding.argo-binding.yaml",
	"serviceaccount.argo.yaml",
	"crd.applications.yaml",
	"crd.clusterworkflowtemplates.yaml",
	"crd.cronworkflows.yaml",
	"crd.viewers.yaml",
	"crd.workflowartifactgctasks.yaml",
	"crd.workfloweventbinding.yaml",
	"crd.workflows.yaml",
	"crd.workflowtaskresult.yaml",
	"crd.workflowtaskset.yaml",
	"crd.workflowtemplate.yaml",
}

//go:embed *.yaml
var assets embed.FS

// Objects returns a fresh copy of every shared Argo manifest.
func Objects(namespace string) ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0, len(assetNames))
	for _, name := range assetNames {
		data, err := assets.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read embedded Argo asset %s: %w", name, err)
		}

		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
		for {
			object := &unstructured.Unstructured{}
			if err := decoder.Decode(&object.Object); err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("decode embedded Argo asset %s: %w", name, err)
			}
			if len(object.Object) == 0 {
				continue
			}
			if object.GetNamespace() != "" {
				object.SetNamespace(namespace)
			}
			if object.GetKind() == "RoleBinding" || object.GetKind() == "ClusterRoleBinding" {
				subjects, found, err := unstructured.NestedSlice(object.Object, "subjects")
				if err != nil {
					return nil, fmt.Errorf("read subjects from embedded Argo asset %s: %w", name, err)
				}
				if found {
					for i := range subjects {
						subject, ok := subjects[i].(map[string]interface{})
						if ok && subject["kind"] == "ServiceAccount" {
							subject["namespace"] = namespace
						}
					}
					if err := unstructured.SetNestedSlice(object.Object, subjects, "subjects"); err != nil {
						return nil, fmt.Errorf("set subjects from embedded Argo asset %s: %w", name, err)
					}
				}
			}
			objects = append(objects, object)
		}
	}

	return objects, nil
}
