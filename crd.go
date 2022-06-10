// Copyright 2015 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"text/template"

	"github.com/karthick18/goyang/pkg/indent"
	"github.com/karthick18/goyang/pkg/yang"
	"github.com/pborman/getopt"
)

var (
	rootNode     string
	instanceNode string
	typeMap      = map[string]string{
		"uint8":   "integer",
		"uint16":  "integer",
		"uint32":  "integer",
		"bits":    "integer",
		"string":  "string",
		"boolean": "boolean",
		"leafref": "string",
	}
)

func init() {
	opt := getopt.New()
	opt.StringVarLong(&rootNode, "root-node", 'n', "specify root node for the yang model")
	opt.StringVarLong(&instanceNode, "crd-node", 'c', "specify crd node for the yang model")

	register(&formatter{
		name:  "crd",
		flags: opt,
		f:     doCrd,
		help:  "display in a crd format",
	})
}

func doCrd(w io.Writer, entries []*yang.Entry, files []string) {
	fileBaseNames := make([]string, len(files))
	for i, f := range files {
		base := path.Base(f)
		fileBaseNames[i] = base[:len(base)-len(path.Ext(base))]
	}

	for _, e := range entries {
		matched := false

		for _, f := range fileBaseNames {
			if e.Name == f && e.Dir != nil {
				matched = true
				break
			}
		}

		if !matched {
			continue
		}

		var processEntry *yang.Entry

		for name, node := range e.Dir {
			if name == rootNode && node.Dir != nil {
				for listNodeName, listNode := range node.Dir {
					if listNodeName == instanceNode {
						processEntry = listNode
						break
					}
				}

				if processEntry != nil {
					break
				}
			}
		}

		if processEntry == nil {
			continue
		}

		if processEntry.Dir != nil && processEntry.ListAttr != nil {
			var b strings.Builder
			prefixLen := 0
			fmt.Fprintln(&b, "spec:")
			fmt.Fprintln(&b, "  properties:")

			prefixLen = 4
			var names []string

			for k := range processEntry.Dir {
				names = append(names, k)
			}

			sort.Strings(names)

			for _, name := range names {
				WriteCrd(indent.NewWriter(&b, indent.GetPrefix(prefixLen)), processEntry.Dir[name])
			}

			emitCrdRequired(&b, processEntry, indent.GetPrefix(2))
			fmt.Fprintln(&b, "  type: object")
			//fmt.Fprintln(w, b.String())
			executeTemplate(rootNode, instanceNode, b.String())

			decodeJsonToXml("data.json", "data-xml.json", processEntry, false)
		}
	}
}

type crdConfig struct {
	CrdName    string
	ShortNames []string
	Group      string
}

func executeTemplate(rootNode, crdNode, spec string) {
	files := []string{"crd.tmpl"}

	pluralize := func(s string) string {
		if len(s) == 0 {
			return ""
		}

		p := strings.ToLower(s)
		if p[len(p)-1] == 's' {
			return p
		}

		return p + "s"
	}

	funcMap := template.FuncMap{
		"ToLower":  strings.ToLower,
		"ToUpper":  strings.ToUpper,
		"Title":    strings.Title,
		"ToPlural": pluralize,
		"GetSpec": func(spaces int) string {
			if spec[len(spec)-1] == '\n' {
				spec = spec[:len(spec)-1]
			}

			return indent.String(indent.GetPrefix(spaces), spec)
		},
	}

	crdTemplate, err := template.New("crd.tmpl").Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		panic(err.Error())
	}

	config := crdConfig{
		CrdName: crdNode,
		Group:   "netconf.ciena.com",
	}

	names := []string{rootNode, crdNode}
	shortNames := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))

	for _, name := range names {
		shortName := pluralize(name)

		if _, ok := seen[shortName]; !ok {
			shortNames = append(shortNames, shortName)
			seen[shortName] = struct{}{}
		}
	}

	config.ShortNames = shortNames

	f, err := os.Create(fmt.Sprintf("%s_%s.yaml", config.Group, pluralize(crdNode)))
	if err != nil {
		panic(err.Error())
	}

	err = crdTemplate.Execute(f, config)
	if err != nil {
		panic("error executing template: " + err.Error())
	}
}

// Write writes e, formatted, and all of its children, to w.
func WriteCrd(w io.Writer, e *yang.Entry) {
	if e.RPC != nil {
		return
	}

	prefixLen := 0
	name := yang.CamelCase(e.Name, false)

	fmt.Fprintf(w, "%s:\n", name)
	prefixLen += 2

	switch {
	case e.Dir == nil && e.ListAttr != nil:
		fmt.Fprintln(w, "  items:")
		emitCrdType(w, e, indent.GetPrefix(prefixLen+2))
		fmt.Fprintln(w, "  type: array")
		return
	case e.Dir == nil:
		emitCrdType(w, e, indent.GetPrefix(prefixLen))
		return
	case e.ListAttr != nil:
		fmt.Fprintln(w, "  items:")
		prefixLen += 2
		fmt.Fprintln(w, "    properties:")
		prefixLen += 2
	default:
		fmt.Fprintln(w, "  properties:")
		prefixLen += 2
	}

	var names []string
	for k := range e.Dir {
		names = append(names, k)
	}

	sort.Strings(names)
	for _, k := range names {
		WriteCrd(indent.NewWriter(w, indent.GetPrefix(prefixLen)), e.Dir[k])
	}

	if e.ListAttr != nil {
		emitCrdRequired(w, e, indent.GetPrefix(prefixLen-2))
	}

	prefixLen -= 2
	fmt.Fprintf(w, "%stype: object\n", indent.GetPrefix(prefixLen))

	prefixLen -= 2
	if e.ListAttr != nil {
		fmt.Fprintf(w, "%stype: array\n", indent.GetPrefix(prefixLen))
	}
}

// Use the key statements for list fields for required
func emitCrdRequired(w io.Writer, e *yang.Entry, prefix string) {
	if e.ListAttr == nil {
		return
	}

	if e.Key == "" {
		return
	}

	required := strings.Split(strings.TrimSpace(e.Key), " ")
	fmt.Fprintf(w, "%srequired:\n", prefix)

	for _, field := range required {
		fmt.Fprintf(w, "%s- %s\n", prefix, field)
	}
}

func emitCrdType(w io.Writer, e *yang.Entry, prefix string) {
	if e == nil || e.Type == nil || e.Type.Root.Name == "" {
		fmt.Fprintf(w, "%stype: string\n", prefix)

		return
	}

	if e.Type.Kind == yang.Yenum {
		names := e.Type.Enum.Names()

		fmt.Fprintf(w, "%senum:\n", prefix)
		for _, n := range names {
			name := yang.CamelCase(n, true)
			fmt.Fprintf(w, "%s- %s\n", prefix, name)
		}

		fmt.Fprintf(w, "%stype: string\n", prefix)
		return
	}

	crdType, ok := typeMap[e.Type.Root.Name]
	if !ok {
		crdType = "string"
	}

	fmt.Fprintf(w, "%stype: %s\n", prefix, crdType)
}
