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
	"errors"
	"io/fs"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
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

	ErrNoModuleSearchPaths = errors.New("no yang module search paths found")
	filteredDirs           = []string{".git", "test", "RFC", "DRAFT"}
)

type trie struct {
	root *trieNode
}

type trieNode struct {
	children map[string]*trieNode
	leaf     string
}

func newTrie() *trie {
	return &trie{root: newTrieNode()}
}

func newTrieNode() *trieNode {
	return &trieNode{children: make(map[string]*trieNode)}
}

func (t *trie) add(key, value string) {
	root := t.root
	parts := strings.Split(key, ".")

	for _, part := range parts {
		p := casefold(part)
		child := root.children[p]
		if child == nil {
			child = newTrieNode()
			root.children[p] = child
		}

		root = child
	}

	root.leaf = value
}

func (t *trie) present(key string) bool {
	k := casefold(key)
	if _, ok := t.root.children[k]; ok {
		return true
	}

	return false
}

func (t *trie) find(key string) string {
	root := t.root
	parts := strings.Split(key, ".")

	for _, part := range parts {
		p := casefold(part)

		root = root.children[p]
		if root == nil {
			return ""
		}
	}

	return root.leaf
}

func getNamespaceAttribute(value string) []string {
	parts := strings.Split(value, "=")
	if len(parts) != 2 {
		return nil
	}

	if !strings.HasPrefix(parts[0], "xmlns") {
		return nil
	}

	parts[0] = "-" + parts[0]

	return parts
}

func augmentMap(augmentIndex *trie, key string, value interface{}) interface{} {
	m, ok := value.(map[string]interface{})
	if ok {
		outputMap := make(map[string]interface{}, len(m))

		nsAttrValue := augmentIndex.find(key)
		if nsAttrValue != "" {
			nsAttrs := getNamespaceAttribute(nsAttrValue)
			if len(nsAttrs) == 2 {
				outputMap[nsAttrs[0]] = nsAttrs[1]
			}
		}

		for k, v := range m {
			outputMap[k] = augmentMap(augmentIndex, key+"."+k, v)
		}

		return outputMap
	}

	items, ok := value.([]interface{})
	if ok {
		outputList := make([]interface{}, len(items))

		for i, item := range items {
			outputList[i] = augmentMap(augmentIndex, key, item)
		}

		return outputList
	}

	val, ok := value.(string)
	if !ok {
		return value
	}

	nsAttrValue := augmentIndex.find(key)
	if nsAttrValue == "" {
		return val
	}

	// embed a #text map
	textMap := make(map[string]interface{}, 2)
	nsAttrs := getNamespaceAttribute(nsAttrValue)
	if len(nsAttrs) != 2 {
		return val
	}

	textMap[nsAttrs[0]] = nsAttrs[1]
	textMap["#text"] = val

	return textMap
}

func augmentMapWithNamespace(object map[string]interface{}, augmentNamespace map[string]string) map[string]interface{} {
	if len(augmentNamespace) == 0 {
		return object
	}

	augmentIndex := newTrie()
	for k, v := range augmentNamespace {
		augmentIndex.add(k, v)
	}

	for k, v := range object {
		if !augmentIndex.present(k) {
			continue
		}

		object[k] = augmentMap(augmentIndex, k, v)
	}

	return object
}

func acceptPath(path string) bool {
	infos, err := ioutil.ReadDir(path)
	if err != nil {
		return false
	}

	// check if there is a yang file present in this directory
	for _, inf := range infos {
		if inf.Mode().IsRegular() && filepath.Ext(inf.Name()) == ".yang" {
			return true
		}
	}

	return false
}

func filteredDirectory(name string) bool {
	for _, dir := range filteredDirs {
		if name == dir {
			return true
		}
	}

	return false
}

func getSearchPaths(searchPath string) ([]string, error) {
	candidateList := []string{}

	err := filepath.WalkDir(searchPath,
		func(path string, dirEntry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if dirEntry.IsDir() {
				if filteredDirectory(dirEntry.Name()) {
					return filepath.SkipDir
				}

				candidateList = append(candidateList, path)
			}

			return nil
		})

	if err != nil {
		return nil, err
	}

	shortList := make([]string, 0, len(candidateList))

	for _, dir := range candidateList {
		if acceptPath(dir) {
			shortList = append(shortList, dir)
		}
	}

	if len(shortList) == 0 {
		return nil, ErrNoModuleSearchPaths
	}

	return shortList, nil
}

func ToYangCompatible(object map[string]interface{}, augmentNamespace map[string]string, module string, searchPath string,
	containerNode,
	leafNode string,
) (map[string]interface{}, string, error) {
	paths, err := getSearchPaths(searchPath)
	if err != nil {
		return nil, "", err
	}

	entry, namespace, err := ModuleToYangEntry(module, paths, containerNode, leafNode)
	if err != nil {
		return nil, "", err
	}

	m, err := toMap(object, nil, entry, true)
	if err != nil {
		return nil, "", err
	}

	return augmentMapWithNamespace(m, augmentNamespace), namespace, nil
}

func FromYangCompatible(object map[string]interface{}, module string, searchPath string,
	containerNode,
	leafNode string,
) (map[string]interface{}, string, error) {
	paths, err := getSearchPaths(searchPath)
	if err != nil {
		return nil, "", err
	}

	entry, namespace, err := ModuleToYangEntry(module, paths, containerNode, leafNode)
	if err != nil {
		return nil, "", err
	}

	m, err := toMap(object, nil, entry, false)
	if err != nil {
		return nil, "", err
	}

	return m, namespace, nil
}

func toMap(object map[string]interface{}, intermediate map[string]interface{}, entry *Entry, toYang bool) (map[string]interface{}, error) {
	if intermediate == nil {
		// transform the object by folding names to lower case
		intermediate = transform(object).(map[string]interface{})
	}

	outputMap := make(map[string]interface{}, len(intermediate))

	for _, e := range entry.Dir {
		if e.IsChoice() {
			for _, caseEntry := range e.Dir {
				result, _ := toMap(object, intermediate, caseEntry, toYang)
				for k, v := range result {
					outputMap[k] = v
				}
			}
			continue
		}
		name := casefold(e.Name)
		value, ok := intermediate[name]
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

func mergeCaseResult(_ *Entry, result interface{}, output map[string]interface{}, _ bool) {
	m, ok := result.(map[string]interface{})
	if ok {
		for k, v := range m {
			output[k] = v
		}

		return
	}
}

func process(e *Entry, value interface{}, toYang bool) interface{} {
	if e.IsChoice() {
		output := make(map[string]interface{}, len(e.Dir))

		for _, caseEntry := range e.Dir {
			result := process(caseEntry, value, toYang)
			mergeCaseResult(caseEntry, result, output, toYang)
		}

		return output
	}

	m, ok := value.(map[string]interface{})
	if ok {
		if e.Dir == nil {
			if textValue, ok := m["#text"]; ok {
				return textValue
			}

			return make(map[string]interface{})
		}

		output := make(map[string]interface{}, len(e.Dir))

		for _, entry := range e.Dir {
			if entry.IsChoice() {
				for _, caseEntry := range entry.Dir {
					result := process(caseEntry, value, toYang)
					mergeCaseResult(caseEntry, result, output, toYang)
				}

				continue
			}

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
			switch TypeMap[e.Type.Root.Name] {
			case "number":
				return t //already right type
			case "integer":
				// convert to int64
				return int64(t)
			case "string":
				fallthrough
			default:
				return strconv.FormatInt(int64(t), 10)
			}
		case int64:
			switch TypeMap[e.Type.Root.Name] {
			case "integer":
				return t //already right type
			case "number":
				return float64(t)
			case "string":
				fallthrough
			default:
				return strconv.FormatInt(t, 10)
			}
		case int32:
			switch TypeMap[e.Type.Root.Name] {
			case "integer":
				return int64(t)
			case "number":
				return float64(t)
			case "string":
				fallthrough
			default:
				return strconv.FormatInt(int64(t), 10)
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
