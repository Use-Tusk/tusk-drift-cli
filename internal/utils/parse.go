package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/log"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

// SpanFilter is a function type for filtering spans during parsing
type SpanFilter func(*core.Span) bool

// ParseSpansFromFile reads a JSONL trace file and returns spans matching the filter.
// If the file is malformed, it returns an error.
func ParseSpansFromFile(filename string, filter SpanFilter) ([]*core.Span, error) {
	file, err := os.Open(filename) // #nosec G304
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Warn("Failed to close file", "error", err, "filename", filename)
		}
	}()

	var spans []*core.Span
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 15*1024*1024) // Initial 64KB, max 15MB

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Parse protobuf JSON directly
		span, err := ParseProtobufSpanFromJSON([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("malformed span in %s at line %d: %w", filename, lineNum, err)
		}

		// if span.IsPreAppStart {
		// 	log.Debug("Found pre-app-start span", "span", span)
		// }

		// Apply filter if provided, otherwise include all spans
		if filter == nil || filter(span) {
			spans = append(spans, span)
		}
	}

	if err := scanner.Err(); err != nil {
		if err == bufio.ErrTooLong {
			return nil, fmt.Errorf(
				"trace file contains a line that exceeds the maximum size (15MB).\n\n"+
					"This typically happens when HTTP responses contain large media files (videos, images, etc.)\n"+
					"that are base64-encoded in the trace.\n\n"+
					"To fix this:\n"+
					"1. Use transforms in your .tusk/config.yaml to exclude large response bodies\n"+
					"2. Consider deleting this trace file if it's not needed\n\n"+
					"File: %s\n"+
					"See: https://docs.usetusk.ai/automated-tests/pii-redaction",
				filename,
			)
		}
		return nil, fmt.Errorf("failed reading %s: %w", filename, err)
	}

	return spans, nil
}

// ParseProtobufSpanFromJSON parses a JSON line into a protobuf Span
func ParseProtobufSpanFromJSON(jsonData []byte) (*core.Span, error) {
	var spanMap map[string]any
	if err := json.Unmarshal(jsonData, &spanMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Helper functions for safe type conversion
	getString := func(key string) string {
		if v, exists := spanMap[key]; exists && v != nil {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	getBool := func(key string) bool {
		if v, exists := spanMap[key]; exists && v != nil {
			if b, ok := v.(bool); ok {
				return b
			}
		}
		return false
	}

	getInt32 := func(key string) int32 {
		if v, exists := spanMap[key]; exists && v != nil {
			if f, ok := v.(float64); ok {
				return int32(f)
			}
		}
		return 0
	}

	// Convert nested objects to protobuf Struct
	convertToStruct := func(key string) *structpb.Struct {
		if v, exists := spanMap[key]; exists && v != nil {
			if objMap, ok := v.(map[string]any); ok {
				if s, err := structpb.NewStruct(objMap); err == nil {
					return s
				}
			}
		}
		return nil
	}

	// Convert schema objects to proto JsonSchema
	convertToJsonSchema := func(key string) *core.JsonSchema {
		if v, exists := spanMap[key]; exists && v != nil {
			if objMap, ok := v.(map[string]any); ok {
				return mapToJsonSchema(objMap)
			}
		}
		return nil
	}

	// Handle status object
	var status *core.SpanStatus
	if statusObj, exists := spanMap["status"]; exists && statusObj != nil {
		if statusMap, ok := statusObj.(map[string]any); ok {
			code := core.StatusCode_STATUS_CODE_UNSPECIFIED
			if codeFloat, ok := statusMap["code"].(float64); ok {
				code = core.StatusCode(int32(codeFloat))
			}
			message := ""
			if msgStr, ok := statusMap["message"].(string); ok {
				message = msgStr
			}
			status = &core.SpanStatus{
				Code:    code,
				Message: message,
			}
		}
	}

	// Handle timestamp
	var timestamp *timestamppb.Timestamp
	if tsObj, exists := spanMap["timestamp"]; exists && tsObj != nil {
		if tsMap, ok := tsObj.(map[string]any); ok {
			seconds := int64(0)
			nanos := int32(0)
			if secFloat, ok := tsMap["seconds"].(float64); ok {
				seconds = int64(secFloat)
			}
			if nanosFloat, ok := tsMap["nanos"].(float64); ok {
				nanos = int32(nanosFloat)
			}
			timestamp = &timestamppb.Timestamp{
				Seconds: seconds,
				Nanos:   nanos,
			}
		}
	}

	// Handle duration
	var duration *durationpb.Duration
	if durObj, exists := spanMap["duration"]; exists && durObj != nil {
		if durMap, ok := durObj.(map[string]any); ok {
			seconds := int64(0)
			nanos := int32(0)
			if secFloat, ok := durMap["seconds"].(float64); ok {
				seconds = int64(secFloat)
			}
			if nanosFloat, ok := durMap["nanos"].(float64); ok {
				nanos = int32(nanosFloat)
			}
			duration = &durationpb.Duration{
				Seconds: seconds,
				Nanos:   nanos,
			}
		}
	}

	// Handle environment (optional string pointer)
	var environment *string
	if env := getString("environment"); env != "" {
		environment = &env
	}

	return &core.Span{
		TraceId:             getString("traceId"),
		SpanId:              getString("spanId"),
		ParentSpanId:        getString("parentSpanId"),
		Name:                getString("name"),
		PackageName:         getString("packageName"),
		InstrumentationName: getString("instrumentationName"),
		SubmoduleName:       getString("submodule_name"),
		InputValue:          convertToStruct("inputValue"),
		OutputValue:         convertToStruct("outputValue"),
		InputSchema:         convertToJsonSchema("inputSchema"),
		OutputSchema:        convertToJsonSchema("outputSchema"),
		InputSchemaHash:     getString("inputSchemaHash"),
		OutputSchemaHash:    getString("outputSchemaHash"),
		InputValueHash:      getString("inputValueHash"),
		OutputValueHash:     getString("outputValueHash"),
		Kind:                core.SpanKind(getInt32("kind")),
		Status:              status,
		Timestamp:           timestamp,
		Duration:            duration,
		IsPreAppStart:       getBool("isPreAppStart"),
		IsRootSpan:          getBool("isRootSpan"),
		Metadata:            convertToStruct("metadata"),
		PackageType:         core.PackageType(getInt32("packageType")),
		Environment:         environment,
	}, nil
}

// mapToJsonSchema converts a map[string]any to a proto JsonSchema
func mapToJsonSchema(m map[string]any) *core.JsonSchema {
	if m == nil {
		return nil
	}

	schema := &core.JsonSchema{
		Type:       core.JsonSchemaType_JSON_SCHEMA_TYPE_UNSPECIFIED,
		Properties: make(map[string]*core.JsonSchema),
	}

	// Extract type (proto enum as numeric value)
	if typeVal, ok := m["type"].(float64); ok {
		schema.Type = core.JsonSchemaType(int32(typeVal))
	}

	// Extract properties
	if propsVal, ok := m["properties"].(map[string]any); ok {
		for key, val := range propsVal {
			if propMap, ok := val.(map[string]any); ok {
				schema.Properties[key] = mapToJsonSchema(propMap)
			}
		}
	}

	// Extract items
	if itemsVal, ok := m["items"].(map[string]any); ok {
		schema.Items = mapToJsonSchema(itemsVal)
	}

	// Extract encoding
	if encVal, ok := m["encoding"].(float64); ok {
		enc := core.EncodingType(int32(encVal))
		schema.Encoding = &enc
	}

	// Extract decodedType
	if decVal, ok := m["decodedType"].(float64); ok {
		dec := core.DecodedType(int32(decVal))
		schema.DecodedType = &dec
	}

	// Extract matchImportance
	if matchVal, ok := m["matchImportance"].(float64); ok {
		schema.MatchImportance = &matchVal
	}

	return schema
}
