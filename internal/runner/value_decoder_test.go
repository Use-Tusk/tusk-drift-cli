package runner

import (
	"encoding/base64"
	"testing"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/require"
)

// TestDecodeValueBySchema tests the value decoding functionality with full schemas
func TestDecodeValueBySchema(t *testing.T) {
	t.Parallel()

	jsonData := `{"key": "value"}`
	base64JSON := base64.StdEncoding.EncodeToString([]byte(jsonData))

	htmlData := "<html><body>Hello</body></html>"
	base64HTML := base64.StdEncoding.EncodeToString([]byte(htmlData))

	tests := []struct {
		name            string
		value           any
		schema          *core.JsonSchema
		wantBytes       []byte
		wantParsed      any
		wantErr         bool
		errContains     string
		skipParsedCheck bool // For cases where parsing might vary
	}{
		{
			name:       "nil value",
			value:      nil,
			schema:     nil,
			wantBytes:  nil,
			wantParsed: nil,
			wantErr:    false,
		},
		{
			name:        "non-string value",
			value:       123,
			schema:      nil,
			wantErr:     true,
			errContains: "expected value to be a string",
		},
		{
			name:      "nil schema defaults to base64",
			value:     base64JSON,
			schema:    nil,
			wantBytes: []byte(jsonData),
			wantParsed: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name:  "empty schema defaults to base64",
			value: base64JSON,
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
			},
			wantBytes: []byte(jsonData),
			wantParsed: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name:  "explicit base64 encoding with JSON type",
			value: base64JSON,
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
			},
			wantBytes: []byte(jsonData),
			wantParsed: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name:  "UNSPECIFIED encoding with plain text",
			value: "plain text content",
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_PLAIN_TEXT.Enum(),
			},
			wantBytes:  []byte("plain text content"),
			wantParsed: "plain text content",
			wantErr:    false,
		},
		{
			name:  "invalid base64 with BASE64 encoding",
			value: "not-valid-base64!!!",
			schema: &core.JsonSchema{
				Type:     core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding: core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
			},
			wantErr:     true,
			errContains: "failed to decode base64 value",
		},
		{
			name:  "HTML with base64 encoding",
			value: base64HTML,
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_HTML.Enum(),
			},
			wantBytes:  []byte(htmlData),
			wantParsed: htmlData,
			wantErr:    false,
		},
		{
			name:  "unknown encoding falls back gracefully",
			value: base64JSON,
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType(999).Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
			},
			wantBytes: []byte(jsonData),
			wantParsed: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name:  "JSON array",
			value: base64.StdEncoding.EncodeToString([]byte(`[1, 2, 3]`)),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
			},
			wantBytes:  []byte(`[1, 2, 3]`),
			wantParsed: []any{float64(1), float64(2), float64(3)},
			wantErr:    false,
		},
		{
			name:  "invalid JSON falls back to string",
			value: base64.StdEncoding.EncodeToString([]byte(`{invalid json`)),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
			},
			wantBytes:  []byte(`{invalid json`),
			wantParsed: `{invalid json`,
			wantErr:    false,
		},
		{
			name:  "CSS text format",
			value: base64.StdEncoding.EncodeToString([]byte("body { color: red; }")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_CSS.Enum(),
			},
			wantBytes:  []byte("body { color: red; }"),
			wantParsed: "body { color: red; }",
			wantErr:    false,
		},
		{
			name:  "JavaScript text format",
			value: base64.StdEncoding.EncodeToString([]byte("function test() { return true; }")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JAVASCRIPT.Enum(),
			},
			wantBytes:  []byte("function test() { return true; }"),
			wantParsed: "function test() { return true; }",
			wantErr:    false,
		},
		{
			name:  "XML text format",
			value: base64.StdEncoding.EncodeToString([]byte("<root><item>value</item></root>")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_XML.Enum(),
			},
			wantBytes:  []byte("<root><item>value</item></root>"),
			wantParsed: "<root><item>value</item></root>",
			wantErr:    false,
		},
		{
			name:  "YAML text format",
			value: base64.StdEncoding.EncodeToString([]byte("key: value\narray:\n  - item1\n  - item2")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_YAML.Enum(),
			},
			wantBytes:  []byte("key: value\narray:\n  - item1\n  - item2"),
			wantParsed: "key: value\narray:\n  - item1\n  - item2",
			wantErr:    false,
		},
		{
			name:  "Markdown text format",
			value: base64.StdEncoding.EncodeToString([]byte("# Header\n\nParagraph")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_MARKDOWN.Enum(),
			},
			wantBytes:  []byte("# Header\n\nParagraph"),
			wantParsed: "# Header\n\nParagraph",
			wantErr:    false,
		},
		{
			name:  "CSV text format",
			value: base64.StdEncoding.EncodeToString([]byte("name,age\nAlice,30\nBob,25")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_CSV.Enum(),
			},
			wantBytes:  []byte("name,age\nAlice,30\nBob,25"),
			wantParsed: "name,age\nAlice,30\nBob,25",
			wantErr:    false,
		},
		{
			name:  "SQL text format",
			value: base64.StdEncoding.EncodeToString([]byte("SELECT * FROM users WHERE id = 1")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_SQL.Enum(),
			},
			wantBytes:  []byte("SELECT * FROM users WHERE id = 1"),
			wantParsed: "SELECT * FROM users WHERE id = 1",
			wantErr:    false,
		},
		{
			name:  "GraphQL text format",
			value: base64.StdEncoding.EncodeToString([]byte("query { user(id: 1) { name } }")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_GRAPHQL.Enum(),
			},
			wantBytes:  []byte("query { user(id: 1) { name } }"),
			wantParsed: "query { user(id: 1) { name } }",
			wantErr:    false,
		},
		{
			name:  "SVG text format",
			value: base64.StdEncoding.EncodeToString([]byte("<svg><circle cx=\"50\" cy=\"50\" r=\"40\"/></svg>")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_SVG.Enum(),
			},
			wantBytes:  []byte("<svg><circle cx=\"50\" cy=\"50\" r=\"40\"/></svg>"),
			wantParsed: "<svg><circle cx=\"50\" cy=\"50\" r=\"40\"/></svg>",
			wantErr:    false,
		},
		{
			name:  "Form data format",
			value: base64.StdEncoding.EncodeToString([]byte("username=alice&password=secret")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_FORM_DATA.Enum(),
			},
			wantBytes:  []byte("username=alice&password=secret"),
			wantParsed: "username=alice&password=secret",
			wantErr:    false,
		},
		{
			name:  "Binary PNG - returns base64 for comparison",
			value: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_PNG.Enum(),
			},
			wantBytes: func() []byte {
				b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
				return b
			}(),
			wantParsed:      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			skipParsedCheck: false,
			wantErr:         false,
		},
		{
			name:  "Binary JPEG - returns base64 for comparison",
			value: "/9j/4AAQSkZJRg==",
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JPEG.Enum(),
			},
			wantBytes:       func() []byte { b, _ := base64.StdEncoding.DecodeString("/9j/4AAQSkZJRg=="); return b }(),
			wantParsed:      "/9j/4AAQSkZJRg==",
			skipParsedCheck: false,
			wantErr:         false,
		},
		{
			name:  "Binary PDF - returns base64 for comparison",
			value: base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_PDF.Enum(),
			},
			wantBytes:  []byte("%PDF-1.4"),
			wantParsed: base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")),
			wantErr:    false,
		},
		{
			name:  "UNSPECIFIED decoded type tries JSON first",
			value: base64JSON,
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_UNSPECIFIED.Enum(),
			},
			wantBytes: []byte(jsonData),
			wantParsed: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name:  "UNSPECIFIED decoded type falls back to string",
			value: base64.StdEncoding.EncodeToString([]byte("not json")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_UNSPECIFIED.Enum(),
			},
			wantBytes:  []byte("not json"),
			wantParsed: "not json",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotBytes, gotParsed, err := DecodeValueBySchema(tt.value, tt.schema)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantBytes, gotBytes)
			if !tt.skipParsedCheck {
				require.Equal(t, tt.wantParsed, gotParsed)
			}
		})
	}
}

// TestDecodeValueBySchemaIntegration tests the full decode and parse flow with real-world scenarios
func TestDecodeValueBySchemaIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		value      any
		schema     *core.JsonSchema
		wantParsed any
		wantErr    bool
	}{
		{
			name:  "JSON object end-to-end",
			value: base64.StdEncoding.EncodeToString([]byte(`{"user": {"id": 123, "name": "Alice"}}`)),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
			},
			wantParsed: map[string]any{
				"user": map[string]any{
					"id":   float64(123),
					"name": "Alice",
				},
			},
			wantErr: false,
		},
		{
			name:  "Plain text end-to-end",
			value: "Hello, World!",
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_PLAIN_TEXT.Enum(),
			},
			wantParsed: "Hello, World!",
			wantErr:    false,
		},
		{
			name:  "HTML end-to-end",
			value: base64.StdEncoding.EncodeToString([]byte("<html><head><title>Test</title></head></html>")),
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_HTML.Enum(),
			},
			wantParsed: "<html><head><title>Test</title></head></html>",
			wantErr:    false,
		},
		{
			name:  "Binary (PNG image) end-to-end",
			value: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			schema: &core.JsonSchema{
				Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
				DecodedType: core.DecodedType_DECODED_TYPE_PNG.Enum(),
			},
			wantParsed: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			wantErr:    false,
		},
		{
			name:   "Nil schema with JSON data",
			value:  base64.StdEncoding.EncodeToString([]byte(`{"status": "ok"}`)),
			schema: nil,
			wantParsed: map[string]any{
				"status": "ok",
			},
			wantErr: false,
		},
		{
			name:  "Schema without encoding/decodedType specified",
			value: base64.StdEncoding.EncodeToString([]byte(`[1, 2, 3]`)),
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
			},
			wantParsed: []any{float64(1), float64(2), float64(3)},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, parsed, err := DecodeValueBySchema(tt.value, tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantParsed, parsed)
		})
	}
}
