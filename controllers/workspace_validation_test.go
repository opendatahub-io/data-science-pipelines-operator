//go:build test_all || test_functional

/*

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
	"fmt"
	"testing"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/testutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *ControllerSuite) TestAPIServerWorkspaceCRDValidation() {
	cases := []struct {
		name      string
		workspace *dspav1.APIServerWorkspace
		expectErr string
	}{
		{
			name: "missing-access-modes",
			workspace: &dspav1.APIServerWorkspace{
				VolumeClaimTemplateSpec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: stringPtr("standard-csi"),
				},
			},
			expectErr: "spec.apiServer.workspace.volumeClaimTemplateSpec.accessModes must be provided",
		},
		{
			name: "missing-storage-class",
			workspace: &dspav1.APIServerWorkspace{
				VolumeClaimTemplateSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				},
			},
			expectErr: "spec.apiServer.workspace.volumeClaimTemplateSpec.storageClassName must be provided",
		},
		{
			name: "valid-workspace",
			workspace: &dspav1.APIServerWorkspace{
				VolumeClaimTemplateSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					StorageClassName: stringPtr("standard-csi"),
				},
			},
		},
	}

	for idx, tc := range cases {
		s.T().Run(tc.name, func(t *testing.T) {
			nsName := fmt.Sprintf("workspace-validation-ns-%d", idx)
			dspa := testutil.CreateEmptyDSPA()
			dspa.Name = fmt.Sprintf("workspace-validation-%d", idx)
			dspa.Namespace = nsName
			dspa.Spec.APIServer = &dspav1.APIServer{Deploy: true, Workspace: tc.workspace}

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			err := k8sClient.Create(ctx, ns)
			require.NoError(t, ctrlclient.IgnoreAlreadyExists(err))
			t.Cleanup(func() {
				_ = ctrlclient.IgnoreNotFound(k8sClient.Delete(ctx, dspa))
			})

			err = k8sClient.Create(ctx, dspa)
			if tc.expectErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func stringPtr(v string) *string {
	return &v
}
