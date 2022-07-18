package yang

import (
	"errors"
	"fmt"
	"path"
	"sort"
)

var (
	ErrInvalidNode = errors.New("invalid node")
)

func ModuleToYangEntry(module string, paths []string, containerNode, leafNode string) (*Entry, string, error) {
	ms := NewModules()
	ms.ParseOptions.IgnoreSubmoduleCircularDependencies = true
	ms.ParseOptions.IgnoreModuleResolveErrors = true

	for _, path := range paths {
		expanded, err := PathsWithModules(path)
		if err != nil {
			continue
		}
		ms.AddPath(expanded...)
	}

	if err := ms.Read(module); err != nil {
		return nil, "", err
	}

	// Process the read files, exiting if any errors were found.
	if errs := ms.Process(); len(errs) > 0 {
		return nil, "", errs[0]
	}

	// Keep track of the top level modules we read in.
	// Those are the only modules we want to print below.
	mods := map[string]*Module{}
	var names []string

	for _, m := range ms.Modules {
		if mods[m.Name] == nil {
			mods[m.Name] = m
			names = append(names, m.Name)
		}
	}

	sort.Strings(names)
	entries := make([]*Entry, len(names))
	for x, n := range names {
		entries[x] = ToEntry(mods[n])
	}

	return findEntry(entries, module, containerNode, leafNode)
}

func findEntry(entries []*Entry, module, containerNode, leafNode string) (*Entry, string, error) {
	base := path.Base(module)
	moduleBaseName := base[:len(base)-len(path.Ext(base))]

	for _, e := range entries {
		if e.Name != moduleBaseName {
			continue
		}

		for name, node := range e.Dir {
			if name == containerNode && node.Dir != nil {

				switch {
				case node.IsContainer(), node.IsList():
				default:
					return nil, "", fmt.Errorf("%w: node: %s, module %s is not a container/list node",
						ErrInvalidNode, containerNode, module)
				}

				if containerNode == leafNode {
					return node, node.Namespace().NName(), nil
				}

				if node.IsList() {
					return nil, "", fmt.Errorf("%w: node: %s is a list node and does not match instance node %s",
						ErrInvalidNode, containerNode, leafNode)
				}

				for childNodeName, childNode := range node.Dir {
					if childNodeName == leafNode {
						if childNode.Dir != nil {
							return childNode, node.Namespace().NName(), nil
						}

						return nil, "", fmt.Errorf("%w: leaf %s, module %s is not a container/list node",
							ErrInvalidNode, leafNode, module)
					}
				}
			}
		}

		break
	}

	return nil, "", fmt.Errorf("unable to find module %s with container node %s, leaf node %s",
		module, containerNode, leafNode)
}
