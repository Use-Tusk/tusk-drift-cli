package runner

// EvalFile represents the top-level JSON file structure.
type EvalFile struct {
	Examples []EvalExample `json:"examples"`
}

// EvalExample is a self-contained matching scenario.
type EvalExample struct {
	ID          string        `json:"id"`
	Description string        `json:"description"`
	Config      EvalConfig    `json:"config"`
	TraceMocks  []EvalSpan    `json:"traceMocks"`
	SuiteMocks  []EvalSpan    `json:"suiteMocks"`
	GlobalMocks []EvalSpan    `json:"globalMocks"`
	Requests    []EvalRequest `json:"requests"`
}

// EvalConfig holds per-example configuration for the MockMatcher.
type EvalConfig struct {
	AllowSuiteWideMatching bool `json:"allowSuiteWideMatching"`
}

// EvalSpan represents a recorded span (mock) in simplified JSON form.
type EvalSpan struct {
	SpanId          string          `json:"spanId"`
	TraceId         string          `json:"traceId"`
	PackageName     string          `json:"packageName"`
	SubmoduleName   string          `json:"submoduleName,omitempty"`
	Name            string          `json:"name,omitempty"`
	InputValue      map[string]any  `json:"inputValue,omitempty"`
	InputSchema     *JsonSchemaEval `json:"inputSchema,omitempty"`
	InputValueHash  string          `json:"inputValueHash,omitempty"`
	InputSchemaHash string          `json:"inputSchemaHash,omitempty"`
	IsPreAppStart   bool            `json:"isPreAppStart,omitempty"`
	Timestamp       string          `json:"timestamp,omitempty"`
}

// EvalRequest pairs a request with its expected outcome.
type EvalRequest struct {
	Request  EvalRequestData `json:"request"`
	Expected EvalExpected    `json:"expected"`
}

// EvalRequestData describes the mock request to send.
type EvalRequestData struct {
	PackageName     string          `json:"packageName"`
	SubmoduleName   string          `json:"submoduleName,omitempty"`
	Name            string          `json:"name,omitempty"`
	InputValue      map[string]any  `json:"inputValue,omitempty"`
	InputSchema     *JsonSchemaEval `json:"inputSchema,omitempty"`
	InputValueHash  string          `json:"inputValueHash,omitempty"`
	InputSchemaHash string          `json:"inputSchemaHash,omitempty"`
	IsPreAppStart   bool            `json:"isPreAppStart,omitempty"`
	Operation       string          `json:"operation,omitempty"`
	TraceId         string          `json:"traceId,omitempty"`
}

// EvalExpected describes the expected matching result.
type EvalExpected struct {
	MatchedSpanId *string `json:"matchedSpanId"`
	MatchType     string  `json:"matchType"`
	MatchScope    string  `json:"matchScope"`
}

// JsonSchemaEval is a simplified JSON-friendly representation of core.JsonSchema.
type JsonSchemaEval struct {
	Type            string                     `json:"type,omitempty"`
	Properties      map[string]*JsonSchemaEval `json:"properties,omitempty"`
	Items           *JsonSchemaEval            `json:"items,omitempty"`
	MatchImportance *float64                   `json:"matchImportance,omitempty"`
}
