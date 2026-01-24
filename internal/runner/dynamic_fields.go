package runner

import (
	"fmt"
	"regexp"
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
}

// NewDynamicFieldMatcher creates a new matcher with default patterns
func NewDynamicFieldMatcher() *DynamicFieldMatcher {
	return &DynamicFieldMatcher{
		uuidRegex:      regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`),
		timestampRegex: regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{3})?Z?$`),
		dateRegex:      regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$|^\d{2}\/\d{2}\/\d{4}$|^\d{2}-\d{2}-\d{4}$`),
		ignoreFields:   make(map[string]bool),
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

	// Check custom patterns - BOTH values must match the pattern
	for _, pattern := range m.customPatterns {
		if pattern.MatchString(expectedStr) && pattern.MatchString(actualStr) {
			log.TestLog(testID, fmt.Sprintf("🔄 Ignoring field '%s' (custom pattern): expected=%v, actual=%v", fieldName, expectedValue, actualValue))
			log.Debug("Field ignored by custom pattern", "field", fieldName, "expected", expectedValue, "actual", actualValue)
			return true
		}
	}

	log.Debug("Field NOT ignored", "field", fieldName, "expected", expectedValue, "actual", actualValue)
	return false
}
