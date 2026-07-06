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

package config

import (
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Manifest(cl client.Client, templatePath string, context interface{}) (mf.Manifest, error) {
	pathTmplSrc, err := PathTemplateSource(templatePath, context)
	if err != nil {
		return mf.Manifest{}, err
	}

	m, err := mf.ManifestFrom(pathTmplSrc)
	if err != nil {
		return mf.Manifest{}, err
	}
	m.Client = mfc.NewClient(cl)

	return m, err
}
