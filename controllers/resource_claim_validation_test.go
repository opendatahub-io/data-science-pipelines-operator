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
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *ControllerSuite) TestResourceClaimCRDValidation() {
	tests := []struct {
		name    string
		claim   dspav1.PodResourceClaim
		wantErr string
	}{
		{
			name: "direct claim",
			claim: dspav1.PodResourceClaim{
				Name:              "gpu",
				ResourceClaimName: ptr.To("existing-gpu"),
			},
		},
		{
			name: "claim template",
			claim: dspav1.PodResourceClaim{
				Name:                      "gpu",
				ResourceClaimTemplateName: ptr.To("gpu-template"),
			},
		},
		{
			name:    "missing claim source",
			claim:   dspav1.PodResourceClaim{Name: "gpu"},
			wantErr: "exactly one of resourceClaimName and resourceClaimTemplateName must be set",
		},
		{
			name: "both claim sources",
			claim: dspav1.PodResourceClaim{
				Name:                      "gpu",
				ResourceClaimName:         ptr.To("existing-gpu"),
				ResourceClaimTemplateName: ptr.To("gpu-template"),
			},
			wantErr: "exactly one of resourceClaimName and resourceClaimTemplateName must be set",
		},
		{
			name: "invalid pod claim name",
			claim: dspav1.PodResourceClaim{
				Name:              "gpu\"\nhostNetwork: true",
				ResourceClaimName: ptr.To("existing-gpu"),
			},
			wantErr: "spec.apiServer.resourceClaims[0].name",
		},
		{
			name: "invalid direct claim name",
			claim: dspav1.PodResourceClaim{
				Name:              "gpu",
				ResourceClaimName: ptr.To("existing\"\nhostNetwork: true"),
			},
			wantErr: "spec.apiServer.resourceClaims[0].resourceClaimName",
		},
		{
			name: "invalid claim template name",
			claim: dspav1.PodResourceClaim{
				Name:                      "gpu",
				ResourceClaimTemplateName: ptr.To("template\"\nhostNetwork: true"),
			},
			wantErr: "spec.apiServer.resourceClaims[0].resourceClaimTemplateName",
		},
	}

	for idx, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			namespace := fmt.Sprintf("resource-claim-validation-%d", idx)
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			require.NoError(t, ctrlclient.IgnoreAlreadyExists(k8sClient.Create(ctx, ns)))

			dspa := testutil.CreateEmptyDSPA()
			dspa.Name = fmt.Sprintf("resource-claim-validation-%d", idx)
			dspa.Namespace = namespace
			dspa.Spec.MLMD.Deploy = false
			dspa.Spec.APIServer.ResourceClaims = []dspav1.PodResourceClaim{tt.claim}
			t.Cleanup(func() {
				_ = ctrlclient.IgnoreNotFound(k8sClient.Delete(ctx, dspa))
			})

			err := k8sClient.Create(ctx, dspa)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
