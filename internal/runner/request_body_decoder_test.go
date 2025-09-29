package runner

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestExtractRequestBodyMetadata tests the extraction of metadata from schemas
func TestExtractRequestBodyMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		schema    *structpb.Struct
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
			name:      "empty schema",
			schema:    makeStruct(t, map[string]any{}),
			fieldPath: "body",
			want:      nil,
		},
		{
			name: "schema without properties",
			schema: makeStruct(t, map[string]any{
				"type": "object",
			}),
			fieldPath: "body",
			want:      nil,
		},
		{
			name: "schema with properties but missing field",
			schema: makeStruct(t, map[string]any{
				"properties": map[string]any{
					"otherField": map[string]any{
						"type": "string",
					},
				},
			}),
			fieldPath: "body",
			want:      nil,
		},
		{
			name: "schema with field but no metadata",
			schema: makeStruct(t, map[string]any{
				"properties": map[string]any{
					"body": map[string]any{
						"type": "string",
					},
				},
			}),
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    EncodingTypeNONE,
				DecodedType: DecodedTypeUNSPECIFIED,
			},
		},
		{
			name: "schema with base64 encoding and JSON type",
			schema: makeStruct(t, map[string]any{
				"properties": map[string]any{
					"body": map[string]any{
						"type":        "string",
						"encoding":    "BASE64",
						"decodedType": "JSON",
					},
				},
			}),
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypeJSON,
			},
		},
		{
			name: "schema with NONE encoding and HTML type",
			schema: makeStruct(t, map[string]any{
				"properties": map[string]any{
					"body": map[string]any{
						"type":        "string",
						"encoding":    "NONE",
						"decodedType": "HTML",
					},
				},
			}),
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    EncodingTypeNONE,
				DecodedType: DecodedTypeHTML,
			},
		},
		{
			name: "schema with only encoding",
			schema: makeStruct(t, map[string]any{
				"properties": map[string]any{
					"body": map[string]any{
						"type":     "string",
						"encoding": "BASE64",
					},
				},
			}),
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypeUNSPECIFIED,
			},
		},
		{
			name: "schema with only decodedType",
			schema: makeStruct(t, map[string]any{
				"properties": map[string]any{
					"body": map[string]any{
						"type":        "string",
						"decodedType": "XML",
					},
				},
			}),
			fieldPath: "body",
			want: &RequestBodyMetadata{
				Encoding:    EncodingTypeNONE,
				DecodedType: DecodedTypeXML,
			},
		},
		{
			name: "schema with binary types",
			schema: makeStruct(t, map[string]any{
				"properties": map[string]any{
					"image": map[string]any{
						"type":        "string",
						"encoding":    "BASE64",
						"decodedType": "PNG",
					},
				},
			}),
			fieldPath: "image",
			want: &RequestBodyMetadata{
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypePNG,
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
		wantType    DecodedType
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
			wantType:  DecodedTypeUNSPECIFIED,
			wantErr:   false,
		},
		{
			name:      "empty metadata defaults to base64",
			bodyValue: base64JSON,
			metadata:  &RequestBodyMetadata{},
			wantBytes: []byte(jsonData),
			wantType:  DecodedTypeUNSPECIFIED,
			wantErr:   false,
		},
		{
			name:      "explicit base64 encoding with JSON type",
			bodyValue: base64JSON,
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypeJSON,
			},
			wantBytes: []byte(jsonData),
			wantType:  DecodedTypeJSON,
			wantErr:   false,
		},
		{
			name:      "NONE encoding with plain text",
			bodyValue: "plain text content",
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingTypeNONE,
				DecodedType: DecodedTypePLAINTEXT,
			},
			wantBytes: []byte("plain text content"),
			wantType:  DecodedTypePLAINTEXT,
			wantErr:   false,
		},
		{
			name:      "invalid base64 with BASE64 encoding",
			bodyValue: "not-valid-base64!!!",
			metadata: &RequestBodyMetadata{
				Encoding: EncodingTypeBASE64,
			},
			wantErr:     true,
			errContains: "failed to decode base64 body",
		},
		{
			name:      "HTML with base64 encoding",
			bodyValue: base64HTML,
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypeHTML,
			},
			wantBytes: []byte(htmlData),
			wantType:  DecodedTypeHTML,
			wantErr:   false,
		},
		{
			name:      "unknown encoding falls back gracefully",
			bodyValue: base64JSON,
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingType("UNKNOWN"),
				DecodedType: DecodedTypeJSON,
			},
			wantBytes: []byte(jsonData), // Should successfully decode as base64
			wantType:  DecodedTypeJSON,
			wantErr:   false,
		},
		{
			name:      "unknown encoding with invalid base64 falls back to raw",
			bodyValue: "not base64",
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingType("UNKNOWN"),
				DecodedType: DecodedTypePLAINTEXT,
			},
			wantBytes: []byte("not base64"),
			wantType:  DecodedTypePLAINTEXT,
			wantErr:   false,
		},
		{
			name:      "empty string body",
			bodyValue: "",
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingTypeNONE,
				DecodedType: DecodedTypePLAINTEXT,
			},
			wantBytes: []byte(""),
			wantType:  DecodedTypePLAINTEXT,
			wantErr:   false,
		},
		{
			name:      "binary data (PNG) with base64",
			bodyValue: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypePNG,
			},
			wantBytes: func() []byte {
				b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
				return b
			}(),
			wantType: DecodedTypePNG,
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
		decodedType  DecodedType
		want         any
		wantErr      bool
	}{
		{
			name:         "JSON valid object",
			decodedBytes: []byte(`{"key": "value", "num": 42}`),
			decodedType:  DecodedTypeJSON,
			want: map[string]any{
				"key": "value",
				"num": float64(42), // JSON numbers are float64
			},
			wantErr: false,
		},
		{
			name:         "JSON valid array",
			decodedBytes: []byte(`[1, 2, 3]`),
			decodedType:  DecodedTypeJSON,
			want:         []any{float64(1), float64(2), float64(3)},
			wantErr:      false,
		},
		{
			name:         "JSON invalid falls back to string",
			decodedBytes: []byte(`{invalid json`),
			decodedType:  DecodedTypeJSON,
			want:         "{invalid json",
			wantErr:      false,
		},
		{
			name:         "PLAIN_TEXT",
			decodedBytes: []byte("plain text content"),
			decodedType:  DecodedTypePLAINTEXT,
			want:         "plain text content",
			wantErr:      false,
		},
		{
			name:         "HTML",
			decodedBytes: []byte("<html><body>Hello</body></html>"),
			decodedType:  DecodedTypeHTML,
			want:         "<html><body>Hello</body></html>",
			wantErr:      false,
		},
		{
			name:         "CSS",
			decodedBytes: []byte("body { color: red; }"),
			decodedType:  DecodedTypeCSS,
			want:         "body { color: red; }",
			wantErr:      false,
		},
		{
			name:         "JAVASCRIPT",
			decodedBytes: []byte("function test() { return true; }"),
			decodedType:  DecodedTypeJAVASCRIPT,
			want:         "function test() { return true; }",
			wantErr:      false,
		},
		{
			name:         "XML",
			decodedBytes: []byte("<root><item>value</item></root>"),
			decodedType:  DecodedTypeXML,
			want:         "<root><item>value</item></root>",
			wantErr:      false,
		},
		{
			name:         "YAML",
			decodedBytes: []byte("key: value\narray:\n  - item1\n  - item2"),
			decodedType:  DecodedTypeYAML,
			want:         "key: value\narray:\n  - item1\n  - item2",
			wantErr:      false,
		},
		{
			name:         "MARKDOWN",
			decodedBytes: []byte("# Header\n\nThis is **bold**"),
			decodedType:  DecodedTypeMARKDOWN,
			want:         "# Header\n\nThis is **bold**",
			wantErr:      false,
		},
		{
			name:         "CSV",
			decodedBytes: []byte("name,age\nAlice,30\nBob,25"),
			decodedType:  DecodedTypeCSV,
			want:         "name,age\nAlice,30\nBob,25",
			wantErr:      false,
		},
		{
			name:         "SQL",
			decodedBytes: []byte("SELECT * FROM users WHERE id = 1"),
			decodedType:  DecodedTypeSQL,
			want:         "SELECT * FROM users WHERE id = 1",
			wantErr:      false,
		},
		{
			name:         "GRAPHQL",
			decodedBytes: []byte("query { user(id: 1) { name } }"),
			decodedType:  DecodedTypeGRAPHQL,
			want:         "query { user(id: 1) { name } }",
			wantErr:      false,
		},
		{
			name:         "SVG",
			decodedBytes: []byte(`<svg><circle cx="50" cy="50" r="40"/></svg>`),
			decodedType:  DecodedTypeSVG,
			want:         `<svg><circle cx="50" cy="50" r="40"/></svg>`,
			wantErr:      false,
		},
		{
			name:         "FORM_DATA",
			decodedBytes: []byte("key1=value1&key2=value2"),
			decodedType:  DecodedTypeFORMDATA,
			want:         "key1=value1&key2=value2",
			wantErr:      false,
		},
		{
			name:         "MULTIPART_FORM",
			decodedBytes: []byte("--boundary\r\nContent-Disposition: form-data; name=\"field\"\r\n\r\nvalue\r\n--boundary--"),
			decodedType:  DecodedTypeMULTIPARTFORM,
			want:         "--boundary\r\nContent-Disposition: form-data; name=\"field\"\r\n\r\nvalue\r\n--boundary--",
			wantErr:      false,
		},
		{
			name:         "BINARY returns base64",
			decodedBytes: []byte{0x00, 0x01, 0x02, 0xff},
			decodedType:  DecodedTypeBINARY,
			want:         base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0x02, 0xff}),
			wantErr:      false,
		},
		{
			name:         "PDF returns base64",
			decodedBytes: []byte("%PDF-1.4"),
			decodedType:  DecodedTypePDF,
			want:         base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")),
			wantErr:      false,
		},
		{
			name:         "JPEG returns base64",
			decodedBytes: []byte{0xff, 0xd8, 0xff, 0xe0},
			decodedType:  DecodedTypeJPEG,
			want:         base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xe0}),
			wantErr:      false,
		},
		{
			name:         "PNG returns base64",
			decodedBytes: []byte{0x89, 0x50, 0x4e, 0x47},
			decodedType:  DecodedTypePNG,
			want:         base64.StdEncoding.EncodeToString([]byte{0x89, 0x50, 0x4e, 0x47}),
			wantErr:      false,
		},
		{
			name:         "GIF returns base64",
			decodedBytes: []byte("GIF89a"),
			decodedType:  DecodedTypeGIF,
			want:         base64.StdEncoding.EncodeToString([]byte("GIF89a")),
			wantErr:      false,
		},
		{
			name:         "WEBP returns base64",
			decodedBytes: []byte("RIFF"),
			decodedType:  DecodedTypeWEBP,
			want:         base64.StdEncoding.EncodeToString([]byte("RIFF")),
			wantErr:      false,
		},
		{
			name:         "AUDIO returns base64",
			decodedBytes: []byte{0x49, 0x44, 0x33}, // MP3 ID3 tag
			decodedType:  DecodedTypeAUDIO,
			want:         base64.StdEncoding.EncodeToString([]byte{0x49, 0x44, 0x33}),
			wantErr:      false,
		},
		{
			name:         "VIDEO returns base64",
			decodedBytes: []byte("ftyp"),
			decodedType:  DecodedTypeVIDEO,
			want:         base64.StdEncoding.EncodeToString([]byte("ftyp")),
			wantErr:      false,
		},
		{
			name:         "GZIP returns base64",
			decodedBytes: []byte{0x1f, 0x8b},
			decodedType:  DecodedTypeGZIP,
			want:         base64.StdEncoding.EncodeToString([]byte{0x1f, 0x8b}),
			wantErr:      false,
		},
		{
			name:         "ZIP returns base64",
			decodedBytes: []byte{0x50, 0x4b, 0x03, 0x04},
			decodedType:  DecodedTypeZIP,
			want:         base64.StdEncoding.EncodeToString([]byte{0x50, 0x4b, 0x03, 0x04}),
			wantErr:      false,
		},
		{
			name:         "UNSPECIFIED with valid JSON tries JSON",
			decodedBytes: []byte(`{"test": true}`),
			decodedType:  DecodedTypeUNSPECIFIED,
			want: map[string]any{
				"test": true,
			},
			wantErr: false,
		},
		{
			name:         "UNSPECIFIED with non-JSON falls back to string",
			decodedBytes: []byte("just plain text"),
			decodedType:  DecodedTypeUNSPECIFIED,
			want:         "just plain text",
			wantErr:      false,
		},
		{
			name:         "unknown type with valid JSON tries JSON",
			decodedBytes: []byte(`[1, 2, 3]`),
			decodedType:  DecodedType("UNKNOWN"),
			want:         []any{float64(1), float64(2), float64(3)},
			wantErr:      false,
		},
		{
			name:         "unknown type with non-JSON falls back to string",
			decodedBytes: []byte("some text"),
			decodedType:  DecodedType("UNKNOWN"),
			want:         "some text",
			wantErr:      false,
		},
		{
			name:         "empty bytes with JSON type",
			decodedBytes: []byte(""),
			decodedType:  DecodedTypeJSON,
			want:         "",
			wantErr:      false,
		},
		{
			name:         "empty bytes with PLAIN_TEXT type",
			decodedBytes: []byte(""),
			decodedType:  DecodedTypePLAINTEXT,
			want:         "",
			wantErr:      false,
		},
		{
			name:         "empty bytes with BINARY type",
			decodedBytes: []byte(""),
			decodedType:  DecodedTypeBINARY,
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
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypeJSON,
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
				Encoding:    EncodingTypeNONE,
				DecodedType: DecodedTypePLAINTEXT,
			},
			wantParsed: "Hello, World!",
			wantErr:    false,
		},
		{
			name:      "HTML end-to-end",
			bodyValue: base64.StdEncoding.EncodeToString([]byte("<html><head><title>Test</title></head></html>")),
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypeHTML,
			},
			wantParsed: "<html><head><title>Test</title></head></html>",
			wantErr:    false,
		},
		{
			name:      "Binary (PNG image) end-to-end",
			bodyValue: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			metadata: &RequestBodyMetadata{
				Encoding:    EncodingTypeBASE64,
				DecodedType: DecodedTypePNG,
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
