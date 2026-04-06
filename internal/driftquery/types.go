package driftquery

import (
	"fmt"

	queryv1 "github.com/Use-Tusk/tusk-drift-schemas/generated/go/query"
	"google.golang.org/protobuf/types/known/structpb"
)

type (
	FieldPredicate          = queryv1.FieldPredicate
	FieldAccess             = queryv1.FieldAccess
	TimestampRange          = queryv1.TimestampRange
	SpanWhereClause         = queryv1.WhereClause
	OrderByField            = queryv1.SpanOrderBy
	MetricOrderBy           = queryv1.MetricOrderBy
	QuerySpansInput         = queryv1.QuerySpansRequest
	GetSchemaInput          = queryv1.GetSchemaRequest
	ListDistinctValuesInput = queryv1.ListDistinctValuesRequest
	AggregateSpansInput     = queryv1.AggregateSpansRequest
	GetTraceInput           = queryv1.GetTraceSpansRequest
	GetSpansByIdsInput      = queryv1.GetSpansByIdsRequest
	CompareSchemaInput      = queryv1.CompareSchemaRequest
	SchemaComparisonPeriod  = queryv1.SchemaComparisonPeriod
)

type (
	SelectableSpanField = queryv1.SelectableSpanField
	SpanSortField       = queryv1.SpanSortField
	SortDirection       = queryv1.SortDirection
	AggregateMetric     = queryv1.AggregateMetric
	AggregateGroupField = queryv1.AggregateGroupField
	TimeBucket          = queryv1.TimeBucket
)

func StringValue(v string) *structpb.Value {
	return structpb.NewStringValue(v)
}

func NumberValue(v float64) *structpb.Value {
	return structpb.NewNumberValue(v)
}

func BoolValue(v bool) *structpb.Value {
	return structpb.NewBoolValue(v)
}

func StringPtr(v string) *string {
	return &v
}

func BoolPtr(v bool) *bool {
	return &v
}

const (
	maxInt32 = int(^uint32(0) >> 1)
	minInt32 = -maxInt32 - 1
)

func Int32Ptr(name string, v int) (*int32, error) {
	if v < minInt32 || v > maxInt32 {
		return nil, fmt.Errorf("%s must be between %d and %d, got %d", name, minInt32, maxInt32, v)
	}

	x := int32(v)
	return &x, nil
}
