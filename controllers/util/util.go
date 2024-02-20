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

package util

import (
	"context"
	"crypto/x509"
	"fmt"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetConditionByType returns condition of type condType if it exists in conditions, otherwise
// return empty condition struct.
func GetConditionByType(condType string, conditions []metav1.Condition) metav1.Condition {
	for _, c := range conditions {
		if c.Type == condType {
			return c
		}
	}
	return metav1.Condition{}
}

func GetDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for _, c := range status.Conditions {
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

func BoolPointer(b bool) *bool {
	return &b
}

// IsX509UnknownAuthorityError checks whether an error is of type x509.UnknownAuthorityError.
func IsX509UnknownAuthorityError(err error) bool {
	urlErr, ok := err.(*url.Error)
	if !ok {
		return false
	}
	_, ok = urlErr.Err.(x509.UnknownAuthorityError)
	return ok
}

// GetConfigMapValue fetches the value for the provided configmap mapped to a given key
func GetConfigMapValue(ctx context.Context, cfgKey, cfgName, ns string, client client.Client, log logr.Logger) (error, string) {
	cfgMap := &v1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      cfgName,
		Namespace: ns,
	}
	err := client.Get(ctx, namespacedName, cfgMap)
	if err != nil {
		return err, ""
	}
	if val, ok := cfgMap.Data[cfgKey]; ok {
		return nil, val
	} else {
		return fmt.Errorf("ConfigMap %s sdoes not contain expected key %s", cfgName, cfgKey), ""
	}
}
