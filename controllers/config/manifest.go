package config

import (
	mfc "github.com/manifestival/controller-runtime-client"
	mf "github.com/manifestival/manifestival"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func Apply(cl client.Client, owner metav1.Object, templatePath string, context interface{}, fns ...mf.Transformer) error {
	m, err := mf.ManifestFrom(PathTemplateSource(templatePath, context))
	if err != nil {
		return err
	}
	m.Client = mfc.NewClient(cl)

	if owner != nil {
		asMfOwner := owner.(mf.Owner)
		fns = append(fns, mf.InjectOwner(asMfOwner))
		fns = append(fns, mf.InjectNamespace(asMfOwner.GetNamespace()))
	}
	m, err = m.Transform(fns...)
	if err != nil {
		return err
	}
	err = m.Apply()
	if err != nil {
		return err
	}

	return nil
}

func Delete(cl client.Client, owner metav1.Object, templatePath string, context interface{}, namespace string, fns ...mf.Transformer) error {
	m, err := mf.ManifestFrom(PathTemplateSource(templatePath, context))
	if err != nil {
		return err
	}
	m.Client = mfc.NewClient(cl)

	if owner != nil {
		asMfOwner := owner.(mf.Owner)
		fns = append(fns, mf.InjectOwner(asMfOwner))
		fns = append(fns, mf.InjectNamespace(namespace))
	}
	m, err = m.Transform(fns...)
	if err != nil {
		return err
	}
	err = m.Delete()
	if err != nil {
		return err
	}

	return nil
}
