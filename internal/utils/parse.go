package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
			slog.Warn("Failed to close file", "error", err, "filename", filename)
		}
	}()

	var spans []*core.Span
	scanner := bufio.NewScanner(file)
	// Increase buffer size to handle large JSON lines in massive trace files
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // Initial 64KB, max 1MB

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

		if span.IsPreAppStart {
			slog.Debug("Found pre-app-start span", "span", span)
		}

		// Apply filter if provided, otherwise include all spans
		if filter == nil || filter(span) {
			spans = append(spans, span)
		}
	}

	if err := scanner.Err(); err != nil {
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
		InputSchema:         convertToStruct("inputSchema"),
		OutputSchema:        convertToStruct("outputSchema"),
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
	}, nil
}
