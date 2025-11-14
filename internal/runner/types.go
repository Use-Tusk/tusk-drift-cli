package runner

import core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"

type Test struct {
	FileName    string         `json:"file_name"`
	TraceID     string         `json:"trace_id"`
	TraceTestID string         `json:"trace_test_id,omitempty"`
	Spans       []*core.Span   `json:"-"`
	Type        string         `json:"type"`         // Used for test execution
	DisplayType string         `json:"display_type"` // Used for CLI display
	Timestamp   string         `json:"timestamp"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`         // Used for test execution
	DisplayName string         `json:"display_name"` // Used for CLI display
	Status      string         `json:"status"`
	Duration    int            `json:"duration"`
	Metadata    map[string]any `json:"metadata"`
	Request     Request        `json:"request"`
	Response    Response       `json:"response"`
}

type Request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body,omitempty"`
}

type Response struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body,omitempty"`
}

type TestResult struct {
	TestID            string      `json:"test_id"`
	Passed            bool        `json:"passed"`
	Cancelled         bool        `json:"cancelled"`
	CrashedServer     bool        `json:"crashed_server,omitempty"`      // Test caused server to crash
	RetriedAfterCrash bool        `json:"retried_after_crash,omitempty"` // Test was retried after batch crash
	Duration          int         `json:"duration"`                      // In milliseconds
	Deviations        []Deviation `json:"deviations,omitempty"`
	Error             string      `json:"error,omitempty"`
}

type Trace struct {
	ID        string       `json:"id"`
	Filename  string       `json:"filename"`
	Timestamp string       `json:"timestamp"`
	Spans     []*core.Span `json:"spans"`
}

type Deviation struct {
	Field       string `json:"field"`
	Expected    any    `json:"expected"`
	Actual      any    `json:"actual"`
	Description string `json:"description"`
}

type matchScope int

const (
	scopeUnknown matchScope = iota
	scopeTrace
	scopeGlobal
)
