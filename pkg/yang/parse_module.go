package yang

import (
	"fmt"
	"path"
	"sort"
)

func ModuleToYangEntry(module string, paths []string, containerNode, leafNode string) (*Entry, error) {
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
		return nil, err
	}

	// Process the read files, exiting if any errors were found.
	if errs := ms.Process(); len(errs) > 0 {
		return nil, errs[0]
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

func findEntry(entries []*Entry, module, containerNode, leafNode string) (*Entry, error) {
	base := path.Base(module)
	moduleBaseName := base[:len(base)-len(path.Ext(base))]

	for _, e := range entries {
		if e.Name != moduleBaseName {
			continue
		}

		for name, node := range e.Dir {
			if name == containerNode && node.Dir != nil {
				for listNodeName, listNode := range node.Dir {
					if listNodeName == leafNode {
						return listNode, nil
					}
				}
			}
		}

		break
	}

	return nil, fmt.Errorf("unable to find module %s with container node %s, leaf node %s",
		module, containerNode, leafNode)
}
