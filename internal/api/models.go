package api

import "time"

type TestRecording struct {
	ID        string            `json:"id"`
	ServiceID string            `json:"service_id"`
	TraceID   string            `json:"trace_id"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Request   RecordedRequest   `json:"request"`
	Response  RecordedResponse  `json:"response"`
	Mocks     []MockInteraction `json:"mocks"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]any    `json:"metadata"`
}

type RecordedRequest struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    any                 `json:"body,omitempty"`
	Query   map[string][]string `json:"query,omitempty"`
}

type RecordedResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    any                 `json:"body,omitempty"`
}

type MockInteraction struct {
	Service   string           `json:"service"`
	Request   RecordedRequest  `json:"request"`
	Response  RecordedResponse `json:"response"`
	Order     int              `json:"order"`
	Timestamp time.Time        `json:"timestamp"`
}

type TestResult struct {
	TestID      string      `json:"test_id"`
	ServiceID   string      `json:"service_id"`
	Passed      bool        `json:"passed"`
	Duration    int         `json:"duration_ms"`
	Deviations  []Deviation `json:"deviations,omitempty"`
	Error       string      `json:"error,omitempty"`
	ExecutedAt  time.Time   `json:"executed_at"`
	Environment string      `json:"environment"`
}

type Deviation struct {
	Type        string `json:"type"`
	Field       string `json:"field"`
	Expected    any    `json:"expected"`
	Actual      any    `json:"actual"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}
