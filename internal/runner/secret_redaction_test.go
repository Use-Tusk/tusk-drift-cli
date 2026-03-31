package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test JWT — gitleaks reliably detects JWTs by their eyJ... structure.
const testJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

func TestRuleIDToPlaceholder(t *testing.T) {
	assert.Equal(t, "TUSK_REDACTED_JWT", ruleIDToPlaceholder("jwt"))
	assert.Equal(t, "TUSK_REDACTED_GENERIC_API_KEY", ruleIDToPlaceholder("generic-api-key"))
	assert.Equal(t, "TUSK_REDACTED_AWS_ACCESS_KEY_ID", ruleIDToPlaceholder("aws-access-key-id"))
}

func TestRedactSecrets_ShortContent(t *testing.T) {
	short := "hello"
	assert.Equal(t, short, RedactSecrets(short))
}

func TestRedactSecrets_NoSecrets(t *testing.T) {
	content := `## Request
POST /api/v1/users
Body:
{"name": "test user", "email": "test@example.com"}

## Response Diff
Status: 200 (OK)
`
	result := RedactSecrets(content)
	assert.Equal(t, content, result)
}

func TestRedactSecrets_JWT(t *testing.T) {
	content := "Authorization: Bearer " + testJWT + "\nBody:\n(empty)\n"
	result := RedactSecrets(content)
	assert.NotContains(t, result, testJWT)
	assert.Contains(t, result, "TUSK_REDACTED_JWT")
}

func TestRedactSecrets_PreservesStructure(t *testing.T) {
	content := `---
deviation_id: trace-123
endpoint: POST /api/v1/users
---

## Request
POST /api/v1/users
Body:
{"name": "test", "token": "` + testJWT + `"}

## Response Diff
Status: 200 (OK)
`
	result := RedactSecrets(content)
	// Frontmatter and structure preserved
	assert.Contains(t, result, "---")
	assert.Contains(t, result, "deviation_id: trace-123")
	assert.Contains(t, result, "## Request")
	assert.Contains(t, result, "## Response Diff")
	// Secret redacted
	assert.NotContains(t, result, testJWT)
	assert.Contains(t, result, "TUSK_REDACTED_JWT")
}
