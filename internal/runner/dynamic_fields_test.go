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

func TestShouldIgnoreField_JWT_DefaultEnabled(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// Both values are JWTs where only jti differs - should ignore
	expectedJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjE5MWY4Y2Y0NjIwNjRkNmY5ZWYzYTJhZDMxZTEwNDJlIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9.hO9f2F0-6ewAk7FaLCjEI9zTOxAB7N3xTvsmBHhcAe0"
	actualJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjYwOTY1YmQ2MTJhZjRkNTA5ZDc1Mjk0NDgxYTc2NDExIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9._cHYQ8YcVfqQpWsuNUd38JYCjKOWXNglGocaWAiuodY"

	require.True(t, matcher.ShouldIgnoreField("access", expectedJWT, actualJWT, "test-jwt-1"))
}

func TestShouldIgnoreField_JWT_RefreshToken(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// Refresh tokens where only jti differs
	expectedJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoicmVmcmVzaCIsImV4cCI6MTc3MDM5NTU3NywiaWF0IjoxNzcwMzA5MTc3LCJqdGkiOiIyZjQzODVhYzdmZDk0ZGM3ODlmYTQ1OWUxMWEzN2Q2MSIsInVzZXJfaWQiOiIwYzcwZjA3Yy1lMDNhLTQyODAtYjlkMC0xNmZhMTY4YTYyZDEifQ.GTExDgK_QZD0GHEbIVOZLdiHMQJdN-58Z12ML-q3LMI"
	actualJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoicmVmcmVzaCIsImV4cCI6MTc3MDM5NTU3NywiaWF0IjoxNzcwMzA5MTc3LCJqdGkiOiI0N2VkMDc2YmVhN2U0M2MxOGE0Mjk3MjY1OWU2OWU3NCIsInVzZXJfaWQiOiIwYzcwZjA3Yy1lMDNhLTQyODAtYjlkMC0xNmZhMTY4YTYyZDEifQ.iedWNUBUAxJXdFbJ6uYlAowQ28esXz9D3HHIkER_glg"

	require.True(t, matcher.ShouldIgnoreField("refresh", expectedJWT, actualJWT, "test-jwt-2"))
}

func TestShouldIgnoreField_JWT_Disabled(t *testing.T) {
	falseValue := false
	cfg := &config.ComparisonConfig{
		IgnoreJWTFields: &falseValue,
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	// Even with valid JWTs, should not ignore when disabled
	expectedJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjE5MWY4Y2Y0NjIwNjRkNmY5ZWYzYTJhZDMxZTEwNDJlIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9.hO9f2F0-6ewAk7FaLCjEI9zTOxAB7N3xTvsmBHhcAe0"
	actualJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjYwOTY1YmQ2MTJhZjRkNTA5ZDc1Mjk0NDgxYTc2NDExIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9._cHYQ8YcVfqQpWsuNUd38JYCjKOWXNglGocaWAiuodY"

	require.False(t, matcher.ShouldIgnoreField("access", expectedJWT, actualJWT, "test-jwt-3"))
}

func TestShouldIgnoreField_JWT_ExplicitlyEnabled(t *testing.T) {
	trueValue := true
	cfg := &config.ComparisonConfig{
		IgnoreJWTFields: &trueValue,
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)

	expectedJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjE5MWY4Y2Y0NjIwNjRkNmY5ZWYzYTJhZDMxZTEwNDJlIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9.hO9f2F0-6ewAk7FaLCjEI9zTOxAB7N3xTvsmBHhcAe0"
	actualJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjYwOTY1YmQ2MTJhZjRkNTA5ZDc1Mjk0NDgxYTc2NDExIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9._cHYQ8YcVfqQpWsuNUd38JYCjKOWXNglGocaWAiuodY"

	require.True(t, matcher.ShouldIgnoreField("access", expectedJWT, actualJWT, "test-jwt-4"))
}

func TestShouldIgnoreField_JWT_PayloadClaimDiffers(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// JWTs where a non-dynamic claim (token_type) differs - should not ignore
	// Expected: token_type=access, Actual: token_type=refresh
	expectedJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjE5MWY4Y2Y0NjIwNjRkNmY5ZWYzYTJhZDMxZTEwNDJlIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9.hO9f2F0-6ewAk7FaLCjEI9zTOxAB7N3xTvsmBHhcAe0"
	actualJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoicmVmcmVzaCIsImV4cCI6MTc3MDM5NTU3NywiaWF0IjoxNzcwMzA5MTc3LCJqdGkiOiI0N2VkMDc2YmVhN2U0M2MxOGE0Mjk3MjY1OWU2OWU3NCIsInVzZXJfaWQiOiIwYzcwZjA3Yy1lMDNhLTQyODAtYjlkMC0xNmZhMTY4YTYyZDEifQ.iedWNUBUAxJXdFbJ6uYlAowQ28esXz9D3HHIkER_glg"

	require.False(t, matcher.ShouldIgnoreField("token", expectedJWT, actualJWT, "test-jwt-5"))
}

func TestShouldIgnoreField_JWT_NotAJWT(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// Values that look like they could have dots but are not JWTs
	require.False(t, matcher.ShouldIgnoreField("field",
		"not.a.jwt",
		"also.not.jwt",
		"test-jwt-6"))

	// One is JWT, other is not
	validJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjE5MWY4Y2Y0NjIwNjRkNmY5ZWYzYTJhZDMxZTEwNDJlIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9.hO9f2F0-6ewAk7FaLCjEI9zTOxAB7N3xTvsmBHhcAe0"
	require.False(t, matcher.ShouldIgnoreField("field",
		validJWT,
		"not-a-jwt",
		"test-jwt-6"))
}

func TestShouldIgnoreField_JWT_IdenticalTokens(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// Identical JWT strings - should be caught by the equality check before JWT logic
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjE5MWY4Y2Y0NjIwNjRkNmY5ZWYzYTJhZDMxZTEwNDJlIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9.hO9f2F0-6ewAk7FaLCjEI9zTOxAB7N3xTvsmBHhcAe0"

	// This actually passes via the equality check in compareJSONValues before
	// ShouldIgnoreField is even called, but verify the matcher also handles it
	require.True(t, matcher.ShouldIgnoreField("access", jwt, jwt, "test-jwt-7"))
}

func TestShouldIgnoreField_JWT_WithUUIDClaimDifference(t *testing.T) {
	matcher := NewDynamicFieldMatcher()

	// JWTs where user_id (a UUID with dashes) differs - UUID ignoring should handle it
	// Expected user_id: 0c70f07c-e03a-4280-b9d0-16fa168a62d1
	expectedJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwianRpIjoiYWJjMTIzIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9.sig1"
	// Actual user_id: 11111111-1111-1111-1111-111111111111
	actualJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwianRpIjoiZGVmNDU2IiwidXNlcl9pZCI6IjExMTExMTExLTExMTEtMTExMS0xMTExLTExMTExMTExMTExMSJ9.sig2"

	require.True(t, matcher.ShouldIgnoreField("access", expectedJWT, actualJWT, "test-jwt-8"))
}

func TestDecodeJWTPayload(t *testing.T) {
	// Valid JWT with known payload
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl90eXBlIjoiYWNjZXNzIiwiZXhwIjoxNzcwMzE2Mzc3LCJpYXQiOjE3NzAzMDkxNzcsImp0aSI6IjE5MWY4Y2Y0NjIwNjRkNmY5ZWYzYTJhZDMxZTEwNDJlIiwidXNlcl9pZCI6IjBjNzBmMDdjLWUwM2EtNDI4MC1iOWQwLTE2ZmExNjhhNjJkMSJ9.hO9f2F0-6ewAk7FaLCjEI9zTOxAB7N3xTvsmBHhcAe0" //nolint:gosec

	payload, err := decodeJWTPayload(token)
	require.NoError(t, err)
	require.Equal(t, "access", payload["token_type"])
	require.Equal(t, float64(1770316377), payload["exp"])
	require.Equal(t, float64(1770309177), payload["iat"])
	require.Equal(t, "191f8cf462064d6f9ef3a2ad31e1042e", payload["jti"])
	require.Equal(t, "0c70f07c-e03a-4280-b9d0-16fa168a62d1", payload["user_id"])
}

func TestDecodeJWTPayload_InvalidFormat(t *testing.T) {
	// Not a JWT - no dots
	_, err := decodeJWTPayload("not-a-jwt")
	require.Error(t, err)

	// Only two segments
	_, err = decodeJWTPayload("abc.def")
	require.Error(t, err)

	// Invalid base64 in payload
	_, err = decodeJWTPayload("eyJhbGciOiJIUzI1NiJ9.!!!invalid!!!.sig")
	require.Error(t, err)

	// Valid base64 but not JSON
	_, err = decodeJWTPayload("eyJhbGciOiJIUzI1NiJ9.bm90LWpzb24.sig")
	require.Error(t, err)
}

func TestNewDynamicFieldMatcherWithConfig_JWTDefault(t *testing.T) {
	// Default (nil config) should have JWT ignoring enabled
	matcher := NewDynamicFieldMatcherWithConfig(nil)
	require.True(t, matcher.ignoreJWT)
}

func TestNewDynamicFieldMatcherWithConfig_JWTDisabled(t *testing.T) {
	falseValue := false
	cfg := &config.ComparisonConfig{
		IgnoreJWTFields: &falseValue,
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)
	require.False(t, matcher.ignoreJWT)
}

func TestNewDynamicFieldMatcherWithConfig_JWTEnabled(t *testing.T) {
	trueValue := true
	cfg := &config.ComparisonConfig{
		IgnoreJWTFields: &trueValue,
	}
	matcher := NewDynamicFieldMatcherWithConfig(cfg)
	require.True(t, matcher.ignoreJWT)
}
