package runner

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
)

// compareAndGenerateResult compares the actual HTTP response with expected results
func (e *Executor) compareAndGenerateResult(test Test, actualResp *http.Response, duration int) (TestResult, error) {
	bodyBytes, err := io.ReadAll(actualResp.Body)
	if err != nil {
		return TestResult{}, fmt.Errorf("failed to read response body: %w", err)
	}

	// Extract decodedType from the server span's output schema
	// This ensures we parse the actual response the same way we parsed the expected value
	var decodedType core.DecodedType = core.DecodedType_DECODED_TYPE_UNSPECIFIED
	for _, span := range test.Spans {
		if span.IsRootSpan && span.OutputSchema != nil && span.OutputSchema.Properties != nil {
			bodySchema := span.OutputSchema.Properties["body"]
			if bodySchema != nil && bodySchema.DecodedType != nil {
				decodedType = *bodySchema.DecodedType
				break
			}
		}
	}

	var actualBody any
	if len(bodyBytes) > 0 {
		actualBody, err = parseDecodedBytes(bodyBytes, decodedType)
		if err != nil {
			return TestResult{}, fmt.Errorf("failed to parse actual response body: %w", err)
		}
	}

	log.TestLog(test.TraceID, "Evaluating replay response...")

	// Compare status code
	var deviations []Deviation
	if actualResp.StatusCode != test.Response.Status {
		log.Debug("Status code mismatch", "traceID", test.TraceID, "expected", test.Response.Status, "actual", actualResp.StatusCode)
		deviations = append(deviations, Deviation{
			Field:       "response.status",
			Expected:    test.Response.Status,
			Actual:      actualResp.StatusCode,
			Description: "HTTP status code mismatch",
		})
	}

	// Compare response headers (check important ones)
	for expectedKey, expectedValue := range test.Response.Headers {
		actualValue := actualResp.Header.Get(expectedKey)
		if actualValue != expectedValue {
			log.Debug("Header mismatch", "traceID", test.TraceID, "header", expectedKey, "expected", expectedValue, "actual", actualValue)
			deviations = append(deviations, Deviation{
				Field:       fmt.Sprintf("response.headers.%s", strings.ToLower(expectedKey)),
				Expected:    expectedValue,
				Actual:      actualValue,
				Description: fmt.Sprintf("Header %s mismatch", expectedKey),
			})
		}
	}

	if !e.compareResponseBodies(test.Response.Body, actualBody, test.TraceID) {
		log.Debug("Body mismatch detected", "traceID", test.TraceID, "expected", test.Response.Body, "actual", actualBody)
		deviations = append(deviations, Deviation{
			Field:       "response.body",
			Expected:    test.Response.Body,
			Actual:      actualBody,
			Description: "Response body content mismatch",
		})
	}

	passed := len(deviations) == 0

	log.Debug("Comparison result", "traceID", test.TraceID, "expected", test.Response.Body, "actual", actualBody, "passed", passed, "deviations", deviations)

	result := TestResult{
		TestID:     test.TraceID,
		Passed:     passed,
		Duration:   duration,
		Deviations: deviations,
	}

	log.TestLog(test.TraceID, "Evaluation complete.")

	if passed {
		log.ServiceLog(fmt.Sprintf("Test passed for trace ID %s (%dms)", test.TraceID, duration))
	} else {
		log.ServiceLog(fmt.Sprintf("Test failed for trace ID %s (%dms)", test.TraceID, duration))
	}

	return result, nil
}

// compareResponseBodies performs comparison of response bodies,
// ignoring dynamic fields like UUIDs, timestamps, and dates
func (e *Executor) compareResponseBodies(expected, actual any, testID string) bool {
	var comparisonConfig *config.ComparisonConfig
	cfg, err := config.Get()
	if err == nil {
		// Check if comparison config has any non-default values
		comp := &cfg.Comparison

		log.Debug("Loaded comparison config from file",
			"ignoreFields", comp.IgnoreFields,
			"ignorePatterns", comp.IgnorePatterns,
			"ignoreUUIDs", comp.IgnoreUUIDs,
			"ignoreTimestamps", comp.IgnoreTimestamps,
			"ignoreDates", comp.IgnoreDates)

		// Check if any comparison config is specified
		hasConfig := len(comp.IgnoreFields) > 0 ||
			len(comp.IgnorePatterns) > 0 ||
			comp.IgnoreUUIDs != nil ||
			comp.IgnoreTimestamps != nil ||
			comp.IgnoreDates != nil

		if hasConfig {
			comparisonConfig = comp
			log.Debug("Using comparison config", "config", comparisonConfig)
		} else {
			log.Debug("No comparison config found, using defaults")
		}
		// If all fields are zero/empty, comparisonConfig stays nil for default behavior
	} else {
		log.Debug("Failed to load config", "error", err)
	}

	log.Debug("Values for comparison",
		"expected", expected,
		"actual", actual)

	matcher := NewDynamicFieldMatcherWithConfig(comparisonConfig)
	result := e.compareJSONValues("", expected, actual, matcher, testID)

	log.Debug("Final comparison result", "result", result)

	return result
}

// compareJSONValues recursively compares JSON values, ignoring dynamic fields
func (e *Executor) compareJSONValues(fieldPath string, expected, actual any, matcher *DynamicFieldMatcher, testID string) bool {
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}

	expectedVal := reflect.ValueOf(expected)
	actualVal := reflect.ValueOf(actual)

	if expectedVal.Type() != actualVal.Type() {
		return false
	}

	switch expectedVal.Kind() {
	case reflect.Map:
		return e.compareMaps(fieldPath, expected, actual, matcher, testID)
	case reflect.Slice, reflect.Array:
		return e.compareSlices(fieldPath, expected, actual, matcher, testID)
	case reflect.String, reflect.Float64, reflect.Float32, reflect.Int, reflect.Int64, reflect.Bool: // For primitive values, check if this field should be ignored
		if expected == actual {
			return true
		}
		fieldName := getFieldName(fieldPath)
		if matcher.ShouldIgnoreField(fieldName, expected, actual, testID) {
			return true
		}
		return false
	default:
		return expected == actual
	}
}

// compareMaps compares two map structures
func (e *Executor) compareMaps(fieldPath string, expected, actual any, matcher *DynamicFieldMatcher, testID string) bool {
	expectedMap, ok1 := expected.(map[string]any)
	actualMap, ok2 := actual.(map[string]any)

	if !ok1 || !ok2 {
		return false
	}

	// Check all keys in expected map
	for key, expectedValue := range expectedMap {
		actualValue, exists := actualMap[key]
		if !exists {
			return false
		}

		newFieldPath := key
		if fieldPath != "" {
			newFieldPath = fieldPath + "." + key
		}

		if isEqual, canCompare := safeEqual(expectedValue, actualValue); canCompare {
			if isEqual {
				continue // Values are equal, no need to check ignore rules
			}
		}

		fieldName := getFieldName(newFieldPath)

		if matcher.ShouldIgnoreField(fieldName, expectedValue, actualValue, testID) {
			continue
		}

		if !e.compareJSONValues(newFieldPath, expectedValue, actualValue, matcher, testID) {
			return false
		}
	}

	// Check for extra keys in actual map
	for key := range actualMap {
		if _, exists := expectedMap[key]; !exists {
			// Check if this extra field should be ignored by field name only
			newFieldPath := key
			if fieldPath != "" {
				newFieldPath = fieldPath + "." + key
			}
			fieldName := getFieldName(newFieldPath)

			if shouldIgnore, exists := matcher.ignoreFields[strings.ToLower(fieldName)]; exists && shouldIgnore {
				log.TestLog(testID, fmt.Sprintf("🔄 Ignoring extra field '%s' (configured field name): %v", fieldName, actualMap[key]))
				continue
			}

			return false
		}
	}

	return true
}

// compareSlices compares two slice structures
func (e *Executor) compareSlices(fieldPath string, expected, actual any, matcher *DynamicFieldMatcher, testID string) bool {
	expectedSlice := reflect.ValueOf(expected)
	actualSlice := reflect.ValueOf(actual)

	if expectedSlice.Len() != actualSlice.Len() {
		return false
	}

	for i := 0; i < expectedSlice.Len(); i++ {
		expectedItem := expectedSlice.Index(i).Interface()
		actualItem := actualSlice.Index(i).Interface()

		newFieldPath := fmt.Sprintf("%s[%d]", fieldPath, i)
		if !e.compareJSONValues(newFieldPath, expectedItem, actualItem, matcher, testID) {
			return false
		}
	}

	return true
}

// getFieldName extracts the field name from a field path (e.g., "user.profile.name" -> "name")
func getFieldName(fieldPath string) string {
	if fieldPath == "" {
		return ""
	}

	// Remove array indices
	fieldPath = regexp.MustCompile(`\[\d+\]`).ReplaceAllString(fieldPath, "")

	parts := strings.Split(fieldPath, ".")
	return parts[len(parts)-1]
}

// safeEqual performs equality comparison for JSON-compatible types
// Returns (isEqual, canCompare) where canCompare indicates if direct comparison is safe
func safeEqual(a, b any) (bool, bool) {
	if a == nil || b == nil {
		return a == b, true
	}

	aType := reflect.TypeOf(a)
	bType := reflect.TypeOf(b)

	// Different types are not equal
	if aType != bType {
		return false, true
	}

	switch a.(type) {
	case string, float64, bool:
		return a == b, true
	case []any, map[string]any:
		return false, false
	default:
		return false, false
	}
}
