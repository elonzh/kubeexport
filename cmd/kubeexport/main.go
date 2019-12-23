package main

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/earlzo/kubeexport/cmd"
	"github.com/earlzo/kubeexport/engine"
)

func NewCommand() *cobra.Command {
	e := engine.NewEngine()
	var rootCmd = &cobra.Command{
		Use:     "kubeexport [flags] resourceTypes...",
		Short:   "Export resources from your kubernetes cluster",
		Long:    "Export resources from your kubernetes cluster",
		Example: `
	# export all exportable resources into directory "exported_resources"
	kubeexport -o "exported_resources" 
		
	# export "deployments", "jobs" resources into default output directory
	kubeexport deployments jobs`,
		Run: func(cmd *cobra.Command, args []string) {
			e.Run(args...)
		},
	}
	e.AddFlags(rootCmd)
	cmd.NormalizeAll(rootCmd)
	return rootCmd
}

func Execute() {
	rootCmd := NewCommand()
	if err := rootCmd.Execute(); err != nil {
		logrus.WithError(err).Fatalln()
	}
}

func main() {
	Execute()
}
