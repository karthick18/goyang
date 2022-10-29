package main

import (
	"fmt"
	"os"
	"strings"
)

type fileOption struct {
	name, opts string
}

type CrdOptions struct {
	Root, Instance   string
	Name             string
	Group            string
	Key              string
	Config           bool
	SkipReconcile    bool
	Augmentor        string
	ModuleSearchPath string
}

const (
	DefaultGroupName = "netconf.ciena.com"
)

func getDefaultOptions() *CrdOptions {
	groupName := crdGroup
	if groupName == "" {
		groupName = DefaultGroupName
	}

	return &CrdOptions{
		Root:             rootNodeModel,
		Instance:         instanceNodeModel,
		Config:           !noConfig,
		Name:             crdName,
		Group:            groupName,
		ModuleSearchPath: moduleSearchPath,
	}
}

func parseOptions(options string) *CrdOptions {
	crdOption := getDefaultOptions()
	if options == "" {
		return crdOption
	}

	parts := strings.Split(options, ",")

	kvstore := map[string]*string{
		"root":      &crdOption.Root,
		"instance":  &crdOption.Instance,
		"name":      &crdOption.Name,
		"key":       &crdOption.Key,
		"augmentor": &crdOption.Augmentor,
		"group":     &crdOption.Group,
	}

	for _, part := range parts {
		kv := strings.Split(part, "=")
		if len(kv) != 2 {
			fmt.Fprintf(os.Stderr, "ignoring invalid option: %s\n", part)

			continue
		}

		vref, ok := kvstore[kv[0]]
		if !ok {
			fmt.Fprintf(os.Stderr, "ignoring invalid option: %s\n", kv[0])

			continue
		}

		*vref = kv[1]
	}

	return crdOption
}

func extractFileOptions(files []string) []FileOption {
	return newFileOptions(files)
}

func newFileOptions(files []string) []FileOption {
	fileOptions := make([]FileOption, len(files))
	for i, file := range files {
		fileOptions[i] = newFileOption(file)
	}

	return fileOptions
}

func newFileOption(filename string) *fileOption {
	parts := strings.Split(filename, ",")
	name := parts[0]
	opts := ""
	if len(parts) > 1 {
		opts = strings.Join(parts[1:], ",")
	}

	return &fileOption{name: name, opts: opts}
}

func (fo *fileOption) Name() string {
	return fo.name
}

func (fo *fileOption) Options() string {
	return fo.opts
}
