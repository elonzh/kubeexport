package kubeexport

import (
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

type ObjectPathFunc func(mapping *meta.RESTMapping, obj runtime.Object) (err error, dir, file string)

func DefaultObjectPath(mapping *meta.RESTMapping, obj runtime.Object) (err error, dir, file string) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err, "", ""
	}
	filename := accessor.GetName()
	// https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/
	if len(accessor.GetOwnerReferences()) > 0 {
		logrus.WithFields(logrus.Fields{
			"Name": filename,
		}).Infoln("Object has OwnerReferences, skip")
		return err, "", ""
	}
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
