package main

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"text/template"
)

const (
	metadataToYaml = `
apiVersion: netconf.ciena.com/v1alpha1
kind: YangMetadata
metadata:
  namespace: {{.Namespace}}
  name: "{{.Name}}"
spec:
  reference:
    kind: {{.Kind}}
    group: {{.Group}}
    apiVersion: {{.Group}}/v1alpha1
  wrapper:
    config: {{.Config}}
    xmlRequestName: "{{.Root}}"
    xmlName: "{{.Instance}}"
    keyField: "{{.Key}}"
    module: "{{.Model}}"
    moduleSearchPath: "{{.ModelSearchPath}}"
`
)

type yangMetadata struct {
	Name, Namespace, Kind string
	Config                bool
	Root, Instance        string
	Key                   string
	Model                 string
	Group                 string
	ModelSearchPath       string
}

func generateMetadata(filename string, dependencies []string, namespace string, options *CrdOptions) error {
	if namespace == "" {
		namespace = "default"
	}

	name := strings.ToLower(options.Name) + "-meta"
	metadata := yangMetadata{
		Name:            name,
		Namespace:       namespace,
		Kind:            options.Name,
		Config:          options.Config,
		Root:            options.Root,
		Instance:        options.Instance,
		Key:             options.Key,
		Model:           path.Base(filename),
		Group:           options.Group,
		ModelSearchPath: getRelativePathWithBase("yang", options.ModuleSearchPath),
	}

	tmpl, err := template.New("metadata").Parse(metadataToYaml)
	if err != nil {
		return fmt.Errorf("%w: error parsing metadata template", err)
	}

	var metadataBuilder bytes.Buffer
	err = tmpl.Execute(&metadataBuilder, metadata)
	if err != nil {
		return fmt.Errorf("%w: error executing metadata template", err)
	}

	var yamlMap map[string]interface{}
	err = yaml.Unmarshal(metadataBuilder.Bytes(), &yamlMap)
	if err != nil {
		return fmt.Errorf("%w: error unmarshaling yaml metadata", err)
	}

	var augmentationMap map[string]interface{}

	if options.Augmentor != "" {
		augmentationData, err := ioutil.ReadFile(options.Augmentor)
		if err != nil {
			return fmt.Errorf("%w: error reading augmentation data", err)
		}

		err = yaml.Unmarshal(augmentationData, &augmentationMap)
		if err != nil {
			return fmt.Errorf("%w: error unmarshaling augmentation data", err)
		}
	}

	spec := yamlMap["spec"].(map[interface{}]interface{})
	specWrapper := spec["wrapper"].(map[interface{}]interface{})

	if len(dependencies) > 0 {
		specWrapper["moduleDependencies"] = dependencies
	}

	if options.Key == "internal" && options.Config {
		specWrapper["skipReconcile"] = true
	}

	if augmentationMap != nil {
		specWrapper["augmentor"] = augmentationMap
	}

	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)

	err = encoder.Encode(yamlMap)
	if err != nil {
		return fmt.Errorf("%w: error encoding yaml metadata", err)
	}

	data := "---\n" + buf.String()

	outputDirectory = getOutputDirectory()
	metadataFileName := fmt.Sprintf("%s/%s_%s_meta.yaml", outputDirectory, options.Group, pluralize(options.Name))

	err = os.WriteFile(metadataFileName, []byte(data), 0666)
	if err != nil {
		return fmt.Errorf("%w: error writing metadata", err)
	}

	fmt.Println("Generated crd metadata", metadataFileName)

	return nil
}
