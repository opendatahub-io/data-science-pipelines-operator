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
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/template"

	mf "github.com/manifestival/manifestival"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PathPrefix is the file system path which template paths will be prefixed with.
// Default is no prefix, which causes paths to be read relative to process working dir
var PathPrefix string

// PathTemplateSource A templating source read from a file
func PathTemplateSource(path string, context interface{}) (mf.Source, error) {
	f, err := os.Open(prefixedPath(path))
	if err != nil {
		return mf.Slice([]unstructured.Unstructured{}), err
	}

	tmplSrc, err := templateSource(f, context)
	if err != nil {
		return mf.Slice([]unstructured.Unstructured{}), err
	}

	return tmplSrc, nil
}

func prefixedPath(p string) string {
	if PathPrefix != "" {
		return PathPrefix + "/" + p
	}
	return p
}

// A templating manifest source
func templateSource(r io.Reader, context interface{}) (mf.Source, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return mf.Slice([]unstructured.Unstructured{}), err
	}
	t, err := template.New("manifestTemplateDSP").Funcs(template.FuncMap{
		"yamlString": yamlString,
	}).Parse(string(b))
	if err != nil {
		return mf.Slice([]unstructured.Unstructured{}), err
	}
	var b2 bytes.Buffer
	err = t.Execute(&b2, context)
	if err != nil {
		return mf.Slice([]unstructured.Unstructured{}), err
	}
	return mf.Reader(&b2), nil
}

func yamlString(value interface{}) (string, error) {
	switch value := value.(type) {
	case string:
		return strconv.Quote(value), nil
	case *string:
		if value == nil {
			return "", fmt.Errorf("cannot encode a nil string pointer")
		}
		return strconv.Quote(*value), nil
	default:
		return "", fmt.Errorf("cannot encode %T as a YAML string", value)
	}
}
