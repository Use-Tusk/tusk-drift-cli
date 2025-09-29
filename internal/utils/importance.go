package utils

import (
	"google.golang.org/protobuf/types/known/structpb"
)

// ReduceByMatchImportance returns a copy of 'value' keeping only fields with matchImportance != 0.
// Schema is a structpb.Struct with optional per-field "matchImportance": 0|1 (default 1).
// Supports nested objects and arrays (uses 'items' schema for arrays if present).
func ReduceByMatchImportance(value any, schema *structpb.Struct) any {
	if value == nil || schema == nil || schema.Fields == nil {
		return value
	}
	return reduce(value, schema.AsMap())
}

func reduce(v any, s any) any {
	if v == nil || s == nil {
		return v
	}
	switch v := v.(type) {
	case map[string]any:
		sm, _ := s.(map[string]any)
		out := map[string]any{}
		for k, val := range v {
			fieldSchema := childFieldSchema(sm, k)
			if importance(fieldSchema) == 0 {
				continue
			}
			out[k] = reduce(val, fieldSchema)
		}
		return out
	case []any:
		sm, _ := s.(map[string]any)
		itemSchema := childFieldSchema(sm, "items")
		if importance(itemSchema) == 0 {
			return []any{} // drop items if items are low-importance
		}
		out := make([]any, 0, len(v))
		for _, it := range v {
			out = append(out, reduce(it, itemSchema))
		}
		return out
	default:
		// primitives: keep if schema importance != 0
		if importance(s) == 0 {
			return nil
		}
		return v
	}
}

func childFieldSchema(schema map[string]any, key string) any {
	if schema == nil {
		return nil
	}
	// Prefer nested under "properties"
	if props, ok := schema["properties"].(map[string]any); ok {
		if f, ok := props[key]; ok {
			return f
		}
	}
	// Or direct field
	if f, ok := schema[key]; ok {
		return f
	}
	return nil
}

func importance(s any) int {
	if m, ok := s.(map[string]any); ok {
		if imp, ok := m["matchImportance"]; ok {
			switch iv := imp.(type) {
			case float64:
				if iv == 0 {
					return 0
				}
				return 1
			case int:
				if iv == 0 {
					return 0
				}
				return 1
			case bool:
				if !iv {
					return 0
				}
				return 1
			}
		}
	}
	return 1
}
