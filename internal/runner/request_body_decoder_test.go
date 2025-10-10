package runner

import (
	"encoding/base64"
	"testing"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"github.com/stretchr/testify/require"
)

// TestExtractRequestBodyMetadata tests the extraction of metadata from schemas
func TestExtractRequestBodyMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		schema    *core.JsonSchema
		fieldPath string
		want      *RequestBodyMetadata
	}{
		{
			name:      "nil schema",
			schema:    nil,
			fieldPath: "body",
			want:      nil,
		},
		{
			name: "empty schema",
			schema: &core.JsonSchema{
				Type:       core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{},
			},
			fieldPath: "body",
			want:      nil,
		},
		{
			name: "schema without properties",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
			},
			fieldPath: "body",
			want:      nil,
		},
		{
			name: "schema with properties but missing field",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"otherField": {
						Type:       core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
						Properties: map[string]*core.JsonSchema{},
					},
				},
			},
			fieldPath: "body",
			want:      nil,
		},
		{
			name: "schema with field but no metadata",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"body": {
						Type:       core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
						Properties: map[string]*core.JsonSchema{},
					},
				},
			},
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED,
				DecodedType: core.DecodedType_DECODED_TYPE_UNSPECIFIED,
			},
		},
		{
			name: "schema with base64 encoding and JSON type",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"body": {
						Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
						Properties:  map[string]*core.JsonSchema{},
						Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
						DecodedType: core.DecodedType_DECODED_TYPE_JSON.Enum(),
					},
				},
			},
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_JSON,
			},
		},
		{
			name: "schema with UNSPECIFIED encoding and HTML type",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"body": {
						Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
						Properties:  map[string]*core.JsonSchema{},
						Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED.Enum(),
						DecodedType: core.DecodedType_DECODED_TYPE_HTML.Enum(),
					},
				},
			},
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED,
				DecodedType: core.DecodedType_DECODED_TYPE_HTML,
			},
		},
		{
			name: "schema with only encoding",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"body": {
						Type:       core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
						Properties: map[string]*core.JsonSchema{},
						Encoding:   core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
					},
				},
			},
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_UNSPECIFIED,
			},
		},
		{
			name: "schema with only decodedType",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"body": {
						Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
						Properties:  map[string]*core.JsonSchema{},
						DecodedType: core.DecodedType_DECODED_TYPE_XML.Enum(),
					},
				},
			},
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED,
				DecodedType: core.DecodedType_DECODED_TYPE_XML,
			},
		},
		{
			name: "schema with binary types",
			schema: &core.JsonSchema{
				Type: core.JsonSchemaType_JSON_SCHEMA_TYPE_OBJECT,
				Properties: map[string]*core.JsonSchema{
					"image": {
						Type:        core.JsonSchemaType_JSON_SCHEMA_TYPE_STRING,
						Properties:  map[string]*core.JsonSchema{},
						Encoding:    core.EncodingType_ENCODING_TYPE_BASE64.Enum(),
						DecodedType: core.DecodedType_DECODED_TYPE_PNG.Enum(),
					},
				},
			},
			fieldPath: "image",
			want: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_PNG,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractRequestBodyMetadata(tt.schema, tt.fieldPath)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestDecodeBody tests the body decoding functionality
func TestDecodeBody(t *testing.T) {
	t.Parallel()

	jsonData := `{"key": "value"}`
	base64JSON := base64.StdEncoding.EncodeToString([]byte(jsonData))

	htmlData := "<html><body>Hello</body></html>"
	base64HTML := base64.StdEncoding.EncodeToString([]byte(htmlData))

	tests := []struct {
		name        string
		bodyValue   any
		metadata    *RequestBodyMetadata
		wantBytes   []byte
		wantType    core.DecodedType
		wantErr     bool
		errContains string
	}{
		{
			name:        "non-string body value",
			bodyValue:   123,
			metadata:    nil,
			wantErr:     true,
			errContains: "expected body to be a string",
		},
		{
			name:      "nil metadata defaults to base64",
			bodyValue: base64JSON,
			metadata:  nil,
			wantBytes: []byte(jsonData),
			wantType:  core.DecodedType_DECODED_TYPE_UNSPECIFIED,
			wantErr:   false,
		},
		{
			name:      "empty metadata defaults to base64",
			bodyValue: base64JSON,
			metadata:  &RequestBodyMetadata{},
			wantBytes: []byte(jsonData),
			wantType:  core.DecodedType_DECODED_TYPE_UNSPECIFIED,
			wantErr:   false,
		},
		{
			name:      "explicit base64 encoding with JSON type",
			bodyValue: base64JSON,
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_JSON,
			},
			wantBytes: []byte(jsonData),
			wantType:  core.DecodedType_DECODED_TYPE_JSON,
			wantErr:   false,
		},
		{
			name:      "NONE encoding with plain text",
			bodyValue: "plain text content",
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED,
				DecodedType: core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			},
			wantBytes: []byte("plain text content"),
			wantType:  core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			wantErr:   false,
		},
		{
			name:      "invalid base64 with BASE64 encoding",
			bodyValue: "not-valid-base64!!!",
			metadata: &RequestBodyMetadata{
				Encoding: core.EncodingType_ENCODING_TYPE_BASE64,
			},
			wantErr:     true,
			errContains: "failed to decode base64 body",
		},
		{
			name:      "HTML with base64 encoding",
			bodyValue: base64HTML,
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_HTML,
			},
			wantBytes: []byte(htmlData),
			wantType:  core.DecodedType_DECODED_TYPE_HTML,
			wantErr:   false,
		},
		{
			name:      "unknown encoding falls back gracefully",
			bodyValue: base64JSON,
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType(999),
				DecodedType: core.DecodedType_DECODED_TYPE_JSON,
			},
			wantBytes: []byte(jsonData), // Should successfully decode as base64
			wantType:  core.DecodedType_DECODED_TYPE_JSON,
			wantErr:   false,
		},
		{
			name:      "unknown encoding with invalid base64 falls back to raw",
			bodyValue: "not base64",
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType(999),
				DecodedType: core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			},
			wantBytes: []byte("not base64"),
			wantType:  core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			wantErr:   false,
		},
		{
			name:      "empty string body",
			bodyValue: "",
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED,
				DecodedType: core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			},
			wantBytes: []byte(""),
			wantType:  core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			wantErr:   false,
		},
		{
			name:      "binary data (PNG) with base64",
			bodyValue: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_PNG,
			},
			wantBytes: func() []byte {
				b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
				return b
			}(),
			wantType: core.DecodedType_DECODED_TYPE_PNG,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotBytes, gotType, err := DecodeBody(tt.bodyValue, tt.metadata)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantBytes, gotBytes)
			require.Equal(t, tt.wantType, gotType)
		})
	}
}

// TestParseBodyForComparison tests the body parsing for comparison
func TestParseBodyForComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		decodedBytes []byte
		decodedType  core.DecodedType
		want         any
		wantErr      bool
	}{
		{
			name:         "JSON valid object",
			decodedBytes: []byte(`{"key": "value", "num": 42}`),
			decodedType:  core.DecodedType_DECODED_TYPE_JSON,
			want: map[string]any{
				"key": "value",
				"num": float64(42), // JSON numbers are float64
			},
			wantErr: false,
		},
		{
			name:         "JSON valid array",
			decodedBytes: []byte(`[1, 2, 3]`),
			decodedType:  core.DecodedType_DECODED_TYPE_JSON,
			want:         []any{float64(1), float64(2), float64(3)},
			wantErr:      false,
		},
		{
			name:         "JSON invalid falls back to string",
			decodedBytes: []byte(`{invalid json`),
			decodedType:  core.DecodedType_DECODED_TYPE_JSON,
			want:         "{invalid json",
			wantErr:      false,
		},
		{
			name:         "PLAIN_TEXT",
			decodedBytes: []byte("plain text content"),
			decodedType:  core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			want:         "plain text content",
			wantErr:      false,
		},
		{
			name:         "HTML",
			decodedBytes: []byte("<html><body>Hello</body></html>"),
			decodedType:  core.DecodedType_DECODED_TYPE_HTML,
			want:         "<html><body>Hello</body></html>",
			wantErr:      false,
		},
		{
			name:         "CSS",
			decodedBytes: []byte("body { color: red; }"),
			decodedType:  core.DecodedType_DECODED_TYPE_CSS,
			want:         "body { color: red; }",
			wantErr:      false,
		},
		{
			name:         "JAVASCRIPT",
			decodedBytes: []byte("function test() { return true; }"),
			decodedType:  core.DecodedType_DECODED_TYPE_JAVASCRIPT,
			want:         "function test() { return true; }",
			wantErr:      false,
		},
		{
			name:         "XML",
			decodedBytes: []byte("<root><item>value</item></root>"),
			decodedType:  core.DecodedType_DECODED_TYPE_XML,
			want:         "<root><item>value</item></root>",
			wantErr:      false,
		},
		{
			name:         "YAML",
			decodedBytes: []byte("key: value\narray:\n  - item1\n  - item2"),
			decodedType:  core.DecodedType_DECODED_TYPE_YAML,
			want:         "key: value\narray:\n  - item1\n  - item2",
			wantErr:      false,
		},
		{
			name:         "MARKDOWN",
			decodedBytes: []byte("# Header\n\nThis is **bold**"),
			decodedType:  core.DecodedType_DECODED_TYPE_MARKDOWN,
			want:         "# Header\n\nThis is **bold**",
			wantErr:      false,
		},
		{
			name:         "CSV",
			decodedBytes: []byte("name,age\nAlice,30\nBob,25"),
			decodedType:  core.DecodedType_DECODED_TYPE_CSV,
			want:         "name,age\nAlice,30\nBob,25",
			wantErr:      false,
		},
		{
			name:         "SQL",
			decodedBytes: []byte("SELECT * FROM users WHERE id = 1"),
			decodedType:  core.DecodedType_DECODED_TYPE_SQL,
			want:         "SELECT * FROM users WHERE id = 1",
			wantErr:      false,
		},
		{
			name:         "GRAPHQL",
			decodedBytes: []byte("query { user(id: 1) { name } }"),
			decodedType:  core.DecodedType_DECODED_TYPE_GRAPHQL,
			want:         "query { user(id: 1) { name } }",
			wantErr:      false,
		},
		{
			name:         "SVG",
			decodedBytes: []byte(`<svg><circle cx="50" cy="50" r="40"/></svg>`),
			decodedType:  core.DecodedType_DECODED_TYPE_SVG,
			want:         `<svg><circle cx="50" cy="50" r="40"/></svg>`,
			wantErr:      false,
		},
		{
			name:         "FORM_DATA",
			decodedBytes: []byte("key1=value1&key2=value2"),
			decodedType:  core.DecodedType_DECODED_TYPE_FORM_DATA,
			want:         "key1=value1&key2=value2",
			wantErr:      false,
		},
		{
			name:         "MULTIPART_FORM",
			decodedBytes: []byte("--boundary\r\nContent-Disposition: form-data; name=\"field\"\r\n\r\nvalue\r\n--boundary--"),
			decodedType:  core.DecodedType_DECODED_TYPE_MULTIPART_FORM,
			want:         "--boundary\r\nContent-Disposition: form-data; name=\"field\"\r\n\r\nvalue\r\n--boundary--",
			wantErr:      false,
		},
		{
			name:         "BINARY returns base64",
			decodedBytes: []byte{0x00, 0x01, 0x02, 0xff},
			decodedType:  core.DecodedType_DECODED_TYPE_BINARY,
			want:         base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0x02, 0xff}),
			wantErr:      false,
		},
		{
			name:         "PDF returns base64",
			decodedBytes: []byte("%PDF-1.4"),
			decodedType:  core.DecodedType_DECODED_TYPE_PDF,
			want:         base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")),
			wantErr:      false,
		},
		{
			name:         "JPEG returns base64",
			decodedBytes: []byte{0xff, 0xd8, 0xff, 0xe0},
			decodedType:  core.DecodedType_DECODED_TYPE_JPEG,
			want:         base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xe0}),
			wantErr:      false,
		},
		{
			name:         "PNG returns base64",
			decodedBytes: []byte{0x89, 0x50, 0x4e, 0x47},
			decodedType:  core.DecodedType_DECODED_TYPE_PNG,
			want:         base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47}),
			wantErr:      false,
		},
		{
			name:         "GIF returns base64",
			decodedBytes: []byte("GIF89a"),
			decodedType:  core.DecodedType_DECODED_TYPE_GIF,
			want:         base64.StdEncoding.EncodeToString([]byte("GIF89a")),
			wantErr:      false,
		},
		{
			name:         "WEBP returns base64",
			decodedBytes: []byte("RIFF"),
			decodedType:  core.DecodedType_DECODED_TYPE_WEBP,
			want:         base64.StdEncoding.EncodeToString([]byte("RIFF")),
			wantErr:      false,
		},
		{
			name:         "AUDIO returns base64",
			decodedBytes: []byte{0x49, 0x44, 0x33}, // MP3 ID3 tag
			decodedType:  core.DecodedType_DECODED_TYPE_AUDIO,
			want:         base64.StdEncoding.EncodeToString([]byte{0x49, 0x44, 0x33}),
			wantErr:      false,
		},
		{
			name:         "VIDEO returns base64",
			decodedBytes: []byte("ftyp"),
			decodedType:  core.DecodedType_DECODED_TYPE_VIDEO,
			want:         base64.StdEncoding.EncodeToString([]byte("ftyp")),
			wantErr:      false,
		},
		{
			name:         "GZIP returns base64",
			decodedBytes: []byte{0x1f, 0x8b},
			decodedType:  core.DecodedType_DECODED_TYPE_GZIP,
			want:         base64.StdEncoding.EncodeToString([]byte{0x1f, 0x8b}),
			wantErr:      false,
		},
		{
			name:         "ZIP returns base64",
			decodedBytes: []byte{0x50, 0x4b, 0x03, 0x04},
			decodedType:  core.DecodedType_DECODED_TYPE_ZIP,
			want:         base64.StdEncoding.EncodeToString([]byte{0x50, 0x4b, 0x03, 0x04}),
			wantErr:      false,
		},
		{
			name:         "UNSPECIFIED with valid JSON tries JSON",
			decodedBytes: []byte(`{"test": true}`),
			decodedType:  core.DecodedType_DECODED_TYPE_UNSPECIFIED,
			want: map[string]any{
				"test": true,
			},
			wantErr: false,
		},
		{
			name:         "UNSPECIFIED with non-JSON falls back to string",
			decodedBytes: []byte("just plain text"),
			decodedType:  core.DecodedType_DECODED_TYPE_UNSPECIFIED,
			want:         "just plain text",
			wantErr:      false,
		},
		{
			name:         "unknown type with valid JSON tries JSON",
			decodedBytes: []byte(`[1, 2, 3]`),
			decodedType:  core.DecodedType(999),
			want:         []any{float64(1), float64(2), float64(3)},
			wantErr:      false,
		},
		{
			name:         "unknown type with non-JSON falls back to string",
			decodedBytes: []byte("some text"),
			decodedType:  core.DecodedType(999),
			want:         "some text",
			wantErr:      false,
		},
		{
			name:         "empty bytes with JSON type",
			decodedBytes: []byte(""),
			decodedType:  core.DecodedType_DECODED_TYPE_JSON,
			want:         "",
			wantErr:      false,
		},
		{
			name:         "empty bytes with PLAIN_TEXT type",
			decodedBytes: []byte(""),
			decodedType:  core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			want:         "",
			wantErr:      false,
		},
		{
			name:         "empty bytes with BINARY type",
			decodedBytes: []byte(""),
			decodedType:  core.DecodedType_DECODED_TYPE_BINARY,
			want:         "",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseBodyForComparison(tt.decodedBytes, tt.decodedType)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestDecodeBodyIntegration tests the full decode and parse flow
func TestDecodeBodyIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		bodyValue  any
		metadata   *RequestBodyMetadata
		wantParsed any
		wantErr    bool
	}{
		{
			name:      "JSON object end-to-end",
			bodyValue: base64.StdEncoding.EncodeToString([]byte(`{"user": {"id": 123, "name": "Alice"}}`)),
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_JSON,
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
			name:      "Plain text end-to-end",
			bodyValue: "Hello, World!",
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED,
				DecodedType: core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
			},
			wantParsed: "Hello, World!",
			wantErr:    false,
		},
		{
			name:      "HTML end-to-end",
			bodyValue: base64.StdEncoding.EncodeToString([]byte("<html><head><title>Test</title></head></html>")),
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_HTML,
			},
			wantParsed: "<html><head><title>Test</title></head></html>",
			wantErr:    false,
		},
		{
			name:      "Binary (PNG image) end-to-end",
			bodyValue: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			metadata: &RequestBodyMetadata{
				Encoding:    core.EncodingType_ENCODING_TYPE_BASE64,
				DecodedType: core.DecodedType_DECODED_TYPE_PNG,
			},
			wantParsed: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decodedBytes, decodedType, err := DecodeBody(tt.bodyValue, tt.metadata)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			parsed, err := ParseBodyForComparison(decodedBytes, decodedType)
			require.NoError(t, err)
			require.Equal(t, tt.wantParsed, parsed)
		})
	}
}
