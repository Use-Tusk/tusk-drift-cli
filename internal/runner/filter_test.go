package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterTestsRequiresFieldedPattern(t *testing.T) {
	_, err := FilterTests(nil, "status")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-fielded filters")
}

func TestFilterTestsFiltersByMultipleFields(t *testing.T) {
	tests := []Test{
		{
			Path:        "/graphql/users",
			DisplayType: "GRAPHQL",
			DisplayName: "query GetUsers",
			Status:      "PASSED",
			Method:      "GET",
			FileName:    "users.graphql",
			TraceID:     "trace-1",
		},
		{
			Path:        "/graphql/users",
			DisplayType: "GRAPHQL",
			DisplayName: "mutation UpdateUser",
			Status:      "FAILED",
			Method:      "POST",
			FileName:    "users.graphql",
			TraceID:     "trace-2",
		},
		{
			Path:     "/rest/users",
			Type:     "HTTP",
			Method:   "GET",
			Status:   "PASSED",
			FileName: "users_rest.json",
			TraceID:  "trace-3",
		},
	}

	filtered, err := FilterTests(tests, `type=^GRAPHQL$,op=^GetUsers$`)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, tests[0], filtered[0])
}

func TestFilterTestsHandlesQuotedValues(t *testing.T) {
	tests := []Test{
		{
			DisplayName: "Friendly,Name",
			DisplayType: "HTTP",
			Status:      "PASSED",
			Path:        "/friendly",
			Method:      "GET",
		},
	}

	filtered, err := FilterTests(tests, `name="Friendly,Name",status=^PASSED$`)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "Friendly,Name", filtered[0].DisplayName)
}

func TestFilterTestsUsesTypeFallback(t *testing.T) {
	tests := []Test{
		{
			Type:   "HTTP",
			Path:   "/rest",
			Method: "GET",
			Status: "PASSED",
		},
	}

	filtered, err := FilterTests(tests, `type=^HTTP$`)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
}

func TestFilterTestsUnknownFieldError(t *testing.T) {
	_, err := FilterTests(nil, "unknown=foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown filter field")
}

func TestParseFieldedFilterTrimsQuotes(t *testing.T) {
	matchers, err := parseFieldedFilter(`name="^Friendly,Name$",status='^(PASS|FAIL)$'`)
	require.NoError(t, err)
	require.Len(t, matchers, 2)

	assert.Equal(t, "name", matchers[0].field)
	assert.Equal(t, "^Friendly,Name$", matchers[0].re.String())

	assert.Equal(t, "status", matchers[1].field)
	assert.Equal(t, "^(PASS|FAIL)$", matchers[1].re.String())
}

func TestParseFieldedFilterInvalidRegex(t *testing.T) {
	_, err := parseFieldedFilter("type=^(")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex for type")
}

func TestSplitCommaAwareHandlesQuotedCommas(t *testing.T) {
	tokens := splitCommaAware(`type=REST,name="Friendly,Name",status=PASSED`)
	require.Equal(t, []string{"type=REST", `name="Friendly,Name"`, "status=PASSED"}, tokens)
}

func TestGetFieldValueForFilter(t *testing.T) {
	graphQLTest := Test{
		Path:        "/graphql/users",
		DisplayName: "query GetUsers",
		DisplayType: "GRAPHQL",
		Type:        "HTTP",
		Method:      "GET",
		Status:      "PASSED",
		TraceID:     "trace-1",
		FileName:    "users.graphql",
	}

	assert.Equal(t, "/graphql/users", getFieldValueForFilter(graphQLTest, "path"))
	assert.Equal(t, "query GetUsers", getFieldValueForFilter(graphQLTest, "name"))
	assert.Equal(t, "GetUsers", getFieldValueForFilter(graphQLTest, "op"))
	assert.Equal(t, "GRAPHQL", getFieldValueForFilter(graphQLTest, "type"))
	assert.Equal(t, "GET", getFieldValueForFilter(graphQLTest, "method"))
	assert.Equal(t, "PASSED", getFieldValueForFilter(graphQLTest, "status"))
	assert.Equal(t, "trace-1", getFieldValueForFilter(graphQLTest, "id"))
	assert.Equal(t, "users.graphql", getFieldValueForFilter(graphQLTest, "file"))

	fallbackType := Test{Type: "REST"}
	assert.Equal(t, "REST", getFieldValueForFilter(fallbackType, "type"))

	assert.Equal(t, "", getFieldValueForFilter(Test{}, "unknown"))
}

func TestExtractGraphQLOperationName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"query GetUser", "GetUser"},
		{"mutation UpdateUser", "UpdateUser"},
		{"subscription OnEvent", "OnEvent"},
		{"plainName", "plainName"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, extractGraphQLOperationName(tc.input))
		})
	}
}
