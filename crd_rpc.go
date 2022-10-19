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
	"github.com/karthick18/goyang/pkg/indent"
	"github.com/karthick18/goyang/pkg/yang"
	"github.com/pborman/getopt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

func init() {
	opt := getopt.New()
	opt.StringVarLong(&crdTemplate, "rpc-template", 'e', "specify template file to generate the crd schema.")
	opt.StringVarLong(&metadataNamespace, "rpc-metadata-namespace", 'v', "specify metadata namespace to generate the crd metadata.")
	opt.StringVarLong(&crdGroup, "rpc-group", 'w', "specify group name for crd creation.")
	opt.StringVarLong(&crdName, "rpc-crd-name", 'y', "specify crd name for openapiv3 schema")
	opt.StringVarLong(&outputDirectory, "rpc-output-dir", 'z', "specify output directory name for generating openapiv3 schema. Defaults to current directory.")
	register(&formatter{
		name:  "rpc",
		flags: opt,
		f:     doRpcCrd,
		help:  "display in a rpc crd format",
	})
}

func doRpcCrd(w io.Writer, entries []*yang.Entry, filename string, dependencies []string, opts ...string) {
	base := path.Base(filename)
	fileBaseName := base[:len(base)-len(path.Ext(base))]
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

	rpcs := []*yang.Entry{}
	for _, e := range entry.Dir {
		if e.RPC == nil {
			continue
		}

		rpcs = append(rpcs, e)
	}

	if len(rpcs) == 0 {
		fmt.Fprintln(os.Stderr, "no rpc entries found")

		os.Exit(1)
	}

	rpcNames := make([]string, len(rpcs))
	for i, rpc := range rpcs {
		rpcNames[i] = rpc.Name
	}

	sort.Strings(rpcNames)
	var b strings.Builder
	fmt.Fprintln(&b, "spec:")
	fmt.Fprintln(&b, "  properties:")
	fmt.Fprintln(&b, "    rpcs:")
	fmt.Fprintln(&b, "      properties:")

	prefixLen := 8

	for _, n := range rpcNames {
		processEntry(entry.Dir[n], &b, prefixLen)
	}

	fmt.Fprintln(&b, "      type: object")
	fmt.Fprintln(os.Stdout, b.String())
}

func processEntry(entry *yang.Entry, builder *strings.Builder, prefixLen int) {
	prefix := indent.GetPrefix(prefixLen)
	name := yang.CamelCase(entry.Name, false)
	fmt.Fprintf(builder, "%s%s:\n", prefix, name)
	fmt.Fprintf(builder, "%stype: boolean\n", indent.GetPrefix(prefixLen+2))
	if entry.RPC.Input != nil {
		fmt.Fprintf(builder, "%s%sInput:\n", prefix, name)
		WriteIntermediate(entry.RPC.Input, builder, prefixLen+2)
	}

	if entry.RPC.Output != nil {
		fmt.Fprintf(builder, "%s%sOutput:\n", prefix, name)
		WriteIntermediate(entry.RPC.Output, builder, prefixLen+2)
	}
}

func WriteIntermediate(entry *yang.Entry, builder *strings.Builder, prefixLen int) {
	switch {
	case entry.Dir == nil && entry.ListAttr != nil:
		fmt.Fprintf(builder, "%stype: array\n", indent.GetPrefix(prefixLen))
		fmt.Fprintf(builder, "%sitems:\n", indent.GetPrefix(prefixLen))
		emitCrdType(builder, entry, indent.GetPrefix(prefixLen+2))
		return
	case entry.Dir == nil:
		emitCrdType(builder, entry, indent.GetPrefix(prefixLen))
		return
	case entry.ListAttr != nil:
		fmt.Fprintf(builder, "%stype: array\n", indent.GetPrefix(prefixLen))
		fmt.Fprintf(builder, "%sitems:\n", indent.GetPrefix(prefixLen))
		prefixLen += 2
		fmt.Fprintf(builder, "%sproperties:\n", indent.GetPrefix(prefixLen))
		prefixLen += 2
	default:
		fmt.Fprintf(builder, "%sproperties:\n", indent.GetPrefix(prefixLen))
		prefixLen += 2
	}

	var names []string
	for k := range entry.Dir {
		names = append(names, k)
	}

	sort.Strings(names)

	for _, k := range names {
		WriteCrd(indent.NewWriter(builder, indent.GetPrefix(prefixLen)), entry.Dir[k])
	}

	prefixLen -= 2
	fmt.Fprintf(builder, "%stype: object\n", indent.GetPrefix(prefixLen))
}
