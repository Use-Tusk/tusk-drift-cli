package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateDeterministicHash_MapOrderInsensitive(t *testing.T) {
	v1 := map[string]any{"a": 1, "b": "x"}
	v2 := map[string]any{"b": "x", "a": 1}

	h1 := GenerateDeterministicHash(v1)
	h2 := GenerateDeterministicHash(v2)

	assert.Equal(t, h1, h2, "hash should be independent of map key order")
}

func TestGenerateDeterministicHash_SkipsNilValues(t *testing.T) {
	v1 := map[string]any{"a": 1, "b": nil}
	v2 := map[string]any{"a": 1}

	h1 := GenerateDeterministicHash(v1)
	h2 := GenerateDeterministicHash(v2)

	assert.Equal(t, h1, h2, "nil values should be ignored for hashing")
}

func TestGenerateDeterministicHash_NestedMapOrderInsensitive(t *testing.T) {
	v1 := map[string]any{
		"a": map[string]any{"x": 1, "y": 2},
		"b": []any{1, 2, 3},
	}
	v2 := map[string]any{
		"b": []any{1, 2, 3},
		"a": map[string]any{"y": 2, "x": 1},
	}

	h1 := GenerateDeterministicHash(v1)
	h2 := GenerateDeterministicHash(v2)

	assert.Equal(t, h1, h2, "nested map key order should not affect hash")
}

func TestGenerateDeterministicHash_ArrayOrderMatters(t *testing.T) {
	v1 := []any{1, 2, 3}
	v2 := []any{3, 2, 1}

	h1 := GenerateDeterministicHash(v1)
	h2 := GenerateDeterministicHash(v2)

	assert.NotEqual(t, h1, h2, "array order should affect hash")
}

func TestRemoveHeadersFromInputValue_PreservesCriticalOnly(t *testing.T) {
	in := map[string]any{
		"method": "GET",
		"headers": map[string]any{
			"accept":        "application/json",
			"content-type":  "application/json",
			"authorization": "Bearer token",
			"x-custom":      "value",
		},
		"path": "/users",
	}

	out := RemoveHeadersFromInputValue(in)
	outMap, ok := out.(map[string]any)
	require.True(t, ok, "expected map output")

	// Non-header fields preserved
	assert.Equal(t, "GET", outMap["method"])
	assert.Equal(t, "/users", outMap["path"])

	// Only accept/content-type preserved
	h, ok := outMap["headers"].(map[string]any)
	require.True(t, ok, "expected headers map to remain because critical keys exist")
	assert.Equal(t, "application/json", h["accept"])
	assert.Equal(t, "application/json", h["content-type"])
	_, hasAuth := h["authorization"]
	_, hasCustom := h["x-custom"]
	assert.False(t, hasAuth)
	assert.False(t, hasCustom)
}

func TestRemoveHeadersFromInputValue_DropsHeadersWhenNoCritical(t *testing.T) {
	in := map[string]any{
		"headers": map[string]any{
			"authorization": "Bearer token",
			"x-custom":      "v",
		},
	}

	out := RemoveHeadersFromInputValue(in)
	outMap, ok := out.(map[string]any)
	require.True(t, ok)

	_, hasHeaders := outMap["headers"]
	assert.False(t, hasHeaders, "headers key should be removed if no critical headers remain")
}

func TestRemoveHeadersFromInputValue_NonMapPassthrough(t *testing.T) {
	in := []any{1, "x"}
	out := RemoveHeadersFromInputValue(in)
	assert.Equal(t, in, out, "non-map inputs should be returned unchanged")
}

func TestGenerateSchemaAndHash_ObjectShapeAndHashes(t *testing.T) {
	value := map[string]any{
		"s": "str",
		"b": true,
		"n": float64(3),
	}

	schema, valueHash, schemaHash := GenerateSchemaAndHash(value)

	// Hashes align with GenerateDeterministicHash
	assert.Equal(t, GenerateDeterministicHash(value), valueHash)
	assert.Equal(t, GenerateDeterministicHash(schema), schemaHash)

	// Schema content sanity
	schemaMap, ok := schema.(map[string]any)
	require.True(t, ok, "schema should be a map")

	assert.Equal(t, "OBJECT", schemaMap["type"])

	props, ok := schemaMap["properties"].(map[string]any)
	require.True(t, ok, "schema.properties should be a map")

	getType := func(m any) string {
		if mm, ok := m.(map[string]any); ok {
			if tval, ok := mm["type"].(string); ok {
				return tval
			}
		}
		return ""
	}

	assert.Equal(t, "STRING", getType(props["s"]))
	assert.Equal(t, "BOOLEAN", getType(props["b"]))
	assert.Equal(t, "NUMBER", getType(props["n"]))
}

func TestGenerateSchemaAndHash_ListItems(t *testing.T) {
	value := []any{"a", "b"}
	schema, _, _ := GenerateSchemaAndHash(value)

	schemaMap, ok := schema.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "ORDERED_LIST", schemaMap["type"])

	items, ok := schemaMap["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "STRING", items["type"])
}
