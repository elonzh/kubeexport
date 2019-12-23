package hooks

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	batchv1api "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type AnyHook struct {
}

func (h *AnyHook) Execute(obj runtime.Unstructured) (runtime.Unstructured, error) {
	objCopy := obj.DeepCopyObject()
	accessor, err := meta.Accessor(objCopy)
	if err != nil {
		return nil, err
	}
	accessor.SetGeneration(0)
	accessor.SetResourceVersion("")
	accessor.SetSelfLink("")
	accessor.SetUID("")
	annotations := accessor.GetAnnotations()
	delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	delete(annotations, "deployment.kubernetes.io/revision")
	delete(annotations, "kubernetes.io/change-cause")
	accessor.SetAnnotations(annotations)

	var rv runtime.Unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(objCopy)
	if err != nil {
		return nil, err
	}
	rv = &unstructured.Unstructured{Object: unstructuredObj}
	content := rv.UnstructuredContent()
	delete(content, "Status")
	rv.SetUnstructuredContent(content)

	return rv, nil
}

type Hook interface {
	Execute(obj runtime.Unstructured) (runtime.Unstructured, error)
}

type JobHook struct {
	logger logrus.FieldLogger
}

func (h *JobHook) Execute(obj runtime.Unstructured) (runtime.Unstructured, error) {
	if obj.GetObjectKind().GroupVersionKind().Kind != "Job"{
		return obj, nil
	}
	job := new(batchv1api.Job)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), job); err != nil {
		return nil, errors.WithStack(err)
	}

	if job.Spec.Selector != nil {
		delete(job.Spec.Selector.MatchLabels, "controller-uid")
	}
	delete(job.Spec.Template.ObjectMeta.Labels, "controller-uid")

	job.Status = batchv1api.JobStatus{}

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(job)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &unstructured.Unstructured{Object: res}, nil
}
