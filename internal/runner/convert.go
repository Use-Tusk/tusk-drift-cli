package runner

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"google.golang.org/protobuf/types/known/structpb"
)

// ConvertTraceTestsToRunnerTests converts protobuf TraceTests to runner.Tests
func ConvertTraceTestsToRunnerTests(traceTests []*backend.TraceTest) []Test {
	tests := make([]Test, len(traceTests))
	for i, tt := range traceTests {
		tests[i] = ConvertTraceTestToRunnerTest(tt)
	}
	return tests
}

// ConvertTraceTestToRunnerTest converts a single protobuf TraceTest to runner.Test
func ConvertTraceTestToRunnerTest(tt *backend.TraceTest) Test {
	test := Test{
		FileName:    fmt.Sprintf("trace_%s.json", tt.TraceId),
		TraceID:     tt.TraceId,
		TraceTestID: tt.Id,
		Spans:       tt.Spans,
		Type:        "http", // execution is HTTP replay
		DisplayType: "HTTP", // will be overridden if we detect GraphQL/etc.
		DisplayName: fmt.Sprintf("Trace %s", tt.TraceId),
		Status:      "pending",
	}

	// Pick spans for execution and display
	var serverSpan *core.Span
	for _, s := range tt.Spans {
		if s.Kind == core.SpanKind_SPAN_KIND_SERVER {
			serverSpan = s
			break
		}
	}
	displaySpan := serverSpan
	for _, s := range tt.Spans {
		if s.GetPackageType() == core.PackageType_PACKAGE_TYPE_GRAPHQL && !s.IsPreAppStart {
			displaySpan = s
			break
		}
	}
	if displaySpan == nil && len(tt.Spans) > 0 {
		displaySpan = tt.Spans[0]
	}

	// DisplayType, Status, Duration, Timestamp from displaySpan
	if displaySpan != nil {
		if pt := displaySpan.GetPackageType(); pt != core.PackageType_PACKAGE_TYPE_UNSPECIFIED {
			if s := packageTypeToString(pt); s != "" {
				test.DisplayType = s
			}
		} else if displaySpan.PackageName != "" {
			test.DisplayType = strings.ToUpper(displaySpan.PackageName)
		}

		test.Status = ""

		if displaySpan.Status != nil {
			switch displaySpan.Status.Code {
			case core.StatusCode_STATUS_CODE_OK:
				test.Status = "success"
			case core.StatusCode_STATUS_CODE_ERROR:
				test.Status = "error"
			}
		}

		if displaySpan.Duration != nil {
			test.Duration = int(displaySpan.Duration.Seconds*1000 + int64(displaySpan.Duration.Nanos)/1_000_000)
		}

		if displaySpan.Timestamp != nil {
			test.Timestamp = displaySpan.Timestamp.AsTime().Format(time.RFC3339)
		}
	}

	// Build request/response from server span (execution details)
	if serverSpan != nil {
		// Prefer recorded input fields
		if serverSpan.InputValue != nil {
			if method, ok := getStringFromStruct(serverSpan.InputValue, "method"); ok {
				test.Method = method
				test.Request.Method = method
			}
			// Prefer target (path+query), then path, then parse url
			if target, ok := getStringFromStruct(serverSpan.InputValue, "target"); ok && target != "" {
				test.Path = target
				test.Request.Path = target
			}
			if test.Path == "" {
				if p, ok := getStringFromStruct(serverSpan.InputValue, "path"); ok && p != "" {
					test.Path = p
					test.Request.Path = p
				}
			}
			if test.Path == "" {
				if urlStr, ok := getStringFromStruct(serverSpan.InputValue, "url"); ok && urlStr != "" {
					if u, err := url.Parse(urlStr); err == nil {
						path := u.Path
						if u.RawQuery != "" {
							path += "?" + u.RawQuery
						}
						if path != "" {
							test.Path = path
							test.Request.Path = path
						}
					}
				}
			}
			test.Request.Headers = extractHeadersFromStruct(serverSpan.InputValue, "headers")
			if body := extractBodyFromStruct(serverSpan.InputValue, "body"); body != nil {
				test.Request.Body = body // Raw value; executor decodes using InputSchema
			}
		}
		// Fallbacks from metadata
		if (test.Method == "" || test.Path == "") && serverSpan.Metadata != nil {
			if test.Method == "" {
				if method, ok := getStringFromStruct(serverSpan.Metadata, "http.method"); ok {
					test.Method = method
					test.Request.Method = method
				}
			}
			if test.Path == "" {
				if target, ok := getStringFromStruct(serverSpan.Metadata, "http.target"); ok && target != "" {
					test.Path = target
					test.Request.Path = target
				} else if urlStr, ok := getStringFromStruct(serverSpan.Metadata, "http.url"); ok && urlStr != "" {
					if u, err := url.Parse(urlStr); err == nil {
						path := u.Path
						if u.RawQuery != "" {
							path += "?" + u.RawQuery
						}
						if path != "" {
							test.Path = path
							test.Request.Path = path
						}
					}
				}
			}
		}
		// Expected response from output value
		if serverSpan.OutputValue != nil {
			status := 0
			if v, ok := serverSpan.OutputValue.Fields["statusCode"]; ok {
				if n := int(v.GetNumberValue()); n != 0 {
					status = n
				}
			}
			if status == 0 {
				if v, ok := serverSpan.OutputValue.Fields["status"]; ok {
					if n := int(v.GetNumberValue()); n != 0 {
						status = n
					}
				}
			}
			test.Response.Status = status

			// Decode body using schema
			if bodyField, ok := serverSpan.OutputValue.Fields["body"]; ok {
				bodyValue := bodyField.AsInterface()
				if bodyValue != nil {
					// Extract body schema from output schema
					var bodySchema *core.JsonSchema
					if serverSpan.OutputSchema != nil && serverSpan.OutputSchema.Properties != nil {
						bodySchema = serverSpan.OutputSchema.Properties["body"]
					}

					// Decode and parse the body (returns both bytes and parsed value)
					_, parsedBody, err := DecodeValueBySchema(bodyValue, bodySchema)
					if err == nil {
						test.Response.Body = parsedBody
					}
				}
			}
		}

		// Attach metadata (for ENV_VARS header)
		if serverSpan.Metadata != nil {
			test.Metadata = serverSpan.Metadata.AsMap()
		}
	}

	// Display name rules:
	// - Prefer GraphQL display span name (e.g., "query GetUser")
	// - Else method + path
	// - Else fallback to display span name
	isGraphQL := displaySpan != nil && (displaySpan.GetPackageType() == core.PackageType_PACKAGE_TYPE_GRAPHQL || strings.EqualFold(displaySpan.PackageName, "graphql"))
	switch {
	case isGraphQL && displaySpan != nil && displaySpan.Name != "":
		test.DisplayName = displaySpan.Name
	case test.Method != "" && test.Path != "":
		test.DisplayName = fmt.Sprintf("%s %s", test.Method, test.Path)
	case displaySpan != nil && displaySpan.Name != "":
		test.DisplayName = displaySpan.Name
	}

	// If we didn't get metadata above, try from displaySpan
	if test.Metadata == nil && displaySpan != nil && displaySpan.Metadata != nil {
		test.Metadata = displaySpan.Metadata.AsMap()
	}

	return test
}

func extractHeadersFromStruct(s *structpb.Struct, key string) map[string]string {
	h := map[string]string{}
	if s == nil || s.Fields == nil {
		return h
	}
	if hf, ok := s.Fields[key]; ok {
		if hs := hf.GetStructValue(); hs != nil {
			for k, v := range hs.Fields {
				if sv := v.GetStringValue(); sv != "" {
					h[k] = sv
				}
			}
		}
	}
	return h
}

func extractBodyFromStruct(s *structpb.Struct, key string) any {
	if s == nil || s.Fields == nil {
		return nil
	}
	if bf, ok := s.Fields[key]; ok {
		return bf.AsInterface()
	}
	return nil
}

// ConvertRunnerResultsToTraceTestResults maps local results for upload to the backend
func ConvertRunnerResultsToTraceTestResults(results []TestResult, tests []Test) []*backend.TraceTestResult {
	traceTestResults := make([]*backend.TraceTestResult, len(results))
	for i, result := range results {
		traceTestResults[i] = ConvertRunnerResultToTraceTestResult(result, tests[i])
	}
	return traceTestResults
}

func ConvertRunnerResultToTraceTestResult(result TestResult, test Test) *backend.TraceTestResult {
	out := &backend.TraceTestResult{
		TraceTestId: result.TestID,
		TestSuccess: result.Passed,
	}

	if !result.Passed {
		if result.Error != "" {
			out.TestFailureMessage = &result.Error
			reason := backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_NO_RESPONSE
			out.TestFailureReason = &reason
		} else if len(result.Deviations) > 0 {
			reason := backend.TraceTestFailureReason_TRACE_TEST_FAILURE_REASON_RESPONSE_MISMATCH
			out.TestFailureReason = &reason
			msg := fmt.Sprintf("Found %d deviations", len(result.Deviations))
			out.TestFailureMessage = &msg
		}
	}

	if len(result.Deviations) > 0 {
		spanResult := &backend.TraceTestSpanResult{}
		for _, dev := range result.Deviations {
			spanResult.Deviations = append(spanResult.Deviations, &backend.Deviation{
				Field:       dev.Field,
				Description: dev.Description,
			})
		}
		out.SpanResults = []*backend.TraceTestSpanResult{spanResult}
	}

	return out
}

func getStringFromStruct(s *structpb.Struct, key string) (string, bool) {
	if s == nil || s.Fields == nil {
		return "", false
	}
	if field, ok := s.Fields[key]; ok {
		if v := field.GetStringValue(); v != "" {
			return v, true
		}
	}
	return "", false
}
