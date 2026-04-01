package driftquery

// Filter types matching drift-mcp/src/types.ts

type StringFilter struct {
	Eq         *string  `json:"eq,omitempty"`
	Neq        *string  `json:"neq,omitempty"`
	In         []string `json:"in,omitempty"`
	Contains   *string  `json:"contains,omitempty"`
	StartsWith *string  `json:"startsWith,omitempty"`
	EndsWith   *string  `json:"endsWith,omitempty"`
}

type NumberFilter struct {
	Eq  *float64 `json:"eq,omitempty"`
	Neq *float64 `json:"neq,omitempty"`
	Gt  *float64 `json:"gt,omitempty"`
	Gte *float64 `json:"gte,omitempty"`
	Lt  *float64 `json:"lt,omitempty"`
	Lte *float64 `json:"lte,omitempty"`
}

type BooleanFilter struct {
	Eq bool `json:"eq"`
}

type SpanWhereClause struct {
	Name                *StringFilter     `json:"name,omitempty"`
	PackageName         *StringFilter     `json:"packageName,omitempty"`
	InstrumentationName *StringFilter     `json:"instrumentationName,omitempty"`
	Environment         *StringFilter     `json:"environment,omitempty"`
	TraceID             *StringFilter     `json:"traceId,omitempty"`
	SpanID              *StringFilter     `json:"spanId,omitempty"`
	Duration            *NumberFilter     `json:"duration,omitempty"`
	IsRootSpan          *BooleanFilter    `json:"isRootSpan,omitempty"`
	AND                 []SpanWhereClause `json:"AND,omitempty"`
	OR                  []SpanWhereClause `json:"OR,omitempty"`
}

type JsonbFilter struct {
	Column       string   `json:"column"`
	JsonPath     string   `json:"jsonPath"`
	Eq           any      `json:"eq,omitempty"`
	Neq          any      `json:"neq,omitempty"`
	Gt           *float64 `json:"gt,omitempty"`
	Gte          *float64 `json:"gte,omitempty"`
	Lt           *float64 `json:"lt,omitempty"`
	Lte          *float64 `json:"lte,omitempty"`
	Contains     *string  `json:"contains,omitempty"`
	StartsWith   *string  `json:"startsWith,omitempty"`
	EndsWith     *string  `json:"endsWith,omitempty"`
	IsNull       *bool    `json:"isNull,omitempty"`
	In           []any    `json:"in,omitempty"`
	CastAs       *string  `json:"castAs,omitempty"`
	DecodeBase64 *bool    `json:"decodeBase64,omitempty"`
	ThenPath     *string  `json:"thenPath,omitempty"`
}

// Tool input types

type OrderByField struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type MetricOrderBy struct {
	Metric    string `json:"metric"`
	Direction string `json:"direction"`
}

type QuerySpansInput struct {
	ObservableServiceID string           `json:"observableServiceId"`
	Where               *SpanWhereClause `json:"where,omitempty"`
	JsonbFilters        []JsonbFilter    `json:"jsonbFilters,omitempty"`
	OrderBy             []OrderByField   `json:"orderBy,omitempty"`
	Limit               int              `json:"limit"`
	Offset              int              `json:"offset"`
	IncludeInputOutput  bool             `json:"includeInputOutput"`
	MaxPayloadLength    int              `json:"maxPayloadLength"`
}

type GetSchemaInput struct {
	ObservableServiceID string  `json:"observableServiceId"`
	PackageName         *string `json:"packageName,omitempty"`
	InstrumentationName *string `json:"instrumentationName,omitempty"`
	Name                *string `json:"name,omitempty"`
	ShowExample         bool    `json:"showExample"`
	MaxPayloadLength    int     `json:"maxPayloadLength"`
}

type ListDistinctValuesInput struct {
	ObservableServiceID string           `json:"observableServiceId"`
	Field               string           `json:"field"`
	Where               *SpanWhereClause `json:"where,omitempty"`
	JsonbFilters        []JsonbFilter    `json:"jsonbFilters,omitempty"`
	Limit               int              `json:"limit"`
}

type AggregateSpansInput struct {
	ObservableServiceID string           `json:"observableServiceId"`
	Where               *SpanWhereClause `json:"where,omitempty"`
	GroupBy             []string         `json:"groupBy,omitempty"`
	Metrics             []string         `json:"metrics"`
	TimeBucket          *string          `json:"timeBucket,omitempty"`
	OrderBy             *MetricOrderBy   `json:"orderBy,omitempty"`
	Limit               int              `json:"limit"`
}

type GetTraceInput struct {
	ObservableServiceID string `json:"observableServiceId"`
	TraceID             string `json:"traceId"`
	IncludePayloads     bool   `json:"includePayloads"`
	MaxPayloadLength    int    `json:"maxPayloadLength"`
}

type GetSpansByIdsInput struct {
	ObservableServiceID string   `json:"observableServiceId"`
	IDs                 []string `json:"ids"`
	IncludePayloads     bool     `json:"includePayloads"`
	MaxPayloadLength    int      `json:"maxPayloadLength"`
}
