package engine

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

	"github.com/earlzo/kubeexport"
	"github.com/earlzo/kubeexport/flags"
	"github.com/earlzo/kubeexport/hooks"
)

func IsListable(r *metav1.APIResource) bool {
	for _, verb := range r.Verbs {
		if verb == "list" {
			return true
		}
	}
	return false
}

type Engine struct {
	configFlags           *genericclioptions.ConfigFlags
	printFlags            *genericclioptions.PrintFlags
	outputFlags           *flags.OutPutFlags
	printer               printers.ResourcePrinter
	excludedResourceTypes []string

	hks []hooks.Hook
}

func NewEngine() *Engine {
	engine := Engine{
		configFlags:           genericclioptions.NewConfigFlags(false),
		printFlags:            genericclioptions.NewPrintFlags(""),
		outputFlags:           &flags.OutPutFlags{},
		excludedResourceTypes: []string{"endpoints", "events"},
		hks: []hooks.Hook{
			&hooks.AnyHook{},
			&hooks.JobHook{},
		},
	}
	engine.printFlags.WithDefaultOutput("yaml")
	return &engine
}

func (e *Engine) AddFlags(cmd *cobra.Command) {
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

func (e *Engine) LoadResourceTypes() []string {
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

func (e *Engine) Run(resourceTypes ...string) {
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
	currentContext := rawConfig.Contexts[contextName]
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

func (e *Engine) VisitObject(info *resource.Info, err error) error {
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

func (e *Engine) exportObject(info *resource.Info, obj runtime.Object, pathFunc kubeexport.ObjectPathFunc) error {
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

	var rv runtime.Unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		logrus.WithError(err).Fatalln(err)
	}
	rv = &unstructured.Unstructured{Object: unstructuredObj}
	for _, hk := range e.hks {
		rv, err = hk.Execute(rv)
		if err != nil {
			logrus.WithError(err).Fatalln(err)
		}
	}
	return e.printer.PrintObj(rv, file)
}
