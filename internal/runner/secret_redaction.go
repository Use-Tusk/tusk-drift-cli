package runner

import (
	"sort"
	"strings"
	"sync"

	"github.com/zricethezav/gitleaks/v8/detect"
)

const minScannableLength = 20

var (
	detectorOnce sync.Once
	detectorInst *detect.Detector
	detectorErr  error
)

// getDetector returns a singleton gitleaks detector with default config.
func getDetector() (*detect.Detector, error) {
	detectorOnce.Do(func() {
		detectorInst, detectorErr = detect.NewDetectorDefaultConfig()
	})
	return detectorInst, detectorErr
}

// ruleIDToPlaceholder converts a gitleaks rule ID to a redaction placeholder.
// Matches the backend format: "jwt" -> "TUSK_REDACTED_JWT"
func ruleIDToPlaceholder(ruleID string) string {
	upper := strings.ToUpper(ruleID)
	upper = strings.ReplaceAll(upper, "-", "_")
	return "TUSK_REDACTED_" + upper
}

// redactSecrets scans content for secrets using gitleaks and replaces them
// with TUSK_REDACTED_{RULE_ID} placeholders. Returns the original content
// unchanged if detection fails (graceful fallback).
func RedactSecrets(content string) string {
	if len(content) < minScannableLength {
		return content
	}

	d, err := getDetector()
	if err != nil {
		return content
	}

	findings := d.DetectString(content)
	if len(findings) == 0 {
		return content
	}

	// Sort by secret length descending to handle substring cases correctly.
	// Same approach as backend's redactSecretsFromString.
	sort.Slice(findings, func(i, j int) bool {
		return len(findings[i].Secret) > len(findings[j].Secret)
	})

	redacted := content
	for _, f := range findings {
		placeholder := ruleIDToPlaceholder(f.RuleID)
		redacted = strings.ReplaceAll(redacted, f.Secret, placeholder)
	}

	return redacted
}
