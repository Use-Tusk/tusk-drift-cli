package utils

import (
	"testing"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/require"
)

// TestReduceByMatchImportance tests the value reduction based on matchImportance
func TestReduceByMatchImportance(t *testing.T) {
	t.Parallel()

	matchImportanceZero := 0.0
	matchImportanceOne := 1.0

	tests := []struct {
		name   string
		value  any
		schema *core.JsonSchema
		want   any
	}{
		{
			name:   "nil value returns nil",
			value:  nil,
			schema: &core.JsonSchema{Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT},
			want:   nil,
		},
		{
			name:   "nil schema returns value unchanged",
			value:  map[string]any{"key": "value"},
			schema: nil,
			want:   map[string]any{"key": "value"},
		},
		{
			name:   "both nil returns nil",
			value:  nil,
			schema: nil,
			want:   nil,
		},
		{
			name:  "empty object with schema",
			value: map[string]any{},
			schema: &core.JsonSchema{
				Type:       core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{},
			},
			want: map[string]any{},
		},
		{
			name: "object with all important fields (default)",
			value: map[string]any{
				"name":  "Alice",
				"email": "alice@example.com",
				"age":   30,
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"name":  {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
					"email": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
					"age":   {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
				},
			},
			want: map[string]any{
				"name":  "Alice",
				"email": "alice@example.com",
				"age":   30,
			},
		},
		{
			name: "object with mixed importance - removes low importance fields",
			value: map[string]any{
				"userId":    "user-123",
				"authToken": "secret-token",
				"timestamp": "2025-01-01T00:00:00Z",
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"userId":    {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
					"authToken": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
					"timestamp": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
				},
			},
			want: map[string]any{
				"userId": "user-123",
			},
		},
		{
			name: "object with explicit importance=1 keeps field",
			value: map[string]any{
				"important": "keep-me",
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"important": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceOne},
				},
			},
			want: map[string]any{
				"important": "keep-me",
			},
		},
		{
			name: "nested objects with mixed importance at different levels",
			value: map[string]any{
				"user": map[string]any{
					"id":       "user-456",
					"password": "secret",
					"profile": map[string]any{
						"name":      "Bob",
						"sessionId": "session-789",
					},
				},
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"user": {
						Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
						Properties: map[string]*core.JsonSchema{
							"id":       {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
							"password": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
							"profile": {
								Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
								Properties: map[string]*core.JsonSchema{
									"name":      {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
									"sessionId": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"user": map[string]any{
					"id": "user-456",
					"profile": map[string]any{
						"name": "Bob",
					},
				},
			},
		},
		{
			name: "array with important items (default)",
			value: []any{
				"item1",
				"item2",
				"item3",
			},
			schema: &core.JsonSchema{
				Type:  core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
				Items: &core.JsonSchema{Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
			},
			want: []any{
				"item1",
				"item2",
				"item3",
			},
		},
		{
			name: "array with low-importance items returns empty array",
			value: []any{
				"unimportant1",
				"unimportant2",
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
				Items: &core.JsonSchema{
					Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
					MatchImportance: &matchImportanceZero,
				},
			},
			want: []any{},
		},
		{
			name: "array of objects with mixed field importance",
			value: []any{
				map[string]any{
					"id":        1,
					"name":      "Product A",
					"tempField": "temp1",
				},
				map[string]any{
					"id":        2,
					"name":      "Product B",
					"tempField": "temp2",
				},
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
				Items: &core.JsonSchema{
					Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
					Properties: map[string]*core.JsonSchema{
						"id":        {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
						"name":      {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
						"tempField": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
					},
				},
			},
			want: []any{
				map[string]any{
					"id":   1,
					"name": "Product A",
				},
				map[string]any{
					"id":   2,
					"name": "Product B",
				},
			},
		},
		{
			name:  "primitive string with no schema keeps value",
			value: "simple string",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
			},
			want: "simple string",
		},
		{
			name:  "primitive with low importance returns nil",
			value: "drop this",
			schema: &core.JsonSchema{
				Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				MatchImportance: &matchImportanceZero,
			},
			want: nil,
		},
		{
			name:  "number primitive with default importance",
			value: 42.5,
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER,
			},
			want: 42.5,
		},
		{
			name:  "boolean primitive with default importance",
			value: true,
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_BOOLEAN,
			},
			want: true,
		},
		{
			name: "deep nesting beyond 3 levels",
			value: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"level4": map[string]any{
								"important": "keep",
								"drop":      "this",
							},
						},
					},
				},
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"level1": {
						Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
						Properties: map[string]*core.JsonSchema{
							"level2": {
								Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
								Properties: map[string]*core.JsonSchema{
									"level3": {
										Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
										Properties: map[string]*core.JsonSchema{
											"level4": {
												Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
												Properties: map[string]*core.JsonSchema{
													"important": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
													"drop":      {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"level4": map[string]any{
								"important": "keep",
							},
						},
					},
				},
			},
		},
		{
			name: "schema without properties map - field not in schema kept by default",
			value: map[string]any{
				"field1": "value1",
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
			},
			want: map[string]any{
				"field1": "value1",
			},
		},
		{
			name: "value field not in schema properties kept by default",
			value: map[string]any{
				"knownField":   "keep",
				"unknownField": "alsoKeep",
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"knownField": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
				},
			},
			want: map[string]any{
				"knownField":   "keep",
				"unknownField": "alsoKeep",
			},
		},
		{
			name:  "empty array with schema",
			value: []any{},
			schema: &core.JsonSchema{
				Type:  core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
				Items: &core.JsonSchema{Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
			},
			want: []any{},
		},
		{
			name: "nested array with objects",
			value: []any{
				[]any{
					map[string]any{"id": 1, "temp": "x"},
					map[string]any{"id": 2, "temp": "y"},
				},
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
				Items: &core.JsonSchema{
					Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
					Items: &core.JsonSchema{
						Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
						Properties: map[string]*core.JsonSchema{
							"id":   {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
							"temp": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
						},
					},
				},
			},
			want: []any{
				[]any{
					map[string]any{"id": 1},
					map[string]any{"id": 2},
				},
			},
		},
		{
			name: "object with all fields having zero importance",
			value: map[string]any{
				"drop1": "value1",
				"drop2": "value2",
			},
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"drop1": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
					"drop2": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
				},
			},
			want: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ReduceByMatchImportance(tt.value, tt.schema)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestReduceSchemaByMatchImportance tests the schema reduction based on matchImportance
func TestReduceSchemaByMatchImportance(t *testing.T) {
	t.Parallel()

	matchImportanceZero := 0.0
	matchImportanceOne := 1.0

	tests := []struct {
		name   string
		schema *core.JsonSchema
		want   *core.JsonSchema
	}{
		{
			name:   "nil schema returns nil",
			schema: nil,
			want:   nil,
		},
		{
			name: "schema with default importance keeps all fields",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"field1": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
					"field2": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
				},
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"field1": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
					"field2": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
				},
			},
		},
		{
			name: "schema with low importance root returns nil",
			schema: &core.JsonSchema{
				Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				MatchImportance: &matchImportanceZero,
				Properties: map[string]*core.JsonSchema{
					"field": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
				},
			},
			want: nil,
		},
		{
			name: "schema with mixed field importance removes low-importance properties",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"important":   {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
					"unimportant": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
					"alsoKeep":    {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER, MatchImportance: &matchImportanceOne},
				},
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"important": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
					"alsoKeep":  {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER, MatchImportance: &matchImportanceOne},
				},
			},
		},
		{
			name: "nested schema with mixed importance at different levels",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"outer": {
						Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
						Properties: map[string]*core.JsonSchema{
							"keep": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
							"drop": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
							"inner": {
								Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
								Properties: map[string]*core.JsonSchema{
									"deepKeep": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
									"deepDrop": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER, MatchImportance: &matchImportanceZero},
								},
							},
						},
					},
					"dropTop": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
				},
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"outer": {
						Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
						Properties: map[string]*core.JsonSchema{
							"keep": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
							"inner": {
								Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
								Properties: map[string]*core.JsonSchema{
									"deepKeep": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "schema with items (array) keeps important items",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
				Items: &core.JsonSchema{
					Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
					Properties: map[string]*core.JsonSchema{
						"id":   {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
						"temp": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
					},
				},
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
				Items: &core.JsonSchema{
					Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
					Properties: map[string]*core.JsonSchema{
						"id": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
					},
				},
			},
		},
		{
			name: "schema with low-importance items field returns schema without items",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
				Items: &core.JsonSchema{
					Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
					MatchImportance: &matchImportanceZero,
				},
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
			},
		},
		{
			name: "complex nested schema with arrays and objects",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"users": {
						Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
						Items: &core.JsonSchema{
							Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
							Properties: map[string]*core.JsonSchema{
								"id":       {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
								"name":     {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
								"password": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
								"tags": {
									Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
									Items: &core.JsonSchema{
										Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
									},
								},
							},
						},
					},
					"metadata": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT, MatchImportance: &matchImportanceZero},
				},
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"users": {
						Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
						Items: &core.JsonSchema{
							Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
							Properties: map[string]*core.JsonSchema{
								"id":   {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
								"name": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
								"tags": {
									Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_ORDERED_LIST,
									Items: &core.JsonSchema{
										Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "schema with empty properties map",
			schema: &core.JsonSchema{
				Type:       core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{},
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				// Properties field will be nil, not an empty map, after reduction
			},
		},
		{
			name: "schema with all properties having zero importance",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"drop1": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceZero},
					"drop2": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER, MatchImportance: &matchImportanceZero},
				},
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				// Properties field will be an empty map when all properties are dropped
				Properties: map[string]*core.JsonSchema{},
			},
		},
		{
			name: "primitive schema with default importance",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
			},
			want: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
			},
		},
		{
			name: "schema with encoding and decoded type preserved",
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
			},
			want: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
			},
		},
		{
			name: "schema with match importance preserved on kept fields",
			schema: &core.JsonSchema{
				Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				MatchImportance: &matchImportanceOne,
				Properties: map[string]*core.JsonSchema{
					"field": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceOne},
				},
			},
			want: &core.JsonSchema{
				Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				MatchImportance: &matchImportanceOne,
				Properties: map[string]*core.JsonSchema{
					"field": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING, MatchImportance: &matchImportanceOne},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ReduceSchemaByMatchImportance(tt.schema)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestGetImportance tests the getImportance helper function
func TestGetImportance(t *testing.T) {
	t.Parallel()

	matchImportanceZero := 0.0
	matchImportanceOne := 1.0
	matchImportanceHalf := 0.5

	tests := []struct {
		name   string
		schema *core.JsonSchema
		want   int
	}{
		{
			name:   "nil schema returns 1 (important by default)",
			schema: nil,
			want:   1,
		},
		{
			name: "schema without matchImportance returns 1 (important by default)",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
			},
			want: 1,
		},
		{
			name: "schema with matchImportance=0 returns 0",
			schema: &core.JsonSchema{
				Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				MatchImportance: &matchImportanceZero,
			},
			want: 0,
		},
		{
			name: "schema with matchImportance=1 returns 1",
			schema: &core.JsonSchema{
				Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				MatchImportance: &matchImportanceOne,
			},
			want: 1,
		},
		{
			name: "schema with matchImportance=0.5 returns 1 (any non-zero is important)",
			schema: &core.JsonSchema{
				Type:            core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				MatchImportance: &matchImportanceHalf,
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := getImportance(tt.schema)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestGetFieldSchema tests the getFieldSchema helper function
func TestGetFieldSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		schema    *core.JsonSchema
		fieldName string
		want      *core.JsonSchema
	}{
		{
			name:      "nil schema returns nil",
			schema:    nil,
			fieldName: "field",
			want:      nil,
		},
		{
			name: "schema without properties returns nil",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
			},
			fieldName: "field",
			want:      nil,
		},
		{
			name: "schema with empty properties returns nil for any field",
			schema: &core.JsonSchema{
				Type:       core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{},
			},
			fieldName: "field",
			want:      nil,
		},
		{
			name: "returns field schema when present",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"name": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
					"age":  {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_NUMBER},
				},
			},
			fieldName: "name",
			want:      &core.JsonSchema{Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
		},
		{
			name: "returns nil when field not in properties",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"name": {Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING},
				},
			},
			fieldName: "nonexistent",
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := getFieldSchema(tt.schema, tt.fieldName)
			require.Equal(t, tt.want, got)
		})
	}
}
