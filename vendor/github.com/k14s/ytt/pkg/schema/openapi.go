// Copyright 2021 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"sort"

	"github.com/k14s/ytt/pkg/yamlmeta"
)

// keys used when generating an OpenAPI Document
const (
	titleProp              = "title"
	typeProp               = "type"
	additionalPropsProp    = "additionalProperties"
	formatProp             = "format"
	nullableProp           = "nullable"
	deprecatedProp         = "deprecated"
	descriptionProp        = "description"
	exampleDescriptionProp = "x-example-description"
	exampleProp            = "example"
	itemsProp              = "items"
	propertiesProp         = "properties"
	defaultProp            = "default"
)

var propOrder = map[string]int{
	titleProp:              0,
	typeProp:               1,
	additionalPropsProp:    2,
	formatProp:             3,
	nullableProp:           4,
	deprecatedProp:         5,
	descriptionProp:        6,
	exampleDescriptionProp: 7,
	exampleProp:            8,
	itemsProp:              9,
	propertiesProp:         10,
	defaultProp:            11,
}

type openAPIKeys []*yamlmeta.MapItem

func (o openAPIKeys) Len() int {
	return len(o)
}

func (o openAPIKeys) Less(i, j int) bool {
	return propOrder[o[i].Key.(string)] < propOrder[o[j].Key.(string)]
}

func (o openAPIKeys) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

// OpenAPIDocument holds the document type used for creating an OpenAPI document
type OpenAPIDocument struct {
	docType *DocumentType
}

// NewOpenAPIDocument creates an instance of an OpenAPIDocument based on the given DocumentType
func NewOpenAPIDocument(docType *DocumentType) *OpenAPIDocument {
	return &OpenAPIDocument{docType}
}

// AsDocument generates a new AST of this OpenAPI v3.0.x document, populating the `schemas:` section with the
// type information contained in `docType`.
func (o *OpenAPIDocument) AsDocument() *yamlmeta.Document {
	openAPIProperties := o.calculateProperties(o.docType)

	return &yamlmeta.Document{Value: &yamlmeta.Map{Items: []*yamlmeta.MapItem{
		{Key: "openapi", Value: "3.0.0"},
		{Key: "info", Value: &yamlmeta.Map{Items: []*yamlmeta.MapItem{
			{Key: "version", Value: "0.1.0"},
			{Key: titleProp, Value: "Schema for data values, generated by ytt"},
		}}},
		{Key: "paths", Value: &yamlmeta.Map{}},
		{Key: "components", Value: &yamlmeta.Map{Items: []*yamlmeta.MapItem{
			{Key: "schemas", Value: &yamlmeta.Map{Items: []*yamlmeta.MapItem{
				{Key: "dataValues", Value: openAPIProperties},
			}}},
		}}},
	}}}
}

func (o *OpenAPIDocument) calculateProperties(schemaVal interface{}) *yamlmeta.Map {
	switch typedValue := schemaVal.(type) {
	case *DocumentType:
		return o.calculateProperties(typedValue.GetValueType())
	case *MapType:
		var items openAPIKeys
		items = append(items, collectDocumentation(typedValue)...)
		items = append(items, &yamlmeta.MapItem{Key: typeProp, Value: "object"})
		items = append(items, &yamlmeta.MapItem{Key: additionalPropsProp, Value: false})

		var properties []*yamlmeta.MapItem
		for _, i := range typedValue.Items {
			mi := yamlmeta.MapItem{Key: i.Key, Value: o.calculateProperties(i.GetValueType())}
			properties = append(properties, &mi)
		}
		items = append(items, &yamlmeta.MapItem{Key: propertiesProp, Value: &yamlmeta.Map{Items: properties}})

		sort.Sort(items)
		return &yamlmeta.Map{Items: items}
	case *ArrayType:
		var items openAPIKeys
		items = append(items, collectDocumentation(typedValue)...)
		items = append(items, &yamlmeta.MapItem{Key: typeProp, Value: "array"})
		items = append(items, &yamlmeta.MapItem{Key: defaultProp, Value: typedValue.GetDefaultValue()})

		valueType := typedValue.GetValueType().(*ArrayItemType)
		properties := o.calculateProperties(valueType.GetValueType())
		items = append(items, &yamlmeta.MapItem{Key: itemsProp, Value: properties})

		sort.Sort(items)
		return &yamlmeta.Map{Items: items}
	case *ScalarType:
		var items openAPIKeys
		items = append(items, collectDocumentation(typedValue)...)
		items = append(items, &yamlmeta.MapItem{Key: defaultProp, Value: typedValue.GetDefaultValue()})

		typeString := o.openAPITypeFor(typedValue)
		items = append(items, &yamlmeta.MapItem{Key: typeProp, Value: typeString})
		if typedValue.String() == "float" {
			items = append(items, &yamlmeta.MapItem{Key: formatProp, Value: "float"})
		}

		sort.Sort(items)
		return &yamlmeta.Map{Items: items}
	case *NullType:
		var items openAPIKeys
		items = append(items, collectDocumentation(typedValue)...)
		items = append(items, &yamlmeta.MapItem{Key: nullableProp, Value: true})

		properties := o.calculateProperties(typedValue.GetValueType())
		items = append(items, properties.Items...)

		sort.Sort(items)
		return &yamlmeta.Map{Items: items}
	case *AnyType:
		var items openAPIKeys
		items = append(items, collectDocumentation(typedValue)...)
		items = append(items, &yamlmeta.MapItem{Key: nullableProp, Value: true})
		items = append(items, &yamlmeta.MapItem{Key: defaultProp, Value: typedValue.GetDefaultValue()})

		sort.Sort(items)
		return &yamlmeta.Map{Items: items}
	default:
		panic(fmt.Sprintf("Unrecognized type %T", schemaVal))
	}
}

func collectDocumentation(typedValue Type) []*yamlmeta.MapItem {
	var items []*yamlmeta.MapItem
	if typedValue.GetTitle() != "" {
		items = append(items, &yamlmeta.MapItem{Key: titleProp, Value: typedValue.GetTitle()})
	}
	if typedValue.GetDescription() != "" {
		items = append(items, &yamlmeta.MapItem{Key: descriptionProp, Value: typedValue.GetDescription()})
	}
	if isDeprecated, _ := typedValue.IsDeprecated(); isDeprecated {
		items = append(items, &yamlmeta.MapItem{Key: deprecatedProp, Value: isDeprecated})
	}
	examples := typedValue.GetExamples()
	if len(examples) != 0 {
		items = append(items, &yamlmeta.MapItem{Key: exampleDescriptionProp, Value: examples[0].description})
		items = append(items, &yamlmeta.MapItem{Key: exampleProp, Value: examples[0].example})
	}
	return items
}

func (o *OpenAPIDocument) openAPITypeFor(astType *ScalarType) string {
	switch astType.ValueType {
	case StringType:
		return "string"
	case FloatType:
		return "number"
	case IntType:
		return "integer"
	case BoolType:
		return "boolean"
	default:
		panic(fmt.Sprintf("Unrecognized type: %T", astType.ValueType))
	}
}