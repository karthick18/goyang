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
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/karthick18/goyang/pkg/indent"
	"github.com/karthick18/goyang/pkg/yang"
	"github.com/pborman/getopt"
)

var (
	TypeMap = map[string]string{
		"int8":      "integer",
		"int16":     "integer",
		"int32":     "integer",
		"int64":     "integer",
		"uint8":     "integer",
		"uint16":    "integer",
		"uint32":    "integer",
		"uint64":    "integer",
		"bits":      "integer",
		"decimal64": "number",
		"string":    "string",
		"boolean":   "boolean",
		"leafref":   "string",
	}

	BooleanToStringMap = map[string]string{
		"on":    "Enable",
		"off":   "Disable",
		"yes":   "Enable",
		"no":    "Disable",
		"true":  "Enable",
		"false": "Disable",
	}

	rootNode        string
	instanceNode    string
	crdName         string
	outputDirectory string
	noConfig        bool
)

func init() {
	opt := getopt.New()
	opt.StringVarLong(&rootNode, "root-node", 'r', "specify root node for the yang model")
	opt.StringVarLong(&instanceNode, "crd-node", 'c', "specify crd node for the yang model")
	opt.StringVarLong(&crdName, "crd-name", 'n', "specify crd name for openapiv3 schema")
	opt.StringVarLong(&outputDirectory, "output-dir", 'd', "specify output directory name for generating openapiv3 schema. Defaults to current directory.")
	opt.BoolVarLong(&noConfig, "no-config", 'o', "enable crd generation with config false. An example could be querying operational status.")
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

	generated := false

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
				switch {
				case node.IsContainer(), node.IsList():
				default:
					fmt.Fprintf(os.Stderr, "Node %s is not a container/list node. Skipping...\n", rootNode)
					continue
				}

				if rootNode == instanceNode {
					processEntry = node
					break
				}

				if node.IsList() {
					fmt.Fprintf(os.Stderr, "Root node %s is a list and does not match instance node. Skipping...\n", rootNode)
					continue
				}

				for childNodeName, childNode := range node.Dir {
					if childNodeName == instanceNode {
						processEntry = childNode
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

		if processEntry.Dir != nil {
			if !noConfig {
				generateSpec(rootNode, instanceNode, processEntry)
			} else {
				generateStatus(rootNode, instanceNode, processEntry)
			}

			generated = true
		}
	}

	if !generated {
		fmt.Fprintf(os.Stderr, "No match found for root %s, crd %s node\n", rootNode, instanceNode)
		os.Exit(1)
	}
}

type crdConfig struct {
	CrdName    string
	ShortNames []string
	Group      string
}

func generateSpec(rootNode, instanceNode string, processEntry *yang.Entry) {
	var spec strings.Builder
	prefixLen := 0
	fmt.Fprintln(&spec, "spec:")
	fmt.Fprintln(&spec, "  properties:")

	prefixLen = 4
	var names []string

	for k := range processEntry.Dir {
		names = append(names, k)
	}

	sort.Strings(names)

	for _, name := range names {
		WriteCrd(indent.NewWriter(&spec, indent.GetPrefix(prefixLen)), processEntry.Dir[name], true)
	}

	emitCrdRequired(&spec, processEntry, indent.GetPrefix(2))
	fmt.Fprintln(&spec, "  type: object")

	// now generate the status fields
	var status strings.Builder
	generateStatusFields(&status, processEntry, false)

	executeTemplate(yang.CamelCase(rootNode, false), yang.CamelCase(instanceNode, false), spec.String(), status.String(), false)
}

func generateStatus(rootNode, instanceNode string, processEntry *yang.Entry) {
	var status strings.Builder
	generateStatusFields(&status, processEntry, true)
	executeTemplate(yang.CamelCase(rootNode, false), yang.CamelCase(instanceNode, false), "", status.String(), true)
}

func generateStatusFields(builder *strings.Builder, processEntry *yang.Entry, property bool) {
	prefixLen := 0
	if property {
		fmt.Fprintln(builder, "properties:")
		prefixLen += 2
	}

	var names []string

	for k := range processEntry.Dir {
		names = append(names, k)
	}

	sort.Strings(names)

	readOnlyRootNode := processEntry.ReadOnly()

	for _, name := range names {
		if readOnlyRootNode {
			WriteCrd(indent.NewWriter(builder, indent.GetPrefix(prefixLen)), processEntry.Dir[name], false)
		} else {
			WriteCrdReadOnly(indent.NewWriter(builder, indent.GetPrefix(prefixLen)), processEntry.Dir[name])
		}
	}

	if property {
		if readOnlyRootNode {
			emitCrdRequired(builder, processEntry, "")
		}

		fmt.Fprintln(builder, "type: object")
	}
}

func executeTemplate(rootNode, crdNode, spec, status string, noConfig bool) {
	crdTemplateFile := "crd.tmpl"
	if noConfig {
		crdTemplateFile = "crd_opstate.tmpl"
	}

	templateFile := filepath.Dir(os.Args[0]) + "/" + crdTemplateFile
	files := []string{templateFile}

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
	}

	indentContent := func(spaces int, content string) string {
		if content == "" {
			return ""
		}

		if content[len(content)-1] == '\n' {
			content = content[:len(content)-1]
		}

		return indent.String(indent.GetPrefix(spaces), content)
	}

	if spec != "" {
		funcMap["GetSpec"] = func(spaces int) string {
			return indentContent(spaces, spec)
		}
	}

	funcMap["GetStatusFields"] = func(spaces int) string {
		if status == "" {
			return ""
		}

		return indentContent(spaces, "\n"+status)
	}

	crdTemplate, err := template.New(crdTemplateFile).Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		panic(err.Error())
	}

	if crdName == "" {
		// try to use the root node if possible
		if strings.ToLower(crdNode)+"s" == strings.ToLower(rootNode) {
			crdName = crdNode
		} else {
			crdName = rootNode
		}
	}

	crdName = yang.CamelCase(crdName, true)

	config := crdConfig{
		CrdName: crdName,
		Group:   "netconf.ciena.com",
	}

	config.ShortNames = []string{pluralize(crdName)}

	if outputDirectory == "" {
		path, err := os.Getwd()
		if err != nil {
			panic("error getting current directory:" + err.Error())
		}

		outputDirectory = path
	} else {
		err = os.MkdirAll(outputDirectory, os.ModePerm)
		if err != nil && !os.IsExist(err) {
			panic("error mkdirall:" + err.Error())
		}
	}

	crdFile := fmt.Sprintf("%s/%s_%s.yaml", outputDirectory, config.Group, pluralize(crdName))
	f, err := os.Create(crdFile)
	if err != nil {
		panic(err.Error())
	}

	err = crdTemplate.Execute(f, config)
	if err != nil {
		panic("error executing template: " + err.Error())
	}

	fmt.Println("Generated crd", crdFile)
}

// Write writes e, formatted, and all of its children, to w.
func WriteCrd(w io.Writer, e *yang.Entry, filterReadOnly bool) {
	if e.RPC != nil {
		return
	}

	if filterReadOnly && e.Dir != nil && e.ReadOnly() {
		return
	}

	if e.IsChoice() {
		caseNames := make([]string, 0, len(e.Dir))

		for k := range e.Dir {
			caseNames = append(caseNames, k)
		}

		sort.Strings(caseNames)

		for _, caseName := range caseNames {
			WriteCrd(w, e.Dir[caseName], filterReadOnly)
		}

		return
	}

	if e.IsCase() {
		cases := make([]string, 0, len(e.Dir))
		for k := range e.Dir {
			cases = append(cases, k)
		}

		sort.Strings(cases)

		for _, name := range cases {
			WriteCrd(w, e.Dir[name], filterReadOnly)
		}

		return
	}

	var names []string

	if e.Dir != nil {
		for k := range e.Dir {
			if filterReadOnly && e.Dir[k].ReadOnly() {
				continue
			}
			names = append(names, k)
		}

		if len(names) == 0 {
			return
		}

		sort.Strings(names)
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

	for _, k := range names {
		WriteCrd(indent.NewWriter(w, indent.GetPrefix(prefixLen)), e.Dir[k], filterReadOnly)
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

func hasReadOnly(e *yang.Entry) bool {
	if e.ReadOnly() {
		return true
	}

	if e.Dir == nil {
		return false
	}

	if e.IsChoice() {
		for _, caseEntry := range e.Dir {
			if hasReadOnly(caseEntry) {
				return true
			}
		}

		return false
	}

	for k := range e.Dir {
		if hasReadOnly(e.Dir[k]) {
			return true
		}
	}

	return false
}

func WriteCrdReadOnly(w io.Writer, e *yang.Entry) {
	if e.RPC != nil {
		return
	}

	// write all nodes
	if e.ReadOnly() {
		WriteCrd(w, e, false)

		return
	}

	if e.IsChoice() {
		caseNames := make([]string, 0, len(e.Dir))

		for k := range e.Dir {
			caseNames = append(caseNames, k)
		}

		sort.Strings(caseNames)

		for _, caseName := range caseNames {
			WriteCrdReadOnly(w, e.Dir[caseName])
		}

		return
	}

	if e.IsCase() {
		cases := make([]string, 0, len(e.Dir))
		for k := range e.Dir {
			cases = append(cases, k)
		}

		sort.Strings(cases)

		for _, name := range cases {
			WriteCrdReadOnly(w, e.Dir[name])
		}

		return
	}

	// if this isn't a read-only node, then nothing to write for scalar entries
	if e.Dir == nil {
		return
	}

	// check if there are any read-only nodes for this node to process
	var names []string

	for k, node := range e.Dir {
		if node.ReadOnly() {
			names = append(names, k)
		} else if hasReadOnly(node) {
			names = append(names, k)
		}
	}

	// there are no read-only nodes. so skip this config=true node
	if len(names) == 0 {
		return
	}

	// we are here when we have read-only child nodes to process
	sort.Strings(names)

	prefixLen := 0
	name := yang.CamelCase(e.Name, false)

	fmt.Fprintf(w, "%s:\n", name)
	prefixLen += 2

	switch {
	case e.ListAttr != nil:
		fmt.Fprintln(w, "  items:")
		prefixLen += 2
		fmt.Fprintln(w, "    properties:")
		prefixLen += 2
	default:
		fmt.Fprintln(w, "  properties:")
		prefixLen += 2
	}

	for _, k := range names {
		if !e.Dir[k].ReadOnly() {
			WriteCrdReadOnly(indent.NewWriter(w, indent.GetPrefix(prefixLen)), e.Dir[k])
		} else {
			WriteCrd(indent.NewWriter(w, indent.GetPrefix(prefixLen)), e.Dir[k], false)
		}
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
		fmt.Fprintf(w, "%s- %s\n", prefix, yang.CamelCase(field, false))
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
			name := BooleanToStringMap[strings.ToLower(n)]
			if name == "" {
				name = yang.CamelCase(n, true)
			}

			fmt.Fprintf(w, "%s- %s\n", prefix, name)
		}

		fmt.Fprintf(w, "%stype: string\n", prefix)
		return
	}

	crdType, ok := TypeMap[e.Type.Root.Name]
	if !ok {
		crdType = "string"
	}

	fmt.Fprintf(w, "%stype: %s\n", prefix, crdType)

	// add ranges for integers
	if crdType == "integer" && len(e.Type.Range) == 1 && e.Type.Range[0].Valid() {
		min, err := e.Type.Range[0].Min.Int()
		if err != nil {
			return
		}

		max, err := e.Type.Range[0].Max.Int()
		if err != nil {
			return
		}
		fmt.Fprintf(w, "%sminimum: %d\n", prefix, min)
		fmt.Fprintf(w, "%smaximum: %d\n", prefix, max)
	}
}
