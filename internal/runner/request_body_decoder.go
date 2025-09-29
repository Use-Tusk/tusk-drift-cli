package runner

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
)

// DecodedType represents the type of decoded content.
// Must match the DecodedType enum in tusk-drift-sdk/src/core/tracing/JsonSchemaHelper.ts.
// TODO: use Protobuf to share this type across the CLI and SDK(s).
type DecodedType string

const (
	DecodedTypeJSON          DecodedType = "JSON"
	DecodedTypeHTML          DecodedType = "HTML"
	DecodedTypeCSS           DecodedType = "CSS"
	DecodedTypeJAVASCRIPT    DecodedType = "JAVASCRIPT"
	DecodedTypeXML           DecodedType = "XML"
	DecodedTypeYAML          DecodedType = "YAML"
	DecodedTypeMARKDOWN      DecodedType = "MARKDOWN"
	DecodedTypeCSV           DecodedType = "CSV"
	DecodedTypeSQL           DecodedType = "SQL"
	DecodedTypeGRAPHQL       DecodedType = "GRAPHQL"
	DecodedTypePLAINTEXT     DecodedType = "PLAIN_TEXT"
	DecodedTypeFORMDATA      DecodedType = "FORM_DATA"
	DecodedTypeMULTIPARTFORM DecodedType = "MULTIPART_FORM"
	DecodedTypePDF           DecodedType = "PDF"
	DecodedTypeAUDIO         DecodedType = "AUDIO"
	DecodedTypeVIDEO         DecodedType = "VIDEO"
	DecodedTypeGZIP          DecodedType = "GZIP"
	DecodedTypeBINARY        DecodedType = "BINARY"
	DecodedTypeJPEG          DecodedType = "JPEG"
	DecodedTypePNG           DecodedType = "PNG"
	DecodedTypeGIF           DecodedType = "GIF"
	DecodedTypeWEBP          DecodedType = "WEBP"
	DecodedTypeSVG           DecodedType = "SVG"
	DecodedTypeZIP           DecodedType = "ZIP"
	DecodedTypeUNSPECIFIED   DecodedType = "UNSPECIFIED"
)

// EncodingType represents how the body is encoded
type EncodingType string

const (
	EncodingTypeBASE64 EncodingType = "BASE64"
	EncodingTypeNONE   EncodingType = "NONE"
)

// RequestBodyMetadata contains encoding information from the schema
type RequestBodyMetadata struct {
	Encoding    EncodingType
	DecodedType DecodedType
}

// ExtractBodyMetadata extracts encoding metadata from a schema for a specific field
func ExtractRequestBodyMetadata(schema *structpb.Struct, fieldPath string) *RequestBodyMetadata {
	if schema == nil || schema.Fields == nil {
		return nil
	}

	properties := schema.Fields["properties"]
	if properties == nil {
		return nil
	}

	propsStruct := properties.GetStructValue()
	if propsStruct == nil {
		return nil
	}

	fieldSchema := propsStruct.Fields[fieldPath]
	if fieldSchema == nil {
		return nil
	}

	fieldStruct := fieldSchema.GetStructValue()
	if fieldStruct == nil {
		return nil
	}

	metadata := &RequestBodyMetadata{
		Encoding:    EncodingTypeNONE,
		DecodedType: DecodedTypeUNSPECIFIED,
	}

	if encodingField := fieldStruct.Fields["encoding"]; encodingField != nil {
		if encodingStr := encodingField.GetStringValue(); encodingStr != "" {
			metadata.Encoding = EncodingType(encodingStr)
		}
	}

	if decodedTypeField := fieldStruct.Fields["decodedType"]; decodedTypeField != nil {
		if decodedTypeStr := decodedTypeField.GetStringValue(); decodedTypeStr != "" {
			metadata.DecodedType = DecodedType(decodedTypeStr)
		}
	}

	return metadata
}

// DecodeBody decodes a body value using schema metadata
// Returns the decoded bytes and the type for further processing
func DecodeBody(bodyValue any, metadata *RequestBodyMetadata) ([]byte, DecodedType, error) {
	bodyStr, ok := bodyValue.(string)
	if !ok {
		return nil, DecodedTypeUNSPECIFIED, fmt.Errorf("expected body to be a string, got %T", bodyValue)
	}

	// Determine encoding - default to base64 if not specified (default SDK behavior)
	encoding := EncodingTypeBASE64
	if metadata != nil && metadata.Encoding != "" {
		encoding = metadata.Encoding
	}

	var decodedBytes []byte
	var err error

	switch encoding {
	case EncodingTypeBASE64:
		decodedBytes, err = base64.StdEncoding.DecodeString(bodyStr)
		if err != nil {
			return nil, DecodedTypeUNSPECIFIED, fmt.Errorf("failed to decode base64 body: %w", err)
		}
	case EncodingTypeNONE:
		decodedBytes = []byte(bodyStr)
	default:
		// Try base64 first, fall back to raw string
		decodedBytes, err = base64.StdEncoding.DecodeString(bodyStr)
		if err != nil {
			decodedBytes = []byte(bodyStr)
		}
	}

	// Determine decoded type
	decodedType := DecodedTypeUNSPECIFIED
	if metadata != nil && metadata.DecodedType != "" {
		decodedType = metadata.DecodedType
	}

	return decodedBytes, decodedType, nil
}

// ParseBodyForComparison parses decoded body bytes for response comparison.
// For JSON, it unmarshals into a structured type; for text, returns string.
// For binary/media types, returns base64-encoded string for comparison.
func ParseBodyForComparison(decodedBytes []byte, decodedType DecodedType) (any, error) {
	switch decodedType {
	case DecodedTypeJSON:
		var parsedBody any
		if err := json.Unmarshal(decodedBytes, &parsedBody); err != nil {
			// If JSON parse fails, fall back to string
			return string(decodedBytes), nil
		}
		return parsedBody, nil

	case DecodedTypePLAINTEXT,
		DecodedTypeHTML,
		DecodedTypeCSS,
		DecodedTypeJAVASCRIPT,
		DecodedTypeXML,
		DecodedTypeYAML,
		DecodedTypeMARKDOWN,
		DecodedTypeCSV,
		DecodedTypeSQL,
		DecodedTypeGRAPHQL,
		DecodedTypeSVG:
		// Text-based formats - return as string for human-readable comparison
		return string(decodedBytes), nil

	case DecodedTypeFORMDATA, DecodedTypeMULTIPARTFORM:
		// Form data - return as string (URL-encoded or multipart boundary)
		return string(decodedBytes), nil

	case DecodedTypeBINARY,
		DecodedTypePDF,
		DecodedTypeAUDIO,
		DecodedTypeVIDEO,
		DecodedTypeGZIP,
		DecodedTypeZIP,
		DecodedTypeJPEG,
		DecodedTypePNG,
		DecodedTypeGIF,
		DecodedTypeWEBP:
		// Binary/media data - return base64 for comparison
		// (comparing raw bytes would be less readable in test output)
		return base64.StdEncoding.EncodeToString(decodedBytes), nil

	case DecodedTypeUNSPECIFIED:
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
