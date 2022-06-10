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
	"encoding/json"
	"fmt"
	"github.com/clbanning/mxj/v2"
	"github.com/openconfig/goyang/pkg/yang"
	"os"
	"strings"
)

func decodeJsonToXml(jsonFile string, jsonOutputFile string, root *yang.Entry, camelCase bool) {
	var object map[string]interface{}
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading json file: %w\n", err)

		return
	}

	err = json.Unmarshal(data, &object)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error json unmarshal: %w\n", err)

		return
	}

	fmt.Fprintf(os.Stdout, "json unmarshalled: %v\n", object)

	output := transform(object).(map[string]interface{})

	//fmt.Fprintf(os.Stdout, "transformed object: %v\n", output)

	xmlMap := make(map[string]interface{}, len(output))

	for _, e := range root.Dir {
		name := casefold(e.Name)
		value, ok := output[name]
		if !ok {
			continue
		}
		entryName := e.Name
		if camelCase {
			entryName = yang.CamelCase(entryName, false)
		}
		xmlMap[entryName] = process(e, value, camelCase)
	}

	fmt.Fprintf(os.Stdout, "transformed xmlmap: %v\n", xmlMap)

	xmlData, err := encodeXml(xmlMap, root.Name)
	if err != nil {
		return
	}

	xmlObject, err := mxj.NewMapXml(xmlData, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xml unmarshal error: %w", err)

		return
	}

	xmlObject = xmlObject[root.Name].(map[string]interface{})
	fmt.Fprintf(os.Stdout, "\n\nxml unmarshalled object:\n%s\n\n", map[string]interface{}(xmlObject))

	data, err = json.MarshalIndent(xmlObject, "", " ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal error: %w\n", err)

		return
	}

	os.WriteFile(jsonOutputFile, data, 0666)
}

func encodeXml(xmlMap map[string]interface{}, root string) ([]byte, error) {
	data, err := mxj.AnyXmlIndent(xmlMap, "", " ", root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling map to xml: %w", err)

		return nil, err
	}

	fmt.Fprintf(os.Stdout, "xml-encode:\n%s\n", string(data))

	return data, nil
}

func process(e *yang.Entry, value interface{}, camelCase bool) interface{} {
	m, ok := value.(map[string]interface{})
	if ok {
		if e.Dir == nil {
			return make(map[string]interface{})
		}

		output := make(map[string]interface{}, len(e.Dir))

		for _, entry := range e.Dir {
			name := casefold(entry.Name)
			val, ok := m[name]
			if !ok {
				continue
			}
			entryName := entry.Name
			if camelCase {
				entryName = yang.CamelCase(entryName, false)
			}
			output[entryName] = process(entry, val, camelCase)
		}

		return output
	}

	items, ok := value.([]interface{})
	if ok {
		var list []interface{}
		if e.ListAttr == nil {
			return list
		}

		list = make([]interface{}, len(items))
		for i, item := range items {
			list[i] = process(e, item, camelCase)
		}

		return list
	}

	v, ok := value.(string)
	if !ok {
		return value
	}

	if e.Type == nil {
		return v
	}

	if e.Type.Kind == yang.Yenum {
		// get actual names and use the one that matches the value
		names := e.Type.Enum.Names()

		for _, name := range names {
			if casefold(name) == casefold(v) {
				if camelCase {
					v = yang.CamelCase(name, true)
				} else {
					v = name
				}
				break
			}
		}
	}

	return v
}

func casefold(input string) string {
	input = strings.ToLower(input)

	return strings.ReplaceAll(strings.ReplaceAll(input, "-", ""), "_", "")
}

func transform(val interface{}) interface{} {
	m, ok := val.(map[string]interface{})
	if ok {
		output := make(map[string]interface{}, len(m))

		for k, v := range m {
			output[casefold(k)] = transform(v)
		}

		return output
	}

	items, ok := val.([]interface{})
	if ok {
		list := make([]interface{}, len(items))
		for i, item := range items {
			list[i] = transform(item)
		}

		return list
	}

	return val
}
