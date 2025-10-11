package runner

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

// DecodeValueBySchema decodes any value using its JsonSchema.
// Handles encoding (e.g., BASE64) and parsing based on decoded type.
// Returns both decoded bytes (for raw use like HTTP requests) and parsed Go value (for comparison/display).
//
// The function:
// - Decodes based on schema.Encoding (BASE64, UNSPECIFIED with fallback to base64 then raw)
// - Parses decoded bytes based on schema.DecodedType (JSON, text formats, binary formats)
// - Returns raw bytes for cases that need them (e.g., HTTP request bodies)
// - Returns parsed any for cases that need structured comparison (e.g., test assertions)
//
// Single-level decoding only (no recursion through nested properties/items).
//
// Parameters:
//   - value: The value to decode (typically a string)
//   - schema: The JsonSchema describing how to decode the value (nil schema is allowed)
//
// Returns:
//   - decodedBytes: The decoded raw bytes
//   - parsedValue: The parsed Go value (JSON object/array, string, or base64 string for binary)
//   - err: Any error encountered during decoding or parsing
func DecodeValueBySchema(value any, schema *core.JsonSchema) (decodedBytes []byte, parsedValue any, err error) {
	// Handle nil/undefined values
	if value == nil {
		return nil, nil, nil
	}

	// Convert value to string (expected format for encoded data)
	valueStr, ok := value.(string)
	if !ok {
		return nil, nil, fmt.Errorf("expected value to be a string, got %T", value)
	}

	// Determine encoding - default to base64 if schema is nil (default SDK behavior)
	encoding := core.EncodingType_ENCODING_TYPE_BASE64
	if schema != nil && schema.Encoding != nil {
		if *schema.Encoding != core.EncodingType_ENCODING_TYPE_UNSPECIFIED {
			encoding = *schema.Encoding
		} else {
			// If explicitly UNSPECIFIED, keep it UNSPECIFIED to try base64 with fallback
			encoding = core.EncodingType_ENCODING_TYPE_UNSPECIFIED
		}
	}

	// Decode based on encoding type
	switch encoding {
	case core.EncodingType_ENCODING_TYPE_BASE64:
		decodedBytes, err = base64.StdEncoding.DecodeString(valueStr)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode base64 value: %w", err)
		}
	case core.EncodingType_ENCODING_TYPE_UNSPECIFIED:
		// Try base64 first, fall back to raw string
		decodedBytes, err = base64.StdEncoding.DecodeString(valueStr)
		if err != nil {
			decodedBytes = []byte(valueStr)
		}
	default:
		// For any other encoding type, try base64 first, fall back to raw string
		decodedBytes, err = base64.StdEncoding.DecodeString(valueStr)
		if err != nil {
			decodedBytes = []byte(valueStr)
		}
	}

	// Determine decoded type
	decodedType := core.DecodedType_DECODED_TYPE_UNSPECIFIED
	if schema != nil && schema.DecodedType != nil {
		decodedType = *schema.DecodedType
	}

	// Parse decoded bytes based on decoded type
	parsedValue, err = parseDecodedBytes(decodedBytes, decodedType)
	if err != nil {
		return nil, nil, err
	}

	return decodedBytes, parsedValue, nil
}

// parseDecodedBytes parses decoded bytes based on the decoded type.
// For JSON, it unmarshals into a structured type; for text, returns string.
// For binary/media types, returns base64-encoded string for comparison.
func parseDecodedBytes(decodedBytes []byte, decodedType core.DecodedType) (any, error) {
	switch decodedType {
	case core.DecodedType_DECODED_TYPE_JSON:
		var parsedValue any
		if err := json.Unmarshal(decodedBytes, &parsedValue); err != nil {
			// If JSON parse fails, fall back to string
			return string(decodedBytes), nil
		}
		return parsedValue, nil

	case core.DecodedType_DECODED_TYPE_PLAIN_TEXT,
		core.DecodedType_DECODED_TYPE_HTML,
		core.DecodedType_DECODED_TYPE_CSS,
		core.DecodedType_DECODED_TYPE_JAVASCRIPT,
		core.DecodedType_DECODED_TYPE_XML,
		core.DecodedType_DECODED_TYPE_YAML,
		core.DecodedType_DECODED_TYPE_MARKDOWN,
		core.DecodedType_DECODED_TYPE_CSV,
		core.DecodedType_DECODED_TYPE_SQL,
		core.DecodedType_DECODED_TYPE_GRAPHQL,
		core.DecodedType_DECODED_TYPE_SVG:
		// Text-based formats - return as string for human-readable comparison
		return string(decodedBytes), nil

	case core.DecodedType_DECODED_TYPE_FORM_DATA, core.DecodedType_DECODED_TYPE_MULTIPART_FORM:
		// Form data - return as string (URL-encoded or multipart boundary)
		return string(decodedBytes), nil

	case core.DecodedType_DECODED_TYPE_BINARY,
		core.DecodedType_DECODED_TYPE_PDF,
		core.DecodedType_DECODED_TYPE_AUDIO,
		core.DecodedType_DECODED_TYPE_VIDEO,
		core.DecodedType_DECODED_TYPE_GZIP,
		core.DecodedType_DECODED_TYPE_ZIP,
		core.DecodedType_DECODED_TYPE_JPEG,
		core.DecodedType_DECODED_TYPE_PNG,
		core.DecodedType_DECODED_TYPE_GIF,
		core.DecodedType_DECODED_TYPE_WEBP:
		// Binary/media data - return base64 for comparison
		// (comparing raw bytes would be less readable in test output)
		return base64.StdEncoding.EncodeToString(decodedBytes), nil

	case core.DecodedType_DECODED_TYPE_UNSPECIFIED:
		// Try JSON first, fall back to string
		var parsedValue any
		if err := json.Unmarshal(decodedBytes, &parsedValue); err == nil {
			return parsedValue, nil
		}
		return string(decodedBytes), nil

	default:
		// Unknown type - try JSON, fall back to string
		var parsedValue any
		if err := json.Unmarshal(decodedBytes, &parsedValue); err == nil {
			return parsedValue, nil
		}
		return string(decodedBytes), nil
	}
}
