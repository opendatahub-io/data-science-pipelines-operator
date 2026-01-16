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

	mf "github.com/manifestival/manifestival"
	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"

	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"strings"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
		if k8serrors.IsNotFound(err) {
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
		if k8serrors.IsNotFound(err) {
			return false, nil, nil
		} else {
			return false, nil, err
		}
	}
	return true, route, nil
}

func GetSecret(ctx context.Context, secretName, ns string, client client.Client) (*v1.Secret, error) {
	secret := &v1.Secret{}
	namespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: ns,
	}
	err := client.Get(ctx, namespacedName, secret)
	if err != nil {
		return &v1.Secret{}, err
	}
	return secret, nil
}

// DSPAWithSupportedDSPVersion returns True if dspa's dsp version is supported, return False otherwise.
// Note that the procedure verifies the DSPA's .spec.dspVerson field. Not to be confused with the apiversion.
func DSPAWithSupportedDSPVersion(dspa *dspav1.DataSciencePipelinesApplication) bool {
	isSupported := false
	for _, supportedVersion := range config.GetSupportedDSPAVersions() {
		if dspa.Spec.DSPVersion == supportedVersion {
			isSupported = true
		}
	}
	return isSupported
}

// HasSupportedDSPVersionLabel returns true if labels (representing labels for a k8s resource)
// has the DSPVersionk8sLabel label AND the value belongs to a supported DSP Version
func HasSupportedDSPVersionLabel(labels map[string]string) bool {
	version, ok := labels[config.DSPVersionk8sLabel]
	if !ok {
		return false
	}
	for _, supportedVersion := range config.GetSupportedDSPAVersions() {
		if version == supportedVersion {
			return true
		}
	}
	return false
}

func AddLabelTransformer(labelKey, labelValue string) mf.Transformer {
	return func(mfObj *unstructured.Unstructured) error {
		// Get existing labels
		labels := mfObj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		// Add or override the label
		labels[labelKey] = labelValue
		// Set the labels back on the object
		mfObj.SetLabels(labels)
		return nil
	}
}

func AddDeploymentPodLabelTransformer(labelKey, labelValue string) mf.Transformer {
	return func(mfObj *unstructured.Unstructured) error {
		// Check if the resource is a Deployment
		if mfObj.GetKind() == "Deployment" {
			// Get the spec.template.metadata.labels (which are the Pod labels)
			podLabels, found, err := unstructured.NestedStringMap(mfObj.Object, "spec", "template", "metadata", "labels")
			if err != nil {
				return err
			}
			if !found {
				podLabels = make(map[string]string)
			}
			// Add or override the pod label
			podLabels[labelKey] = labelValue
			// Set the updated labels back to spec.template.metadata.labels
			err = unstructured.SetNestedStringMap(mfObj.Object, podLabels, "spec", "template", "metadata", "labels")
			if err != nil {
				return fmt.Errorf("failed to set pod labels: %w", err)
			}
		}
		return nil
	}
}

// IsTransientStartup returns true for common transient rollout errors such as
// timeouts, connection refused, DNS not ready.
func IsTransientStartup(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	transientTokens := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"context deadline exceeded",
		"temporary failure",
		"503 service unavailable",
		"could not connect",
	}
	for _, t := range transientTokens {
		if strings.Contains(msg, t) {
			return true
		}
	}
	return false
}
