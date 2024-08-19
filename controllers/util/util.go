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
	"fmt"
	"os"
	"path/filepath"

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"

	"context"
	"crypto/x509"
	"net/url"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func GetTemplatesInDir(templatesDirectory, componentSubdirectory string) ([]string, error) {
	files, err := os.ReadDir(templatesDirectory + componentSubdirectory)
	if err != nil {
		return nil, err
	}

	var templates []string
	for _, f := range files {
		if !f.IsDir() {
			templates = append(templates, filepath.Join(componentSubdirectory, f.Name()))
		}
	}
	return templates, nil
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

func GetConfigMap(ctx context.Context, cfgName, ns string, client client.Client) (*v1.ConfigMap, error) {
	cfgMap := &v1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      cfgName,
		Namespace: ns,
	}
	err := client.Get(ctx, namespacedName, cfgMap)
	if err != nil {
		return &v1.ConfigMap{}, err
	}
	return cfgMap, nil
}

// GetConfigMapValue fetches the value for the provided configmap mapped to a given key
func GetConfigMapValue(cfgKey string, cfgMap *v1.ConfigMap) string {
	if val, ok := cfgMap.Data[cfgKey]; ok {
		return val
	} else {
		return ""
	}
}

// GetConfigMapValues fetches the value for the provided configmap mapped to a given key
func GetConfigMapValues(cfgMap *v1.ConfigMap) []string {
	var values []string
	for _, val := range cfgMap.Data {
		values = append(values, val)
	}
	return values
}

func GetSystemCerts() ([]byte, error) {
	sslCertFile := os.Getenv(config.DefaultSystemSSLCertFile)
	if sslCertFile == "" {
		sslCertFile = config.DefaultSystemSSLCertFilePath
	}
	data, err := os.ReadFile(sslCertFile)
	if err != nil {
		return []byte{}, err
	}
	return data, err
}

func GetServiceHostname(ctx context.Context, svcName, ns string, client client.Client) (string, error) {
	serviceHostname := ""
	isAvailable, service, err := GetServiceIfAvailable(ctx, svcName, ns, client)
	if err != nil {
		return "", err
	}

	if isAvailable {
		// Loop over all Service ports, if a secured port is found
		// set port and scheme to its secured ones and exit the loop
		servicePort := ""
		scheme := "http"
		for _, port := range service.Spec.Ports {
			servicePort = fmt.Sprintf("%d", port.Port)
			if servicePort == "8443" || servicePort == "443" {
				// If a secured port is found, just set scheme to 'https://' and exit the loop
				scheme = "https"
				break
			}
		}
		serviceHostname = scheme + "://" + service.Name + "." + service.Namespace + ".svc.cluster.local:" + servicePort
	}
	return serviceHostname, nil
}

func GetRouteHostname(ctx context.Context, routeName, ns string, client client.Client) (string, error) {
	routeHostname := ""
	isAvailable, route, err := GetRouteIfAvailable(ctx, routeName, ns, client)
	if err != nil {
		return "", err
	}

	if isAvailable {
		scheme := "http"
		if route.Spec.TLS != nil {
			scheme = "https"
		}

		routeHostname = scheme + "://" + route.Spec.Host
	}
	return routeHostname, nil
}

func GetServiceIfAvailable(ctx context.Context, svcName, ns string, client client.Client) (bool, *v1.Service, error) {
	service := &v1.Service{}
	namespacedNamed := types.NamespacedName{Name: svcName, Namespace: ns}
	err := client.Get(ctx, namespacedNamed, service)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	}
	return true, service, nil
}

func GetRouteIfAvailable(ctx context.Context, routeName, ns string, client client.Client) (bool, *routev1.Route, error) {
	route := &routev1.Route{}
	namespacedNamed := types.NamespacedName{Name: routeName, Namespace: ns}
	err := client.Get(ctx, namespacedNamed, route)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	}
	return true, route, nil
}
