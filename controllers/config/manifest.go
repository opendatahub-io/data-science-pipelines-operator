package config

import (
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Manifest(cl client.Client, templatePath string, context interface{}) (mf.Manifest, error) {
	m, err := mf.ManifestFrom(PathTemplateSource(templatePath, context))
	if err != nil {
		return mf.Manifest{}, err
	}
	m.Client = mfc.NewClient(cl)

	return m, err
}
