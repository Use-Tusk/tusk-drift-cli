package runner

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
)

// DynamicFieldMatcher defines patterns for identifying dynamic fields that should be ignored
type DynamicFieldMatcher struct {
	// UUID patterns (various formats)
	uuidRegex *regexp.Regexp
	// ISO 8601 timestamp patterns
	timestampRegex *regexp.Regexp
	// Date patterns (YYYY-MM-DD, MM/DD/YYYY, etc.)
	dateRegex *regexp.Regexp
	// Custom field patterns from config
	customPatterns []*regexp.Regexp
	// Exact field names to ignore
	ignoreFields map[string]bool
	// Whether to decode and compare JWT tokens by payload
	ignoreJWT bool
	// Whether to ignore numeric epoch timestamps (seconds and milliseconds)
	ignoreEpoch bool
}

// jwtRegex matches the general JWT format: three base64url segments separated by dots.
// JWT headers always start with "eyJ" (base64url for '{"'), which reduces false positives.
var jwtRegex = regexp.MustCompile(`^eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*$`)

// jwtDynamicClaims are JWT claims that are inherently dynamic per-issuance
// and should be automatically ignored when comparing JWT payloads.
var jwtDynamicClaims = map[string]bool{
	"jti": true, // JWT ID - unique per token issuance
}

// Epoch timestamp range boundaries.
// Seconds:      1_000_000_000 (Sep 2001) to 4_102_444_800 (Jan 2100)
// Milliseconds: 1_000_000_000_000        to 4_102_444_800_000
const (
	epochSecondsMin = 1_000_000_000
	epochSecondsMax = 4_102_444_800
	epochMillisMin  = 1_000_000_000_000
	epochMillisMax  = 4_102_444_800_000
)

// NewDynamicFieldMatcher creates a new matcher with default patterns
func NewDynamicFieldMatcher() *DynamicFieldMatcher {
	return &DynamicFieldMatcher{
		uuidRegex:      regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`),
		timestampRegex: regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{3})?Z?$`),
		dateRegex:      regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$|^\d{2}\/\d{2}\/\d{4}$|^\d{2}-\d{2}-\d{4}$`),
		ignoreFields:   make(map[string]bool),
		ignoreJWT:      true,
		ignoreEpoch:    true,
	}
}

func NewDynamicFieldMatcherWithConfig(cfg *config.ComparisonConfig) *DynamicFieldMatcher {
	matcher := NewDynamicFieldMatcher()

	if cfg != nil {
		// Check if UUID ignoring is explicitly disabled
		if cfg.IgnoreUUIDs != nil && !*cfg.IgnoreUUIDs {
			matcher.uuidRegex = nil
		}
		// Check if timestamp ignoring is explicitly disabled
		if cfg.IgnoreTimestamps != nil && !*cfg.IgnoreTimestamps {
			matcher.timestampRegex = nil
		}
		// Check if date ignoring is explicitly disabled
		if cfg.IgnoreDates != nil && !*cfg.IgnoreDates {
			matcher.dateRegex = nil
		}
		// Check if JWT ignoring is explicitly disabled
		if cfg.IgnoreJWTFields != nil && !*cfg.IgnoreJWTFields {
			matcher.ignoreJWT = false
		}
		// Check if epoch timestamp ignoring is explicitly disabled
		if cfg.IgnoreEpochTimestamps != nil && !*cfg.IgnoreEpochTimestamps {
			matcher.ignoreEpoch = false
		}

		// Add custom field names
		for _, field := range cfg.IgnoreFields {
			matcher.ignoreFields[strings.ToLower(field)] = true
		}

		// Add custom patterns
		for _, pattern := range cfg.IgnorePatterns {
			if compiled, err := regexp.Compile(pattern); err == nil {
				matcher.customPatterns = append(matcher.customPatterns, compiled)
			}
		}
	}

	return matcher
}

// ShouldIgnoreField determines if a field should be ignored during comparison
func (m *DynamicFieldMatcher) ShouldIgnoreField(fieldName string, expectedValue, actualValue any, testID string) bool {
	// Check exact field names first
	if shouldIgnore, exists := m.ignoreFields[strings.ToLower(fieldName)]; exists && shouldIgnore {
		log.TestLog(testID, fmt.Sprintf("🔄 Ignoring field '%s' (configured field name): expected=%v, actual=%v", fieldName, expectedValue, actualValue))
		log.Debug("Field ignored by name match", "field", fieldName, "expected", expectedValue, "actual", actualValue)
		return true
	}

	// Convert both values to strings for pattern matching
	expectedStr := fmt.Sprintf("%v", expectedValue)
	actualStr := fmt.Sprintf("%v", actualValue)

	// Check for UUID pattern - BOTH values must be UUIDs
	if m.uuidRegex != nil && m.uuidRegex.MatchString(expectedStr) && m.uuidRegex.MatchString(actualStr) {
		log.TestLog(testID, fmt.Sprintf("🔄 Ignoring field '%s' (UUID pattern): expected=%v, actual=%v", fieldName, expectedValue, actualValue))
		log.Debug("Field ignored by UUID pattern", "field", fieldName, "expected", expectedValue, "actual", actualValue)
		return true
	}

	// Check for timestamp pattern - BOTH values must be timestamps
	if m.timestampRegex != nil && m.timestampRegex.MatchString(expectedStr) && m.timestampRegex.MatchString(actualStr) {
		log.TestLog(testID, fmt.Sprintf("🔄 Ignoring field '%s' (timestamp pattern): expected=%v, actual=%v", fieldName, expectedValue, actualValue))
		log.Debug("Field ignored by timestamp pattern", "field", fieldName, "expected", expectedValue, "actual", actualValue)
		return true
	}

	// Check for date pattern - BOTH values must be dates
	if m.dateRegex != nil && m.dateRegex.MatchString(expectedStr) && m.dateRegex.MatchString(actualStr) {
		log.TestLog(testID, fmt.Sprintf("🔄 Ignoring field '%s' (date pattern): expected=%v, actual=%v", fieldName, expectedValue, actualValue))
		log.Debug("Field ignored by date pattern", "field", fieldName, "expected", expectedValue, "actual", actualValue)
		return true
	}

	// Check for numeric epoch timestamps (Unix seconds or milliseconds) - BOTH values must be in range
	if m.ignoreEpoch && isEpochTimestamp(expectedValue) && isEpochTimestamp(actualValue) {
		log.TestLog(testID, fmt.Sprintf("🔄 Ignoring field '%s' (epoch timestamp): expected=%v, actual=%v", fieldName, expectedValue, actualValue))
		log.Debug("Field ignored by epoch timestamp range", "field", fieldName, "expected", expectedValue, "actual", actualValue)
		return true
	}

	// Check custom patterns - BOTH values must match the pattern
	for _, pattern := range m.customPatterns {
		if pattern.MatchString(expectedStr) && pattern.MatchString(actualStr) {
			log.TestLog(testID, fmt.Sprintf("🔄 Ignoring field '%s' (custom pattern): expected=%v, actual=%v", fieldName, expectedValue, actualValue))
			log.Debug("Field ignored by custom pattern", "field", fieldName, "expected", expectedValue, "actual", actualValue)
			return true
		}
	}

	// Check for JWT tokens - decode payloads and compare claims
	if m.ignoreJWT && m.shouldIgnoreJWT(expectedStr, actualStr, testID, fieldName) {
		return true
	}

	log.Debug("Field NOT ignored", "field", fieldName, "expected", expectedValue, "actual", actualValue)
	return false
}

// shouldIgnoreJWT checks if both values are JWT tokens whose payloads match
// after ignoring known dynamic claims (like jti) and applying pattern matching.
// JWT signatures are derived from the payload content + secret, so they are
// expected to differ when any claim differs - we only compare payloads.
func (m *DynamicFieldMatcher) shouldIgnoreJWT(expectedStr, actualStr, testID, fieldName string) bool {
	if !jwtRegex.MatchString(expectedStr) || !jwtRegex.MatchString(actualStr) {
		return false
	}

	expectedPayload, err := decodeJWTPayload(expectedStr)
	if err != nil {
		log.Debug("Failed to decode expected JWT payload", "field", fieldName, "error", err)
		return false
	}
	actualPayload, err := decodeJWTPayload(actualStr)
	if err != nil {
		log.Debug("Failed to decode actual JWT payload", "field", fieldName, "error", err)
		return false
	}

	if len(expectedPayload) != len(actualPayload) {
		log.Debug("JWT payload key count mismatch", "field", fieldName, "expected_keys", len(expectedPayload), "actual_keys", len(actualPayload))
		return false
	}

	// Compare each claim in the expected payload
	for key, expectedVal := range expectedPayload {
		actualVal, exists := actualPayload[key]
		if !exists {
			return false
		}

		if reflect.DeepEqual(expectedVal, actualVal) {
			continue
		}

		if jwtDynamicClaims[key] {
			log.TestLog(testID, fmt.Sprintf("🔄 Ignoring JWT claim '%s' in field '%s' (known dynamic JWT claim): expected=%v, actual=%v", key, fieldName, expectedVal, actualVal))
			continue
		}

		if m.ShouldIgnoreField(key, expectedVal, actualVal, testID) {
			continue
		}

		log.Debug("JWT payload claim mismatch", "field", fieldName, "claim", key, "expected", expectedVal, "actual", actualVal)
		return false
	}

	// Check for extra keys in actual payload
	for key := range actualPayload {
		if _, exists := expectedPayload[key]; !exists {
			return false
		}
	}

	log.TestLog(testID, fmt.Sprintf("🔄 Ignoring field '%s' (JWT tokens with matching payload after ignoring dynamic claims)", fieldName))
	log.Debug("Field ignored by JWT payload comparison", "field", fieldName)
	return true
}

// decodeJWTPayload decodes the payload (second segment) of a JWT token
// and returns it as a map. Returns an error if the token is malformed.
func decodeJWTPayload(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	// Decode the payload using base64url without padding (standard JWT encoding)
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to base64url decode JWT payload: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse JWT payload as JSON: %w", err)
	}

	return payload, nil
}

// isEpochTimestamp returns true if the value is a number in a plausible
// Unix timestamp range — either seconds (10-digit) or milliseconds (13-digit).
// JSON numbers arrive as float64 from encoding/json; integer types and
// numeric strings (e.g. "1773268785165") are also handled.
func isEpochTimestamp(v any) bool {
	var n float64
	switch val := v.(type) {
	case float64:
		n = val
	case float32:
		n = float64(val)
	case int:
		n = float64(val)
	case int64:
		n = float64(val)
	case json.Number:
		f, err := val.Float64()
		if err != nil {
			return false
		}
		n = f
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		n = f
	default:
		return false
	}

	// Reject fractional numbers — real epoch timestamps are always whole numbers
	if n != float64(int64(n)) {
		return false
	}

	return (n >= epochSecondsMin && n <= epochSecondsMax) ||
		(n >= epochMillisMin && n <= epochMillisMax)
}
