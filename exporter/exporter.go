package exporter

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/cli-runtime/pkg/resource"

	"github.com/elonzh/kubeexport"
	"github.com/elonzh/kubeexport/flags"
	"github.com/elonzh/kubeexport/processor"
)

func IsListable(r *metav1.APIResource) bool {
	for _, verb := range r.Verbs {
		if verb == "list" {
			return true
		}
	}
	return false
}

type Exporter struct {
	configFlags           *genericclioptions.ConfigFlags
	printFlags            *genericclioptions.PrintFlags
	outputFlags           *flags.OutPutFlags
	printer               printers.ResourcePrinter
	excludedResourceTypes []string

	processors []processor.Processor
}

func NewExporter() *Exporter {
	engine := Exporter{
		configFlags:           genericclioptions.NewConfigFlags(false),
		printFlags:            genericclioptions.NewPrintFlags(""),
		outputFlags:           &flags.OutPutFlags{},
		excludedResourceTypes: []string{"endpoints", "events"},
		processors: []processor.Processor{
			&processor.SkipOwnerReferencesProcessor{},
			&processor.CommonProcessor{},
			&processor.JobProcessor{},
		},
	}
	engine.printFlags.WithDefaultOutput("yaml")
	return &engine
}

func (e *Exporter) AddFlags(cmd *cobra.Command) {
	e.configFlags.AddFlags(cmd.PersistentFlags())
	e.printFlags.AddFlags(cmd)
	e.outputFlags.AddFlags(cmd)
	cmd.Flags().StringSliceVar(&e.excludedResourceTypes, "exclude", e.excludedResourceTypes, "excluded resource types")
}

func Contains(slice []string, elem string) bool {
	for _, v := range slice {
		if v == elem {
			return true
		}
	}
	return false
}

func (e *Exporter) LoadResourceTypes() []string {
	var resourceTypes = make([]string, 0, 10)
	discoveryClient, err := e.configFlags.ToDiscoveryClient()
	if err != nil {
		logrus.WithError(err).Fatalln()
	}
	apiResourceLists, err := discoveryClient.ServerPreferredNamespacedResources()
	if err != nil {
		logrus.WithError(err).Fatalln()
	}
	for _, apiResourceList := range apiResourceLists {
		for _, apiResource := range apiResourceList.APIResources {
			// TODO: Configurable resource type filter
			if apiResource.Namespaced && IsListable(&apiResource) && !Contains(e.excludedResourceTypes, apiResource.Name) {
				resourceTypes = append(resourceTypes, apiResource.Name)
			}
		}
	}
	return resourceTypes
}

func (e *Exporter) Run(resourceTypes ...string) {
	e.outputFlags.ValidateDir()
	var err error

	e.printer, err = e.printFlags.ToPrinter()
	if err != nil {
		logrus.WithError(err).Fatalln()
	}

	configLoader := e.configFlags.ToRawKubeConfigLoader()
	rawConfig, err := configLoader.RawConfig()
	if err != nil {
		logrus.WithError(err).Fatalln()
	}
	contextName := *e.configFlags.Context
	if contextName == "" {
		contextName = rawConfig.CurrentContext
	}
	currentContext, ok := rawConfig.Contexts[contextName]
	if !ok {
		logrus.WithField("ContextName", contextName).Fatalln("no such context")
	}
	clusterName := *e.configFlags.ClusterName
	if clusterName == "" {
		clusterName = currentContext.Cluster
	}
	namespace := *e.configFlags.Namespace
	if namespace == "" {
		namespace = currentContext.Namespace
	}

	if len(resourceTypes) == 0 {
		resourceTypes = e.LoadResourceTypes()
	}

	logrus.WithFields(logrus.Fields{
		"KubeConfig":    currentContext.LocationOfOrigin,
		"ClusterName":   clusterName,
		"Namespace":     namespace,
		"resourceTypes": resourceTypes,
	}).Infoln("start dump resources")

	builder := resource.NewBuilder(e.configFlags).
		Unstructured().
		DefaultNamespace().
		ContinueOnError().
		Latest().
		ContinueOnError().
		SelectAllParam(true)
	result := builder.
		NamespaceParam(namespace).
		ResourceTypes(resourceTypes...).
		Do()

	err = result.Visit(e.VisitObject)
	if err != nil {
		logrus.WithError(err).Fatalln("error when visit object")
	}

}

func (e *Exporter) VisitObject(info *resource.Info, err error) error {
	if err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{
		"Scope":            info.ResourceMapping().Scope.Name(),
		"Resource":         info.ResourceMapping().Resource,
		"GroupVersionKind": info.ResourceMapping().GroupVersionKind,
	}).Infof("start visit object")
	objList, err := meta.ExtractList(info.Object)
	if err != nil {
		return err
	}
	for _, obj := range objList {
		err = e.exportObject(info, obj, kubeexport.DefaultObjectPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Exporter) exportObject(info *resource.Info, obj runtime.Object, pathFunc kubeexport.ObjectPathFunc) error {
	var rv runtime.Unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		logrus.WithError(err).Fatalln(err)
	}
	rv = &unstructured.Unstructured{Object: unstructuredObj}
	for i := 0; i < len(e.processors) && rv != nil; i++ {
		rv, err = e.processors[i].Process(rv)
		if err != nil {
			logrus.WithError(err).Fatalln(err)
		}
	}
	if rv == nil {
		return nil
	}

	err, dir, filename := pathFunc(info.Mapping, obj)
	if err != nil || filename == "" {
		return err
	}
	dir = e.outputFlags.EnsureDir(dir)
	file, err := os.Create(filepath.Join(dir, filename+"."+*e.printFlags.OutputFormat))
	if err != nil {
		logrus.WithError(err).Fatalln(err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			logrus.WithError(err).WithField("filename", file.Name()).Fatalln("error when closing file")
		}
	}()
	return e.printer.PrintObj(rv, file)
}
