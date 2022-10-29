package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/karthick18/goyang/pkg/yang"
)

var (
	ErrNodeNotFound = errors.New("node not found in yang model")
	ErrMultipleKeys = errors.New("multiple keys for yang entry")
	ErrInvalidArgs  = errors.New("invalid args")
)

func getShortNames(camelCasedName string) []string {
	if len(camelCasedName) <= 5 {
		sn := pluralize(camelCasedName)
		if noConfig && sn[0] != 'q' {
			sn = "q" + sn
		}

		return []string{sn}
	}

	sn := []byte{}
	var shortName string

	for _, b := range camelCasedName {
		if b >= 'A' && b <= 'Z' {
			sn = append(sn, byte(b))
		}
	}

	shortName = strings.ToLower(string(sn))
	ls := len(shortName)

	if ls == 0 {
		return []string{string(camelCasedName[0]) + "s"}
	}

	if noConfig && shortName[0] != 'q' {
		shortName = "q" + shortName
		ls += 1
	}

	if ls == 1 {
		// take first 3 bytes of crd name in case we have a single byte
		shortName = strings.ToLower(camelCasedName)[:3]

		return []string{shortName}
	}

	if shortName[ls-1] == 's' {
		return []string{shortName}
	}

	return []string{shortName, shortName + "s"}
}

func pluralize(s string) string {
	if len(s) == 0 {
		return ""
	}

	p := strings.ToLower(s)
	if p[len(p)-1] == 's' {
		return p
	}

	return p + "s"
}

func getKeyForEntry(e *yang.Entry) (string, error) {
	if e.Key == "" {
		if noConfig {
			return "name", nil
		}

		return "internal", nil
	}

	keys := strings.Split(strings.TrimSpace(e.Key), " ")
	if len(keys) > 1 {
		return "", fmt.Errorf("%w: multiple keys for node %s. specify key option with filename", ErrMultipleKeys, e.Name)
	}

	return keys[0], nil
}

func getRootInstanceEntry(entry *yang.Entry) (string, string, *yang.Entry, error) {
	if len(entry.Dir) > 1 {
		return "", "", nil, fmt.Errorf("%w: cannot derive root/instance node as there are multiple root nodes", ErrNodeNotFound)
	}

	for name, node := range entry.Dir {
		if node.IsList() {
			return name, name, node, nil
		}

		if node.IsContainer() {
			if len(node.Dir) == 0 {
				return name, name, node, nil
			}

			root := name
			instance := ""
			var instanceEntry *yang.Entry

			for n, d := range node.Dir {
				if d.IsList() {
					if instance == "" {
						instance = n
						instanceEntry = d
					} else {
						return "", "", nil, fmt.Errorf("%w:warning multiple list nodes: %s, instance: %s\n", ErrNodeNotFound, n, instance)
					}
				}
			}

			if instance == "" {
				instance = root
				instanceEntry = node
			}

			return root, instance, instanceEntry, nil
		}
	}

	return "", "", nil, fmt.Errorf("%w: could not find root/instance node", ErrNodeNotFound)
}

func dumpRootInstanceEntry(entry *yang.Entry) {
	for name, node := range entry.Dir {
		if node.IsList() {
			fmt.Println("list", "root", name, "instance", name)
			continue
		}

		if node.IsContainer() {
			if len(node.Dir) == 0 {
				fmt.Println("single-container", "root", name, "instance", name)

				continue
			}

			root := name
			instance := ""

			for n, d := range node.Dir {
				if d.IsList() {
					if instance == "" {
						instance = n
					} else {
						instance += "," + n
					}
				}
			}

			if instance == "" {
				instance = root
			}

			fmt.Println("container", "root", root, "instance", instance)
		}
	}
}

func getOutputDirectory() string {
	if outputDirectory == "" {
		path, err := os.Getwd()
		if err != nil {
			panic("error getting current directory:" + err.Error())
		}

		outputDirectory = path
	} else {
		err := os.MkdirAll(outputDirectory, os.ModePerm)
		if err != nil && !os.IsExist(err) {
			panic("error mkdirall:" + err.Error())
		}
	}

	return outputDirectory
}

func validateArgs(files []string) error {
	if len(files) > 1 {
		if rootNodeModel != "" {
			return fmt.Errorf("%w: root node specified with multiple files. they should be part of each filename separated by comma", ErrInvalidArgs)
		}

		if instanceNodeModel != "" {
			return fmt.Errorf("%w: instance node specified with multiple files. they should be part of each filename separated by comma", ErrInvalidArgs)
		}

		if crdName != "" {
			return fmt.Errorf("%w: crd name specified with multiple files. they should be part of each filename separated by comma", ErrInvalidArgs)
		}
	}

	return nil
}

// returns a relative path from base including the base
func getRelativePathWithBase(base, path string) string {
	paths := strings.Split(path, "/")
	dirs := []string{}
	for _, d := range paths {
		if d == "" {
			continue
		}
		dirs = append(dirs, d)
	}

	if len(dirs) == 0 {
		return base
	}

	baseIndex := -1
	for i, d := range dirs {
		if d == base {
			baseIndex = i + 1
			break
		}
	}

	if baseIndex == -1 || baseIndex >= len(dirs) {
		return base
	}

	res := []string{base}
	res = append(res, dirs[baseIndex:]...)

	return strings.Join(res, "/")
}
