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

package yang

import (
	"strings"
)

func ToYangCompatible(object map[string]interface{}, module string, paths []string,
	containerNode,
	leafNode string,
) (map[string]interface{}, error) {
	entry, err := ModuleToYangEntry(module, paths, containerNode, leafNode)
	if err != nil {
		return nil, err
	}

	return toMap(object, entry, true)
}

func FromYangCompatible(object map[string]interface{}, module string, paths []string,
	containerNode,
	leafNode string,
) (map[string]interface{}, error) {
	entry, err := ModuleToYangEntry(module, paths, containerNode, leafNode)
	if err != nil {
		return nil, err
	}

	return toMap(object, entry, false)
}

func toMap(object map[string]interface{}, entry *Entry, toYang bool) (map[string]interface{}, error) {
	// transform the object by folding names to lower case
	im := transform(object).(map[string]interface{})

	outputMap := make(map[string]interface{}, len(im))

	for _, e := range entry.Dir {
		name := casefold(e.Name)
		value, ok := im[name]
		if !ok {
			continue
		}

		entryName := e.Name
		if !toYang {
			entryName = CamelCase(entryName, false)
		}

		outputMap[entryName] = process(e, value, toYang)
	}

	return outputMap, nil
}

func process(e *Entry, value interface{}, toYang bool) interface{} {
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
			if !toYang {
				entryName = CamelCase(entryName, false)
			}

			output[entryName] = process(entry, val, toYang)
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
			list[i] = process(e, item, toYang)
		}

		return list
	}

	v, ok := value.(string)
	if !ok {
		if toYang || e.Type == nil {
			return value
		}

		switch t := value.(type) {
		case float64:
			switch e.Type.Kind {
			case Ydecimal64:
				return t //already right type
			default:
				// convert to int64
				return int64(t)
			}
		default:
			return value
		}
	}

	if e.Type == nil {
		return v
	}

	if e.Type.Kind == Yenum {
		// get actual names and use the one that matches the value
		names := e.Type.Enum.Names()

		for _, name := range names {
			if casefold(name) == casefold(v) {
				if !toYang {
					v = CamelCase(name, true)
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
