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

package v1alpha1_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/opendatahub-io/data-science-pipelines-operator/api/aipipelines/v1alpha1"
	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	commonvalidation "github.com/opendatahub-io/odh-platform-utilities/api/common/validation"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const generatedCRDPath = "../../../config/crd/bases/components.platform.opendatahub.io_aipipelines.yaml"

func TestGroupVersion(t *testing.T) {
	if v1alpha1.GroupVersion.Group != "components.platform.opendatahub.io" {
		t.Fatalf("unexpected API group: %s", v1alpha1.GroupVersion.Group)
	}
	if v1alpha1.GroupVersion.Version != "v1alpha1" {
		t.Fatalf("unexpected API version: %s", v1alpha1.GroupVersion.Version)
	}
}

func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("registering AIPipelines API: %v", err)
	}

	for _, object := range []runtime.Object{&v1alpha1.AIPipelines{}, &v1alpha1.AIPipelinesList{}} {
		gvks, _, err := scheme.ObjectKinds(object)
		if err != nil {
			t.Errorf("type %T is not registered: %v", object, err)
			continue
		}
		if len(gvks) != 1 || gvks[0].GroupVersion() != v1alpha1.GroupVersion {
			t.Errorf("unexpected GVKs for %T: %v", object, gvks)
		}
	}
}

func TestPlatformObjectContract(t *testing.T) {
	commonvalidation.ValidatePlatformObject(t, &v1alpha1.AIPipelines{})
}

func TestArgoWorkflowsControllersManagementState(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha1.AIPipelinesSpec
		want common.ManagementState
	}{
		{name: "omitted configuration", want: common.Managed},
		{
			name: "empty management state",
			spec: v1alpha1.AIPipelinesSpec{
				ArgoWorkflowsControllers: &v1alpha1.ArgoWorkflowsControllersSpec{},
			},
			want: common.Managed,
		},
		{
			name: "explicit removed",
			spec: v1alpha1.AIPipelinesSpec{
				ArgoWorkflowsControllers: &v1alpha1.ArgoWorkflowsControllersSpec{ManagementState: common.Removed},
			},
			want: common.Removed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.ArgoWorkflowsControllersManagementState(); got != tt.want {
				t.Errorf("management state = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeepCopy(t *testing.T) {
	original := &v1alpha1.AIPipelines{
		Spec: v1alpha1.AIPipelinesSpec{
			ArgoWorkflowsControllers: &v1alpha1.ArgoWorkflowsControllersSpec{ManagementState: common.Managed},
		},
		Status: v1alpha1.AIPipelinesStatus{
			Status: common.Status{
				Phase: common.PhaseReady,
				Conditions: []common.Condition{
					{Type: string(common.ConditionTypeReady), Status: metav1.ConditionTrue},
				},
			},
			ComponentReleaseStatus: common.ComponentReleaseStatus{
				Releases: []common.ComponentRelease{{Name: common.ReleasePlatform, Version: "3.6.0"}},
			},
		},
	}

	copy := original.DeepCopy()
	copy.Spec.ArgoWorkflowsControllers.ManagementState = common.Removed
	copy.Status.Conditions[0].Status = metav1.ConditionFalse
	copy.Status.Releases[0].Version = "3.7.0"

	if got := original.Spec.ArgoWorkflowsControllers.ManagementState; got != common.Managed {
		t.Errorf("original management state changed to %q", got)
	}
	if got := original.Status.Conditions[0].Status; got != metav1.ConditionTrue {
		t.Errorf("original condition status changed to %q", got)
	}
	if got := original.Status.Releases[0].Version; got != "3.6.0" {
		t.Errorf("original release version changed to %q", got)
	}
}

func TestGeneratedCRDSchema(t *testing.T) {
	data, err := os.ReadFile(generatedCRDPath)
	if err != nil {
		t.Fatalf("reading generated CRD: %v", err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(data, crd); err != nil {
		t.Fatalf("unmarshalling generated CRD: %v", err)
	}

	if crd.Name != "aipipelines.components.platform.opendatahub.io" {
		t.Errorf("unexpected CRD name: %s", crd.Name)
	}
	if crd.Spec.Group != v1alpha1.GroupVersion.Group {
		t.Errorf("unexpected CRD group: %s", crd.Spec.Group)
	}
	if crd.Spec.Scope != apiextensionsv1.ClusterScoped {
		t.Errorf("scope = %s, want %s", crd.Spec.Scope, apiextensionsv1.ClusterScoped)
	}
	if crd.Spec.Names.Kind != v1alpha1.AIPipelinesKind ||
		crd.Spec.Names.ListKind != v1alpha1.AIPipelinesKind+"List" ||
		crd.Spec.Names.Plural != v1alpha1.AIPipelinesComponentName ||
		crd.Spec.Names.Singular != v1alpha1.AIPipelinesComponentName {
		t.Errorf("unexpected CRD names: %+v", crd.Spec.Names)
	}

	if len(crd.Spec.Versions) != 1 {
		t.Fatalf("expected one CRD version, got %d", len(crd.Spec.Versions))
	}
	version := crd.Spec.Versions[0]
	if version.Name != v1alpha1.GroupVersion.Version || !version.Served || !version.Storage {
		t.Errorf("unexpected CRD version: %+v", version)
	}
	if version.Subresources == nil || version.Subresources.Status == nil {
		t.Error("status subresource is not enabled")
	}
	if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
		t.Fatal("OpenAPI v3 schema is missing")
	}

	schema := version.Schema.OpenAPIV3Schema
	if !slices.Contains(schema.Required, "spec") {
		t.Errorf("top-level required fields = %v, want spec", schema.Required)
	}
	if !hasValidationRule(schema.XValidations, "self.metadata.name == 'default-aipipelines'") {
		t.Errorf("singleton validation is missing: %+v", schema.XValidations)
	}

	spec := schema.Properties["spec"]
	argo := spec.Properties["argoWorkflowsControllers"]
	managementState := argo.Properties["managementState"]
	if got := decodeJSONString(t, managementState.Default); got != string(common.Managed) {
		t.Errorf("managementState default = %q, want %q", got, common.Managed)
	}
	if got := decodeJSONStrings(t, managementState.Enum); !slices.Equal(got, []string{string(common.Managed), string(common.Removed)}) {
		t.Errorf("managementState enum = %v", got)
	}

	status := schema.Properties["status"]
	for _, field := range []string{"phase", "observedGeneration", "conditions", "releases"} {
		if _, found := status.Properties[field]; !found {
			t.Errorf("status.%s is missing", field)
		}
	}
	if status.Properties["conditions"].XListType == nil || *status.Properties["conditions"].XListType != "atomic" {
		t.Error("status.conditions is not an atomic list")
	}
	if status.Properties["releases"].XListType == nil || *status.Properties["releases"].XListType != "map" {
		t.Error("status.releases is not a map list")
	}
	if got := status.Properties["releases"].XListMapKeys; !slices.Equal(got, []string{"name"}) {
		t.Errorf("status.releases map keys = %v, want [name]", got)
	}
}

func hasValidationRule(rules apiextensionsv1.ValidationRules, want string) bool {
	for _, rule := range rules {
		if rule.Rule == want {
			return true
		}
	}
	return false
}

func decodeJSONString(t *testing.T, value *apiextensionsv1.JSON) string {
	t.Helper()
	if value == nil {
		t.Fatal("expected JSON value, got nil")
	}

	var result string
	if err := json.Unmarshal(value.Raw, &result); err != nil {
		t.Fatalf("decoding JSON string %s: %v", value.Raw, err)
	}
	return result
}

func decodeJSONStrings(t *testing.T, values []apiextensionsv1.JSON) []string {
	t.Helper()
	result := make([]string, 0, len(values))
	for i := range values {
		result = append(result, decodeJSONString(t, &values[i]))
	}
	return result
}
