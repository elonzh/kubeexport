package flags

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type OutPutFlags struct {
	Dir   string
	Force bool
}

func (o *OutPutFlags) Join(elem ...string) string {
	return filepath.Join(append([]string{o.Dir}, elem...)...)
}

func (o *OutPutFlags) EnsureDir(dir string) string {
	path := o.Join(dir)
	err := os.MkdirAll(path, os.ModeDir|os.ModePerm)
	if err != nil && !os.IsExist(err) {
		logrus.WithError(err).Fatalln()
	}
	return path
}

func (o *OutPutFlags) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.Dir, "dir", "output", "output directory")
	cmd.Flags().BoolVar(&o.Force, "force", false, "force delete directory when directory is not empty")
}

func (o *OutPutFlags) ValidateDir() {
	fileInfo, err := os.Stat(o.Dir)
	if err != nil && !os.IsNotExist(err) {
		logrus.WithError(err).Fatalln("get dir stat failed")
	}
	if fileInfo == nil {
		return
	}
	if o.Force {
		err := os.RemoveAll(o.Dir)
		if err != nil && !os.IsNotExist(err) {
			logrus.WithError(err).Fatalln()
		}
		return
	}

	if !fileInfo.IsDir() {
		logrus.WithError(errors.Errorf("%s is not a dir", o.Dir)).Fatalln()
	}
	f, err := os.Open(o.Dir)
	if err != nil {
		logrus.WithError(err).Fatalln()
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		logrus.WithError(err).Fatalln()
	}
	if len(names) > 0 {
		logrus.WithError(errors.Errorf("%s is not empty", o.Dir)).Fatalln()
	}
	err = f.Close()
	if err != nil {
		logrus.WithError(err).Fatalln()
	}
}
