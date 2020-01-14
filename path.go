package kubeexport

import (
	"path/filepath"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ObjectPathFunc func(mapping *meta.RESTMapping, obj runtime.Object) (err error, dir, file string)

func DefaultObjectPath(mapping *meta.RESTMapping, obj runtime.Object) (err error, dir, file string) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err, "", ""
	}
	filename := accessor.GetName()
	labels := accessor.GetLabels()
	appName := labels["app"]
	namespace := accessor.GetNamespace()
	if namespace == "" {
		dir = filepath.Join(mapping.Resource.Resource)
	} else if appName == "" {
		dir = filepath.Join(mapping.Resource.Resource)
	} else {
		dir = filepath.Join("projects", appName, mapping.Resource.Resource)
	}
	return nil, dir, filename
}

func DefaultTemplatePath(mapping *meta.RESTMapping, obj runtime.Object) (err error, dir, file string) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err, "", ""
	}
	filename := accessor.GetName()
	path := &strings.Builder{}
	t := template.Must(template.New("path_template").Parse(`{{-.Mapping.Resource.Resource-}}`))
	err = t.Execute(path, struct {
		Mapping *meta.RESTMapping
		Object  *v1.Object
	}{mapping, &accessor})
	if err != nil {
		return err, "", ""
	}
	return nil, path.String(), filename
}
