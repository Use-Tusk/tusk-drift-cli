package cmd

import (
	"testing"

	queryv1 "github.com/Use-Tusk/tusk-drift-schemas/generated/go/query"
)

func TestParseWhereJSONAcceptsErgonomicAccessEnums(t *testing.T) {
	where, err := parseWhereJSON(`{
		"and": [
			{
				"fields": {
					"outputValue.statusCode": {
						"gte": 500,
						"access": { "castAs": "int" }
					}
				}
			},
			{
				"fields": {
					"outputValue.body": {
						"contains": "error",
						"access": { "decode": "base64", "thenPath": "$" }
					}
				}
			}
		]
	}`)
	if err != nil {
		t.Fatalf("parseWhereJSON returned error: %v", err)
	}

	if len(where.And) != 2 {
		t.Fatalf("expected 2 nested clauses, got %d", len(where.And))
	}

	statusPredicate := where.And[0].Fields["outputValue.statusCode"]
	if statusPredicate == nil || statusPredicate.Access == nil {
		t.Fatalf("expected normalized access for outputValue.statusCode")
	}
	if got := statusPredicate.Access.CastAs; got != queryv1.CastType_CAST_TYPE_INT {
		t.Fatalf("expected castAs=%v, got %v", queryv1.CastType_CAST_TYPE_INT, got)
	}

	bodyPredicate := where.And[1].Fields["outputValue.body"]
	if bodyPredicate == nil || bodyPredicate.Access == nil {
		t.Fatalf("expected normalized access for outputValue.body")
	}
	if got := bodyPredicate.Access.Decode; got != queryv1.DecodeStrategy_DECODE_STRATEGY_BASE64 {
		t.Fatalf("expected decode=%v, got %v", queryv1.DecodeStrategy_DECODE_STRATEGY_BASE64, got)
	}
}

func TestParseWhereJSONRejectsUnknownAccessEnum(t *testing.T) {
	_, err := parseWhereJSON(`{
		"fields": {
			"outputValue.statusCode": {
				"gte": 500,
				"access": { "castAs": "bogus" }
			}
		}
	}`)
	if err == nil {
		t.Fatalf("expected parseWhereJSON to reject unknown castAs value")
	}
}
