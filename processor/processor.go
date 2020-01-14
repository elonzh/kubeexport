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
	annotations := accessor.GetAnnotations()
	delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	delete(annotations, "deployment.kubernetes.io/revision")
	delete(annotations, "kubernetes.io/change-cause")
	accessor.SetAnnotations(annotations)

	var rv runtime.Unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	rv = &unstructured.Unstructured{Object: unstructuredObj}
	content := rv.UnstructuredContent()
	delete(content, "Status")
	rv.SetUnstructuredContent(content)
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
