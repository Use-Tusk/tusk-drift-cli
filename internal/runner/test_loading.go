package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

func (e *Executor) LoadTestsFromFolder(folder string) ([]Test, error) {
	var tests []Test

	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".jsonl") {
			traceTests, err := e.LoadTestsFromTraceFile(path)
			if err != nil {
				return err
			}
			tests = append(tests, traceTests...)
		}

		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []Test{}, fmt.Errorf("traces folder not found: %s", folder)
		}
		return nil, err
	}

	return tests, nil
}

// LoadTestsFromTraceFile loads all tests from a trace file
func (e *Executor) LoadTestsFromTraceFile(path string) ([]Test, error) {
	spans, err := utils.ParseSpansFromFile(path, nil)
	if err != nil {
		return nil, err
	}

	// Group all spans by trace ID so each test can include full context (including pre-app-start)
	fullByTrace := make(map[string][]*core.Span) // traceId -> spans
	for _, s := range spans {
		fullByTrace[s.TraceId] = append(fullByTrace[s.TraceId], s)
	}

	traceMap := make(map[string]*core.Span) // traceId -> representative span
	filename := filepath.Base(path)

	// Choose a representative span per trace for display (prefer root; skip pre-app-start)
	for _, span := range spans {
		if span.IsPreAppStart {
			continue
		}

		if span.IsRootSpan || traceMap[span.TraceId] == nil {
			traceMap[span.TraceId] = span
		}
		if span.IsRootSpan && traceMap[span.TraceId] != nil {
			traceMap[span.TraceId] = span
		}
	}

	var tests []Test
	for _, span := range traceMap {
		test := spanToTest(span, filename)
		// Attach full span set for this trace to enable suite/global matching locally
		// This will be deduped in buildSuiteSpansForRun()
		if all := fullByTrace[span.TraceId]; len(all) > 0 {
			test.Spans = all
		}
		tests = append(tests, test)
	}

	return tests, nil
}

func (e *Executor) LoadSpansForTrace(traceID string, filename string) ([]*core.Span, error) {
	tracePath, err := utils.FindTraceFile(traceID, filename)
	if err != nil {
		return nil, err
	}

	filter := func(span *core.Span) bool {
		return span.TraceId == traceID
	}

	return utils.ParseSpansFromFile(tracePath, filter)
}

// spanToTest converts a protobuf Span to Test format for display
func spanToTest(span *core.Span, filename string) Test {
	durationMs := 0
	if span.Duration != nil {
		// Convert protobuf Duration to milliseconds
		durationMs = int(span.Duration.Seconds*1000 + int64(span.Duration.Nanos)/1000000)
	}

	// TODO: In order to replay inbound requests different from HTTP (e.g. gRPC, etc.),
	// we need to check for the span.PackageName here and depending on the type create a new test
	// with the appropriate request structure since they would likely be different for different
	// inbound request protocols. Each protocol will have its own input format, headers, and
	// execution patterns that need to be handled accordingly.
	// The logic below is for HTTP spans only.

	// Determine type based on package name
	spanType := span.PackageName

	// Extract method and path based on span type
	method := span.SubmoduleName
	path := ""
	if span.InputValue != nil {
		if target, exists := span.InputValue.Fields["target"]; exists {
			if targetStr := target.GetStringValue(); targetStr != "" {
				path = targetStr
			}
		}
	}

	displayName := span.Name
	displayType := spanType
	if span.GetPackageType() != core.PackageType_PACKAGE_TYPE_UNSPECIFIED {
		displayType = packageTypeToString(span.GetPackageType())
	}

	// For HTTP spans, try to extract more meaningful info
	if (span.GetPackageType() == core.PackageType_PACKAGE_TYPE_HTTP || span.GetPackageType() == core.PackageType_PACKAGE_TYPE_GRAPHQL) && span.InputValue != nil {
		if httpMethod, exists := span.InputValue.Fields["method"]; exists {
			if methodStr := httpMethod.GetStringValue(); methodStr != "" {
				method = methodStr
			}
		}
		if target, exists := span.InputValue.Fields["target"]; exists {
			if targetStr := target.GetStringValue(); targetStr != "" {
				path = targetStr
			}
		}
	}

	status := "success"
	if span.Status != nil && span.Status.Code != core.StatusCode_STATUS_CODE_OK {
		status = "error"
	}

	httpStatus := 200 // Default status code
	var responseBody any

	if (span.GetPackageType() == core.PackageType_PACKAGE_TYPE_HTTP || span.GetPackageType() == core.PackageType_PACKAGE_TYPE_GRAPHQL) && span.OutputValue != nil {
		if statusCode, exists := span.OutputValue.Fields["statusCode"]; exists {
			if statusFloat := statusCode.GetNumberValue(); statusFloat != 0 {
				httpStatus = int(statusFloat)
			}
		}

		if bodyField, exists := span.OutputValue.Fields["body"]; exists {
			if bodyStr := bodyField.GetStringValue(); bodyStr != "" {
				bodyMetadata := ExtractRequestBodyMetadata(span.OutputSchema, "body")
				decodedBytes, decodedType, err := DecodeBody(bodyStr, bodyMetadata)
				if err != nil {
					// Fall back to raw value on error
					responseBody = bodyStr
				} else {
					parsedBody, err := ParseBodyForComparison(decodedBytes, decodedType)
					if err != nil {
						// Fall back to decoded string on parse error
						responseBody = string(decodedBytes)
					} else {
						responseBody = parsedBody
					}
				}
			}
		}
	}

	timestampStr := ""
	if span.Timestamp != nil {
		timestampStr = span.Timestamp.AsTime().Format(time.RFC3339)
	}

	return Test{
		FileName:    filename,
		TraceID:     span.TraceId,
		Type:        spanType,
		DisplayType: displayType,
		Timestamp:   timestampStr,
		Method:      method,
		Path:        path,
		DisplayName: displayName,
		Status:      status,
		Duration:    durationMs,
		Metadata:    span.Metadata.AsMap(),
		Request: Request{
			Method:  method,
			Path:    path,
			Headers: extractHeaders(span),
			Body:    extractBody(span),
		},
		Response: Response{
			Status: httpStatus,
			Body:   responseBody,
		},
	}
}

// extractHeaders extracts HTTP headers from span input data
func extractHeaders(span *core.Span) map[string]string {
	headers := make(map[string]string)
	if (span.GetPackageType() == core.PackageType_PACKAGE_TYPE_HTTP || span.GetPackageType() == core.PackageType_PACKAGE_TYPE_GRAPHQL) && span.InputValue != nil {
		if headersField, exists := span.InputValue.Fields["headers"]; exists {
			if headersStruct := headersField.GetStructValue(); headersStruct != nil {
				for key, value := range headersStruct.Fields {
					if strValue := value.GetStringValue(); strValue != "" {
						headers[key] = strValue
					}
				}
			}
		}
	}
	return headers
}

// extractBody extracts HTTP request body from span input data
func extractBody(span *core.Span) any {
	if (span.GetPackageType() == core.PackageType_PACKAGE_TYPE_HTTP || span.GetPackageType() == core.PackageType_PACKAGE_TYPE_GRAPHQL) && span.InputValue != nil {
		if bodyField, exists := span.InputValue.Fields["body"]; exists {
			return bodyField.AsInterface()
		}
	}
	return nil
}

// packageTypeToString converts PackageType enum to human-readable string
func packageTypeToString(packageType core.PackageType) string {
	switch packageType {
	case core.PackageType_PACKAGE_TYPE_HTTP:
		return "HTTP"
	case core.PackageType_PACKAGE_TYPE_GRAPHQL:
		return "GRAPHQL"
	case core.PackageType_PACKAGE_TYPE_GRPC:
		return "GRPC"
	case core.PackageType_PACKAGE_TYPE_PG:
		return "PG"
	case core.PackageType_PACKAGE_TYPE_MYSQL:
		return "MYSQL"
	case core.PackageType_PACKAGE_TYPE_MONGODB:
		return "MONGODB"
	case core.PackageType_PACKAGE_TYPE_REDIS:
		return "REDIS"
	case core.PackageType_PACKAGE_TYPE_KAFKA:
		return "KAFKA"
	case core.PackageType_PACKAGE_TYPE_RABBITMQ:
		return "RABBITMQ"
	default:
		return ""
	}
}
