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

	rootNodeModel     string
	instanceNodeModel string
	crdName           string
	metadataNamespace string
	outputDirectory   string
	noConfig          bool
	crdTemplate       string
	crdGroup          string
)

func init() {
	opt := getopt.New()
	opt.StringVarLong(&rootNodeModel, "root-node", 'r', "specify root node for the yang model")
	opt.StringVarLong(&instanceNodeModel, "crd-node", 'c', "specify crd node for the yang model")
	opt.StringVarLong(&crdName, "crd-name", 'n', "specify crd name for openapiv3 schema")
	opt.StringVarLong(&outputDirectory, "output-dir", 'd', "specify output directory name for generating openapiv3 schema. Defaults to current directory.")
	opt.BoolVarLong(&noConfig, "no-config", 'o', "enable crd generation with config false. An example could be querying operational status.")
	opt.StringVarLong(&crdTemplate, "crd-template", 'l', "specify template file to generate the crd schema.")
	opt.StringVarLong(&metadataNamespace, "metadata-namespace", 'm', "specify metadata namespace to generate the crd metadata.")
	opt.StringVarLong(&crdGroup, "group", 'u', "specify group name for crd creation.")
	register(&formatter{
		name:               "crd",
		flags:              opt,
		f:                  doCrd,
		validateArgs:       validateArgs,
		extractFileOptions: extractFileOptions,
		help:               "display in a crd format",
	})
}

func doCrd(w io.Writer, entries []*yang.Entry, filename string, opts ...string) {
	base := path.Base(filename)
	fileBaseName := base[:len(base)-len(path.Ext(base))]
	options := ""
	if len(opts) > 0 {
		options = opts[0]
	}

	crdOptions := parseOptions(options)

	var entry *yang.Entry

	for _, e := range entries {
		if e.Name != fileBaseName {
			continue
		}

		entry = e
		break
	}

	if entry == nil || entry.Dir == nil {
		fmt.Fprintf(os.Stderr, "Unable to find entry %s for module:%s\n", fileBaseName, filename)
		os.Exit(1)
	}

	var processEntry *yang.Entry
	var err error

	rootNode, instanceNode := crdOptions.Root, crdOptions.Instance
	if rootNode == "" || instanceNode == "" {
		rootNode, instanceNode, processEntry, err = getRootInstanceEntry(entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)

			os.Exit(1)
		}

		crdOptions.Root, crdOptions.Instance = rootNode, instanceNode
	} else {
		for name, node := range entry.Dir {
			if name == rootNode {
				if node.Dir != nil {
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
				}
				break
			}
		}
	}

	if processEntry == nil {
		fmt.Fprintf(os.Stderr, "Unable to find root %s, instance %s for module %s\n", rootNode, instanceNode, filename)
		os.Exit(1)
	}

	if crdOptions.Key == "" {
		key, err := getKeyForEntry(processEntry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}

		crdOptions.Key = key
	}

	crdOptions.Key = yang.CamelCase(crdOptions.Key, false)

	if !noConfig {
		generateSpec(crdOptions, processEntry)
	} else {
		generateStatus(crdOptions, processEntry)
	}

	if err := generateMetadata(filename, metadataNamespace, crdOptions); err != nil {
		fmt.Fprintf(os.Stderr, "generating metadata failed with error: %s\n", err.Error())
		os.Exit(1)
	}
}

type crdConfig struct {
	CrdName    string
	ShortNames []string
	Group      string
}

func generateSpec(options *CrdOptions, processEntry *yang.Entry) {
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

	executeTemplate(options, b.String(), "")
}

func generateStatus(options *CrdOptions, processEntry *yang.Entry) {
	var status strings.Builder
	generateStatusFields(&status, processEntry, true)

	statusContent := status.String()
	if statusContent == "" {
		fmt.Fprintf(os.Stderr, "no status fields to generate\n")
		os.Exit(1)
	}

	executeTemplate(options, "", statusContent)
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
		if readOnlyRootNode { //all nodes below it are read-only if root node is read-only
			WriteCrd(indent.NewWriter(builder, indent.GetPrefix(prefixLen)), processEntry.Dir[name])
		} else if processEntry.Dir[name].ReadOnly() {
			// take only the children of root nodes that are config false
			WriteCrd(indent.NewWriter(builder, indent.GetPrefix(prefixLen)), processEntry.Dir[name])
		}
	}

	if property {
		if readOnlyRootNode {
			emitCrdRequired(builder, processEntry, "")
		}

		fmt.Fprintln(builder, "type: object")
	}
}

func executeTemplate(options *CrdOptions, spec, status string) {
	crdTemplateFile := filepath.Base(crdTemplate)
	templateFile := crdTemplate

	if _, err := os.Stat(templateFile); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "template file %s does not exist\n", templateFile)
		} else {
			fmt.Fprintf(os.Stderr, "error %v accessing template file %s\n", err, templateFile)
		}

		os.Exit(1)
	}

	files := []string{templateFile}

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

	if status != "" {
		funcMap["GetStatusFields"] = func(spaces int) string {
			return indentContent(spaces, status)
		}
	}

	crdTemplate, err := template.New(crdTemplateFile).Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		panic(err.Error())
	}

	rootNode := yang.CamelCase(options.Root, false)
	crdNode := yang.CamelCase(options.Instance, false)
	crdName := options.Name

	if crdName == "" {
		// try to use the root node if possible
		if strings.ToLower(crdNode)+"s" == strings.ToLower(rootNode) {
			crdName = crdNode
		} else {
			crdName = rootNode
		}
	}

	crdName = yang.CamelCase(crdName, true)
	options.Name = crdName
	config := crdConfig{
		CrdName: crdName,
		Group:   options.Group,
	}

	config.ShortNames = getShortNames(crdName)

	outputDirectory = getOutputDirectory()

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
func WriteCrd(w io.Writer, e *yang.Entry) {
	if e.RPC != nil {
		return
	}

	if e.IsChoice() {
		caseNames := make([]string, 0, len(e.Dir))

		for k := range e.Dir {
			caseNames = append(caseNames, k)
		}

		sort.Strings(caseNames)

		for _, caseName := range caseNames {
			WriteCrd(w, e.Dir[caseName])
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
			WriteCrd(w, e.Dir[name])
		}

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
