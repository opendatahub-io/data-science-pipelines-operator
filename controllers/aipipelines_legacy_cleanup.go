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

package controllers

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	legacyDataSciencePipelinesName  = "default-datasciencepipelines"
	dataScienceClusterAPIGroup      = "datasciencecluster.opendatahub.io"
	legacyDataSciencePipelinesGroup = "components.platform.opendatahub.io"
	legacyDataSciencePipelinesKind  = "DataSciencePipelines"
)

var legacyDataSciencePipelinesGVK = schema.GroupVersionKind{
	Group:   legacyDataSciencePipelinesGroup,
	Version: "v1alpha1",
	Kind:    legacyDataSciencePipelinesKind,
}

// cleanupLegacyDataSciencePipelines completes the handoff from the in-tree
// platform component to this module. User-created or otherwise foreign legacy
// resources are preserved.
func cleanupLegacyDataSciencePipelines(
	ctx context.Context,
	cli client.Client,
) error {
	legacy := &unstructured.Unstructured{}
	legacy.SetGroupVersionKind(legacyDataSciencePipelinesGVK)
	legacy.SetName(legacyDataSciencePipelinesName)

	if err := cli.Get(ctx, client.ObjectKeyFromObject(legacy), legacy); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("get legacy DataSciencePipelines %s: %w", legacy.GetName(), err)
	}
	if !ownedByDataScienceCluster(legacy) {
		return nil
	}

	if err := cli.Delete(ctx, legacy); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete legacy DataSciencePipelines %s: %w", legacy.GetName(), err)
	}
	logf.FromContext(ctx).Info("deleted legacy DataSciencePipelines after AIPipelines module handoff", "name", legacy.GetName())
	return nil
}

func ownedByDataScienceCluster(object client.Object) bool {
	for _, owner := range object.GetOwnerReferences() {
		groupVersion, err := schema.ParseGroupVersion(owner.APIVersion)
		if err == nil && groupVersion.Group == dataScienceClusterAPIGroup && owner.Kind == "DataScienceCluster" {
			return true
		}
	}
	return false
}
