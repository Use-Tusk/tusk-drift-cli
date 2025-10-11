package utils

import (
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

// ReduceByMatchImportance returns a copy of 'value' keeping only fields with matchImportance != 0.
// Schema is a JsonSchema proto with optional per-field "matchImportance": 0|1 (default 1).
// Supports nested objects and arrays (uses 'items' schema for arrays if present).
func ReduceByMatchImportance(value any, schema *core.JsonSchema) any {
	if value == nil || schema == nil {
		return value
	}
	return reduce(value, schema)
}

func reduce(v any, schema *core.JsonSchema) any {
	if v == nil || schema == nil {
		return v
	}

	switch v := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range v {
			fieldSchema := getFieldSchema(schema, k)
			if getImportance(fieldSchema) == 0 {
				continue
			}
			out[k] = reduce(val, fieldSchema)
		}
		return out

	case []any:
		itemSchema := schema.Items
		if getImportance(itemSchema) == 0 {
			return []any{} // drop items if items are low-importance
		}
		out := make([]any, 0, len(v))
		for _, it := range v {
			out = append(out, reduce(it, itemSchema))
		}
		return out

	default:
		// primitives: keep if schema importance != 0
		if getImportance(schema) == 0 {
			return nil
		}
		return v
	}
}

// getFieldSchema returns the schema for a specific field in an object
func getFieldSchema(schema *core.JsonSchema, fieldName string) *core.JsonSchema {
	if schema == nil || schema.Properties == nil {
		return nil
	}
	return schema.Properties[fieldName]
}

// getImportance returns 0 if matchImportance is explicitly 0, otherwise returns 1
func getImportance(schema *core.JsonSchema) int {
	if schema == nil {
		return 1 // default to important if no schema
	}
	if schema.MatchImportance != nil && *schema.MatchImportance == 0 {
		return 0
	}
	return 1
}

// ReduceSchemaByMatchImportance returns a copy of the schema with only fields that have matchImportance != 0
// Theoretically match importance could be a decimal, but we don't support that yet.
func ReduceSchemaByMatchImportance(schema *core.JsonSchema) *core.JsonSchema {
	if schema == nil {
		return nil
	}

	if getImportance(schema) == 0 {
		return nil
	}

	// Create a shallow copy of the schema
	reduced := &core.JsonSchema{
		Type:            schema.Type,
		Encoding:        schema.Encoding,
		DecodedType:     schema.DecodedType,
		MatchImportance: schema.MatchImportance,
	}

	// Recursively reduce properties
	if len(schema.Properties) > 0 {
		reduced.Properties = make(map[string]*core.JsonSchema)
		for key, propSchema := range schema.Properties {
			if getImportance(propSchema) != 0 {
				reduced.Properties[key] = ReduceSchemaByMatchImportance(propSchema)
			}
		}
	}

	// Recursively reduce items schema
	if schema.Items != nil && getImportance(schema.Items) != 0 {
		reduced.Items = ReduceSchemaByMatchImportance(schema.Items)
	}

	return reduced
}
