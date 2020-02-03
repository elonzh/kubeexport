package processor

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	batchv1api "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type Processor interface {
	Process(obj runtime.Unstructured) (runtime.Unstructured, error)
}

type SkipOwnerReferencesProcessor struct{}

func (p *SkipOwnerReferencesProcessor) Process(obj runtime.Unstructured) (runtime.Unstructured, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	// https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/
	if len(accessor.GetOwnerReferences()) > 0 {
		logrus.WithFields(logrus.Fields{
			"Name": accessor.GetName(),
		}).Debugln("Object has OwnerReferences, skip")
		return nil, nil
	}
	return obj, nil
}

func deleteStringFields(m map[string]string, fields ...string) map[string]string {
	for _, f := range fields {
		delete(m, f)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func deleteInterfaceFields(m map[string]interface{}, fields ...string) map[string]interface{} {
	for _, f := range fields {
		delete(m, f)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

type CommonProcessor struct{}

func (p *CommonProcessor) Process(obj runtime.Unstructured) (runtime.Unstructured, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	accessor.SetGeneration(0)
	accessor.SetResourceVersion("")
	accessor.SetSelfLink("")
	accessor.SetUID("")
	accessor.SetAnnotations(deleteStringFields(
		accessor.GetAnnotations(),
		"kubectl.kubernetes.io/last-applied-configuration",
		"deployment.kubernetes.io/revision",
		"kubernetes.io/change-cause",
	))

	var rv runtime.Unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	rv = &unstructured.Unstructured{Object: unstructuredObj}

	rv.SetUnstructuredContent(deleteInterfaceFields(rv.UnstructuredContent(), "status"))
	return rv, nil
}

type JobProcessor struct{}

func (h *JobProcessor) Process(obj runtime.Unstructured) (runtime.Unstructured, error) {
	if obj.GetObjectKind().GroupVersionKind().Kind != "Job" {
		return obj, nil
	}
	job := new(batchv1api.Job)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), job); err != nil {
		return nil, errors.WithStack(err)
	}

	job.Spec.Selector.MatchLabels = deleteStringFields(job.Spec.Selector.MatchLabels, "controller-uid")
	job.Spec.Template.ObjectMeta.Labels = deleteStringFields(job.Spec.Template.ObjectMeta.Labels, "controller-uid")

	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(job)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	res = deleteInterfaceFields(res, "status")
	return &unstructured.Unstructured{Object: res}, nil
}
