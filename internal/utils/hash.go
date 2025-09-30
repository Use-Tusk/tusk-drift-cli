package utils

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// GenerateSchemaAndHash generates JSON schema and deterministic hash for any value
func GenerateSchemaAndHash(value any) (schema any, valueHash string, schemaHash string) {
	schema = generateJSONSchema(value)
	valueHash = GenerateDeterministicHash(value)
	schemaHash = GenerateDeterministicHash(schema)
	return schema, valueHash, schemaHash
}

// GenerateDeterministicHash creates a deterministic hash of any JSON-serializable value
func GenerateDeterministicHash(value any) string {
	normalizedJSON := normalizeForHashing(value)
	jsonBytes, _ := json.Marshal(normalizedJSON)
	hash := sha256.Sum256(jsonBytes)
	return fmt.Sprintf("%x", hash)
}

// RemoveHeadersFromInputValue removes non-critical headers for header-agnostic matching
// but preserves headers that affect response format like Accept and Content-Type
func RemoveHeadersFromInputValue(inputValue any) any {
	if inputMap, ok := inputValue.(map[string]any); ok {
		result := make(map[string]any)
		for k, v := range inputMap {
			if k == "headers" {
				// Keep only critical headers that affect response format
				if headersMap, ok := v.(map[string]any); ok {
					criticalHeaders := make(map[string]any)
					for headerKey, headerValue := range headersMap {
						// Preserve headers that affect the response format
						if headerKey == "accept" || headerKey == "content-type" {
							criticalHeaders[headerKey] = headerValue
						}
					}
					if len(criticalHeaders) > 0 {
						result[k] = criticalHeaders
					}
				}
			} else {
				result[k] = v
			}
		}
		return result
	}
	return inputValue
}

// normalizeForHashing sorts maps by keys for consistent hashing
func normalizeForHashing(value any) any {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k, val := range v {
			// Skip null values to ensure consistency between recorded and live requests
			if val != nil {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)

		normalized := make(map[string]any)
		for _, k := range keys {
			if v[k] != nil {
				normalized[k] = normalizeForHashing(v[k])
			}
		}
		return normalized
	case []any:
		normalized := make([]any, len(v))
		for i, item := range v {
			normalized[i] = normalizeForHashing(item)
		}
		return normalized
	default:
		return v
	}
}

// generateJSONSchema creates a simple JSON schema for any value
func generateJSONSchema(value any) any {
	switch v := value.(type) {
	case nil:
		return map[string]any{"type": "NULL"}
	case bool:
		return map[string]any{"type": "BOOLEAN"}
	case float64:
		return map[string]any{"type": "NUMBER"}
	case string:
		return map[string]any{"type": "STRING"}
	case map[string]any:
		properties := make(map[string]any)
		for k, val := range v {
			properties[k] = generateJSONSchema(val)
		}
		return map[string]any{
			"type":       "OBJECT",
			"properties": properties,
		}
	case []any:
		if len(v) > 0 {
			return map[string]any{
				"type":  "ORDERED_LIST",
				"items": generateJSONSchema(v[0]),
			}
		}
		return map[string]any{
			"type":  "ORDERED_LIST",
			"items": map[string]any{"type": "UNKNOWN"},
		}
	default:
		return map[string]any{"type": "UNKNOWN"}
	}
}
