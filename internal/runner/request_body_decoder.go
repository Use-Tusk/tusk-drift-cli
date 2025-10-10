package runner

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

// RequestBodyMetadata contains encoding information from the schema
type RequestBodyMetadata struct {
	Encoding    core.EncodingType
	DecodedType core.DecodedType
}

// ExtractBodyMetadata extracts encoding metadata from a JsonSchema for a specific field
func ExtractRequestBodyMetadata(schema *core.JsonSchema, fieldPath string) *RequestBodyMetadata {
	if schema == nil || schema.Properties == nil {
		return nil
	}

	fieldSchema, ok := schema.Properties[fieldPath]
	if !ok || fieldSchema == nil {
		return nil
	}

	metadata := &RequestBodyMetadata{
		Encoding:    core.EncodingType_ENCODING_TYPE_UNSPECIFIED,
		DecodedType: core.DecodedType_DECODED_TYPE_UNSPECIFIED,
	}

	if fieldSchema.Encoding != nil {
		metadata.Encoding = *fieldSchema.Encoding
	}

	if fieldSchema.DecodedType != nil {
		metadata.DecodedType = *fieldSchema.DecodedType
	}

	return metadata
}

// DecodeBody decodes a body value using schema metadata
// Returns the decoded bytes and the type for further processing
func DecodeBody(bodyValue any, metadata *RequestBodyMetadata) ([]byte, core.DecodedType, error) {
	bodyStr, ok := bodyValue.(string)
	if !ok {
		return nil, core.DecodedType_DECODED_TYPE_UNSPECIFIED, fmt.Errorf("expected body to be a string, got %T", bodyValue)
	}

	// Determine encoding - default to base64 if metadata is nil (default SDK behavior)
	encoding := core.EncodingType_ENCODING_TYPE_BASE64
	if metadata != nil {
		if metadata.Encoding != core.EncodingType_ENCODING_TYPE_UNSPECIFIED {
			encoding = metadata.Encoding
		} else {
			// If explicitly UNSPECIFIED, keep it UNSPECIFIED to try base64 with fallback
			encoding = core.EncodingType_ENCODING_TYPE_UNSPECIFIED
		}
	}

	var decodedBytes []byte
	var err error

	switch encoding {
	case core.EncodingType_ENCODING_TYPE_BASE64:
		decodedBytes, err = base64.StdEncoding.DecodeString(bodyStr)
		if err != nil {
			return nil, core.DecodedType_DECODED_TYPE_UNSPECIFIED, fmt.Errorf("failed to decode base64 body: %w", err)
		}
	case core.EncodingType_ENCODING_TYPE_UNSPECIFIED:
		// Try base64 first, fall back to raw string
		decodedBytes, err = base64.StdEncoding.DecodeString(bodyStr)
		if err != nil {
			decodedBytes = []byte(bodyStr)
		}
	default:
		// For any other encoding type, try base64 first, fall back to raw string
		decodedBytes, err = base64.StdEncoding.DecodeString(bodyStr)
		if err != nil {
			decodedBytes = []byte(bodyStr)
		}
	}

	// Determine decoded type
	decodedType := core.DecodedType_DECODED_TYPE_UNSPECIFIED
	if metadata != nil && metadata.DecodedType != core.DecodedType_DECODED_TYPE_UNSPECIFIED {
		decodedType = metadata.DecodedType
	}

	return decodedBytes, decodedType, nil
}

// ParseBodyForComparison parses decoded body bytes for response comparison.
// For JSON, it unmarshals into a structured type; for text, returns string.
// For binary/media types, returns base64-encoded string for comparison.
func ParseBodyForComparison(decodedBytes []byte, decodedType core.DecodedType) (any, error) {
	switch decodedType {
	case core.DecodedType_DECODED_TYPE_JSON:
		var parsedBody any
		if err := json.Unmarshal(decodedBytes, &parsedBody); err != nil {
			// If JSON parse fails, fall back to string
			return string(decodedBytes), nil
		}
		return parsedBody, nil

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
		var parsedBody any
		if err := json.Unmarshal(decodedBytes, &parsedBody); err == nil {
			return parsedBody, nil
		}
		return string(decodedBytes), nil

	default:
		// Unknown type - try JSON, fall back to string
		var parsedBody any
		if err := json.Unmarshal(decodedBytes, &parsedBody); err == nil {
			return parsedBody, nil
		}
		return string(decodedBytes), nil
	}
}
