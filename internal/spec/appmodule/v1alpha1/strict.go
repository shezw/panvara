/*
   Panvara
   internal/spec/appmodule/v1alpha1/strict.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package v1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	maxDocumentDepth = 64
	maxDocumentNodes = 100_000
)

type shapeKind uint8

const (
	shapeScalar shapeKind = iota
	shapeObject
	shapeArray
)

type jsonShape struct {
	kind       shapeKind
	fields     map[string]*jsonShape
	additional *jsonShape
	element    *jsonShape
}

func validateJSONStructure(source []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	nodes := 0
	if err := consumeJSONValue(decoder, token, documentShape(), "$", 1, &nodes); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple values are not allowed")
		}
		return err
	}
	return nil
}

func consumeJSONValue(
	decoder *json.Decoder,
	token json.Token,
	shape *jsonShape,
	path string,
	depth int,
	nodes *int,
) error {
	*nodes++
	if *nodes > maxDocumentNodes {
		return fmt.Errorf("document has more than %d nodes", maxDocumentNodes)
	}
	if depth > maxDocumentDepth {
		return fmt.Errorf("document exceeds maximum depth %d", maxDocumentDepth)
	}

	delimiter, isDelimiter := token.(json.Delim)
	switch shape.kind {
	case shapeObject:
		if !isDelimiter || delimiter != '{' {
			return fmt.Errorf("%s must be an object", path)
		}
		seen := make(map[string]struct{}, len(shape.fields))
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("%s has a non-string key", path)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("%s has duplicate key %q", path, key)
			}
			seen[key] = struct{}{}
			child, known := shape.fields[key]
			if !known {
				child = shape.additional
			}
			if child == nil {
				return fmt.Errorf("%s has unknown field %q", path, key)
			}
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := consumeJSONValue(decoder, valueToken, child, path+"."+key, depth+1, nodes); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("%s object is not closed", path)
		}
		return nil
	case shapeArray:
		if !isDelimiter || delimiter != '[' {
			return fmt.Errorf("%s must be an array", path)
		}
		index := 0
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := consumeJSONValue(
				decoder,
				valueToken,
				shape.element,
				fmt.Sprintf("%s[%d]", path, index),
				depth+1,
				nodes,
			); err != nil {
				return err
			}
			index++
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("%s array is not closed", path)
		}
		return nil
	default:
		if isDelimiter {
			return fmt.Errorf("%s must be a scalar", path)
		}
		if token == nil {
			return fmt.Errorf("%s cannot be null", path)
		}
		return nil
	}
}

func validateYAMLStructure(source []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		return errors.New("document is empty")
	}
	nodes := 0
	if err := inspectYAMLNode(&root, 1, &nodes); err != nil {
		return err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple documents are not allowed")
		}
		return err
	}
	return nil
}

func inspectYAMLNode(node *yaml.Node, depth int, nodes *int) error {
	*nodes++
	if *nodes > maxDocumentNodes {
		return fmt.Errorf("document has more than %d nodes", maxDocumentNodes)
	}
	if depth > maxDocumentDepth {
		return fmt.Errorf("document exceeds maximum depth %d", maxDocumentDepth)
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return errors.New("YAML aliases and anchors are not allowed")
	}
	if node.Tag == "!!merge" || node.Value == "<<" && node.Kind == yaml.ScalarNode {
		return errors.New("YAML merge keys are not allowed")
	}
	if node.Tag == "!!null" || node.Tag == "tag:yaml.org,2002:null" {
		return errors.New("YAML null values are not allowed")
	}
	if node.Tag != "" &&
		!strings.HasPrefix(node.Tag, "!!") &&
		!strings.HasPrefix(node.Tag, "tag:yaml.org,2002:") {
		return fmt.Errorf("custom YAML tag %q is not allowed", node.Tag)
	}
	for _, child := range node.Content {
		if err := inspectYAMLNode(child, depth+1, nodes); err != nil {
			return err
		}
	}
	return nil
}

func documentShape() *jsonShape {
	scalar := &jsonShape{kind: shapeScalar}
	stringsArray := &jsonShape{kind: shapeArray, element: scalar}
	labels := &jsonShape{kind: shapeObject, additional: scalar}
	constraints := objectShape(map[string]*jsonShape{
		"maxLength": scalar,
		"precision": scalar,
		"scale":     scalar,
	})
	access := objectShape(map[string]*jsonShape{
		"operations": stringsArray,
		"writable":   stringsArray,
		"filterable": stringsArray,
		"sortable":   stringsArray,
	})
	api := objectShape(map[string]*jsonShape{"public": access, "admin": access})
	manager := objectShape(map[string]*jsonShape{
		"list": objectShape(map[string]*jsonShape{
			"columns": stringsArray,
			"filters": stringsArray,
		}),
		"form": objectShape(map[string]*jsonShape{"fields": stringsArray}),
	})
	field := objectShape(map[string]*jsonShape{
		"name": scalar, "labels": labels, "type": scalar, "required": scalar,
		"unique": scalar, "target": scalar, "options": stringsArray,
		"constraints": constraints,
	})
	resource := objectShape(map[string]*jsonShape{
		"name": scalar, "labels": labels,
		"fields":  {kind: shapeArray, element: field},
		"api":     api,
		"manager": manager,
	})
	moduleRequirement := objectShape(map[string]*jsonShape{"name": scalar, "version": scalar})
	requires := objectShape(map[string]*jsonShape{
		"modules":      {kind: shapeArray, element: moduleRequirement},
		"capabilities": stringsArray,
	})
	return objectShape(map[string]*jsonShape{
		"apiVersion": scalar,
		"kind":       scalar,
		"metadata": objectShape(map[string]*jsonShape{
			"name": scalar, "version": scalar, "labels": labels,
		}),
		"spec": objectShape(map[string]*jsonShape{
			"requires": requires, "provides": stringsArray, "conflicts": stringsArray,
			"resources": {kind: shapeArray, element: resource},
		}),
	})
}

func objectShape(fields map[string]*jsonShape) *jsonShape {
	return &jsonShape{kind: shapeObject, fields: fields}
}
