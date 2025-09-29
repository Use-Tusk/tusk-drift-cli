package runner

import (
	"testing"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewDynamicFieldMatcher(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	require.NotNil(t, matcher.uuidRegex)
	require.NotNil(t, matcher.timestampRegex)
	require.NotNil(t, matcher.dateRegex)
	require.Empty(t, matcher.customPatterns)
	require.NotNil(t, matcher.ignoreFields)
	require.Empty(t, matcher.ignoreFields)
}

func TestNewDynamicFieldMatcherWithConfig_DefaultBehavior(t *testing.T) {
	// Test with nil config - should behave like NewDynamicFieldMatcher
	matcher := NewDynamicFieldMatcherWithConfig(nil)

	require.NotNil(t, matcher.uuidRegex)
	require.NotNil(t, matcher.timestampRegex)
	require.NotNil(t, matcher.dateRegex)
	require.Empty(t, matcher.customPatterns)
	require.Empty(t, matcher.ignoreFields)
}

func TestNewDynamicFieldMatcherWithConfig_DisablePatterns(t *testing.T) {
	falseValue := false
	cfg := &config.ComparisonConfig{
		IgnoreUUIDs:      &falseValue,
		IgnoreTimestamps: &falseValue,
		IgnoreDates:      &falseValue,
	}

	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	require.Nil(t, matcher.uuidRegex)
	require.Nil(t, matcher.timestampRegex)
	require.Nil(t, matcher.dateRegex)
}

func TestNewDynamicFieldMatcherWithConfig_EnablePatterns(t *testing.T) {
	trueValue := true
	cfg := &config.ComparisonConfig{
		IgnoreUUIDs:      &trueValue,
		IgnoreTimestamps: &trueValue,
		IgnoreDates:      &trueValue,
	}

	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	require.NotNil(t, matcher.uuidRegex)
	require.NotNil(t, matcher.timestampRegex)
	require.NotNil(t, matcher.dateRegex)
}

func TestNewDynamicFieldMatcherWithConfig_IgnoreFields(t *testing.T) {
	cfg := &config.ComparisonConfig{
		IgnoreFields: []string{"traceId", "requestId", "TIMESTAMP"},
	}

	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	require.True(t, matcher.ignoreFields["traceid"])
	require.True(t, matcher.ignoreFields["requestid"])
	require.True(t, matcher.ignoreFields["timestamp"])
	require.False(t, matcher.ignoreFields["otherId"])
}

func TestNewDynamicFieldMatcherWithConfig_CustomPatterns(t *testing.T) {
	cfg := &config.ComparisonConfig{
		IgnorePatterns: []string{`^temp_\d+$`, `^session_[a-f0-9]+$`, `invalid_regex[`},
	}

	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	// Should have 2 valid patterns (invalid regex is skipped)
	require.Len(t, matcher.customPatterns, 2)
}

func TestShouldIgnoreField_ExactFieldNames(t *testing.T) {
	cfg := &config.ComparisonConfig{
		IgnoreFields: []string{"traceId", "requestId"},
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	// Case insensitive matching
	require.True(t, matcher.ShouldIgnoreField("traceId", "abc", "def", "test-1"))
	require.True(t, matcher.ShouldIgnoreField("TRACEID", "abc", "def", "test-1"))
	require.True(t, matcher.ShouldIgnoreField("requestid", "abc", "def", "test-1"))

	// Field not in ignore list
	require.False(t, matcher.ShouldIgnoreField("userId", "abc", "def", "test-1"))
}

func TestShouldIgnoreField_UUIDPattern(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// Both values are UUIDs - should ignore
	require.True(t, matcher.ShouldIgnoreField("id",
		"550e8400-e29b-41d4-a716-446655440000",
		"123e4567-e89b-12d3-a456-426614174000",
		"test-1"))

	// Only one value is UUID - should not ignore
	require.False(t, matcher.ShouldIgnoreField("id",
		"550e8400-e29b-41d4-a716-446655440000",
		"not-a-uuid",
		"test-1"))

	// Neither value is UUID - should not ignore
	require.False(t, matcher.ShouldIgnoreField("id",
		"not-a-uuid",
		"also-not-uuid",
		"test-1"))

	// Invalid UUID formats - should not ignore
	require.False(t, matcher.ShouldIgnoreField("id",
		"550e8400-e29b-41d4-a716-44665544000", // too short
		"123e4567-e89b-12d3-a456-426614174000",
		"test-1"))
}

func TestShouldIgnoreField_UUIDPattern_Disabled(t *testing.T) {
	falseValue := false
	cfg := &config.ComparisonConfig{
		IgnoreUUIDs: &falseValue,
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	// Even with valid UUIDs, should not ignore when disabled
	require.False(t, matcher.ShouldIgnoreField("id",
		"550e8400-e29b-41d4-a716-446655440000",
		"123e4567-e89b-12d3-a456-426614174000",
		"test-1"))
}

func TestShouldIgnoreField_TimestampPattern(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// Both values are ISO 8601 timestamps - should ignore
	require.True(t, matcher.ShouldIgnoreField("createdAt",
		"2023-01-01T00:00:00Z",
		"2024-02-02T12:34:56Z",
		"test-1"))

	// With milliseconds
	require.True(t, matcher.ShouldIgnoreField("updatedAt",
		"2023-01-01T00:00:00.123Z",
		"2024-02-02T12:34:56.789Z",
		"test-1"))

	// Without Z suffix
	require.True(t, matcher.ShouldIgnoreField("timestamp",
		"2023-01-01T00:00:00",
		"2024-02-02T12:34:56",
		"test-1"))

	// Only one value is timestamp - should not ignore
	require.False(t, matcher.ShouldIgnoreField("createdAt",
		"2023-01-01T00:00:00Z",
		"not-a-timestamp",
		"test-1"))

	// Invalid timestamp format - should not ignore
	require.False(t, matcher.ShouldIgnoreField("createdAt",
		"2023-01-01 00:00:00", // space instead of T
		"2024-02-02T12:34:56Z",
		"test-1"))
}

func TestShouldIgnoreField_TimestampPattern_Disabled(t *testing.T) {
	falseValue := false
	cfg := &config.ComparisonConfig{
		IgnoreTimestamps: &falseValue,
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	// Even with valid timestamps, should not ignore when disabled
	require.False(t, matcher.ShouldIgnoreField("createdAt",
		"2023-01-01T00:00:00Z",
		"2024-02-02T12:34:56Z",
		"test-1"))
}

func TestShouldIgnoreField_DatePattern(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// YYYY-MM-DD format
	require.True(t, matcher.ShouldIgnoreField("birthDate",
		"2023-01-01",
		"1990-12-31",
		"test-1"))

	// MM/DD/YYYY format
	require.True(t, matcher.ShouldIgnoreField("eventDate",
		"01/01/2023",
		"12/31/1990",
		"test-1"))

	// MM-DD-YYYY format
	require.True(t, matcher.ShouldIgnoreField("startDate",
		"01-01-2023",
		"12-31-1990",
		"test-1"))

	// Only one value is date - should not ignore
	require.False(t, matcher.ShouldIgnoreField("birthDate",
		"2023-01-01",
		"not-a-date",
		"test-1"))

	// Invalid date format - should not ignore
	require.False(t, matcher.ShouldIgnoreField("birthDate",
		"Jan 1, 2023",
		"Dec 31, 1990",
		"test-1"))
}

func TestShouldIgnoreField_DatePattern_Disabled(t *testing.T) {
	falseValue := false
	cfg := &config.ComparisonConfig{
		IgnoreDates: &falseValue,
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	// Even with valid dates, should not ignore when disabled
	require.False(t, matcher.ShouldIgnoreField("birthDate",
		"2023-01-01",
		"1990-12-31",
		"test-1"))
}

func TestShouldIgnoreField_CustomPatterns(t *testing.T) {
	cfg := &config.ComparisonConfig{
		IgnorePatterns: []string{`^temp_\d+$`, `^session_[a-f0-9]+$`},
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	// Both values match first pattern
	require.True(t, matcher.ShouldIgnoreField("tempId",
		"temp_123",
		"temp_456",
		"test-1"))

	// Both values match second pattern
	require.True(t, matcher.ShouldIgnoreField("sessionId",
		"session_abc123",
		"session_def456",
		"test-1"))

	// Only one value matches pattern - should not ignore
	require.False(t, matcher.ShouldIgnoreField("tempId",
		"temp_123",
		"not_temp",
		"test-1"))

	// Neither value matches pattern - should not ignore
	require.False(t, matcher.ShouldIgnoreField("otherId",
		"other_123",
		"other_456",
		"test-1"))
}

func TestShouldIgnoreField_MultipleRules(t *testing.T) {
	cfg := &config.ComparisonConfig{
		IgnoreFields:   []string{"traceId"},
		IgnorePatterns: []string{`^temp_\d+$`},
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	// Should ignore by field name
	require.True(t, matcher.ShouldIgnoreField("traceId",
		"any-value",
		"other-value",
		"test-1"))

	// Should ignore by custom pattern
	require.True(t, matcher.ShouldIgnoreField("tempField",
		"temp_123",
		"temp_456",
		"test-1"))

	// Should ignore by UUID pattern (default)
	require.True(t, matcher.ShouldIgnoreField("id",
		"550e8400-e29b-41d4-a716-446655440000",
		"123e4567-e89b-12d3-a456-426614174000",
		"test-1"))
}

func TestShouldIgnoreField_NonStringValues(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// Numeric values that look like dates when converted to string
	require.False(t, matcher.ShouldIgnoreField("year",
		2023,
		2024,
		"test-1"))

	// Boolean values
	require.False(t, matcher.ShouldIgnoreField("active",
		true,
		false,
		"test-1"))

	// Nil values
	require.False(t, matcher.ShouldIgnoreField("optional",
		nil,
		nil,
		"test-1"))
}
