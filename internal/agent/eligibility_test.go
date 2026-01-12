package agent

import (
	"testing"
)

func TestValidateEligibilityReport_Valid(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusCompatible,
				StatusReasoning: "Node.js service with Express. All dependencies supported.",
				Runtime:         RuntimeNodeJS,
				Framework:       "express",
				SupportedPackages: &PackageInfo{
					Packages:  []string{"pg@8.11.0"},
					Reasoning: "pg is in manifest with version 8.*",
				},
			},
		},
		Summary: EligibilitySummary{
			TotalServices:       1,
			Compatible:          1,
			PartiallyCompatible: 0,
			NotCompatible:       0,
		},
	}

	err := ValidateEligibilityReport(report)
	if err != nil {
		t.Errorf("expected valid report, got error: %v", err)
	}
}

func TestValidateEligibilityReport_MultipleServices(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusCompatible,
				StatusReasoning: "All dependencies supported.",
				Runtime:         RuntimeNodeJS,
				Framework:       "express",
			},
			"./services/auth": {
				Status:          StatusPartiallyCompatible,
				StatusReasoning: "Some dependencies not instrumented.",
				Runtime:         RuntimePython,
				Framework:       "fastapi",
				UnsupportedPackages: &PackageInfo{
					Packages:  []string{"redis==5.0.0"},
					Reasoning: "Redis 5.x not in Python SDK manifest",
				},
			},
			"./services/legacy": {
				Status:          StatusNotCompatible,
				StatusReasoning: "Go is not currently supported.",
				Runtime:         RuntimeOther,
				Framework:       "gin",
			},
		},
		Summary: EligibilitySummary{
			TotalServices:       3,
			Compatible:          1,
			PartiallyCompatible: 1,
			NotCompatible:       1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err != nil {
		t.Errorf("expected valid report, got error: %v", err)
	}
}

func TestValidateEligibilityReport_NilServices(t *testing.T) {
	report := &EligibilityReport{
		Services: nil,
		Summary: EligibilitySummary{
			TotalServices: 0,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for nil services, got nil")
	}
}

func TestValidateEligibilityReport_EmptyServicesAllowed(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{},
		Summary: EligibilitySummary{
			TotalServices:       0,
			Compatible:          0,
			PartiallyCompatible: 0,
			NotCompatible:       0,
		},
	}

	err := ValidateEligibilityReport(report)
	if err != nil {
		t.Errorf("expected empty services to be valid, got error: %v", err)
	}
}

func TestValidateEligibilityReport_EmptyPath(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"": {
				Status:          StatusCompatible,
				StatusReasoning: "Valid service.",
				Runtime:         RuntimeNodeJS,
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 1,
			Compatible:    1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}
}

func TestValidateEligibilityReport_InvalidStatus(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          "invalid_status",
				StatusReasoning: "Some reasoning.",
				Runtime:         RuntimeNodeJS,
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 1,
			Compatible:    1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for invalid status, got nil")
	}
}

func TestValidateEligibilityReport_MissingStatusReasoning(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusCompatible,
				StatusReasoning: "", // Missing
				Runtime:         RuntimeNodeJS,
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 1,
			Compatible:    1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for missing status_reasoning, got nil")
	}
}

func TestValidateEligibilityReport_InvalidRuntime(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusCompatible,
				StatusReasoning: "Valid reasoning.",
				Runtime:         "java", // Invalid - should be "other"
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 1,
			Compatible:    1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for invalid runtime, got nil")
	}
}

func TestValidateEligibilityReport_AllValidRuntimes(t *testing.T) {
	runtimes := []Runtime{RuntimeNodeJS, RuntimePython, RuntimeOther}

	for _, runtime := range runtimes {
		report := &EligibilityReport{
			Services: map[string]ServiceEligibility{
				"./service": {
					Status:          StatusNotCompatible,
					StatusReasoning: "Testing runtime validation.",
					Runtime:         runtime,
				},
			},
			Summary: EligibilitySummary{
				TotalServices: 1,
				NotCompatible: 1,
			},
		}

		err := ValidateEligibilityReport(report)
		if err != nil {
			t.Errorf("expected runtime %s to be valid, got error: %v", runtime, err)
		}
	}
}

func TestValidateEligibilityReport_SupportedPackagesMissingReasoning(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusCompatible,
				StatusReasoning: "Valid reasoning.",
				Runtime:         RuntimeNodeJS,
				SupportedPackages: &PackageInfo{
					Packages:  []string{"pg@8.11.0"},
					Reasoning: "", // Missing
				},
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 1,
			Compatible:    1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for missing supported_packages.reasoning, got nil")
	}
}

func TestValidateEligibilityReport_UnsupportedPackagesMissingReasoning(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusPartiallyCompatible,
				StatusReasoning: "Valid reasoning.",
				Runtime:         RuntimeNodeJS,
				UnsupportedPackages: &PackageInfo{
					Packages:  []string{"mongodb@6.0.0"},
					Reasoning: "", // Missing
				},
			},
		},
		Summary: EligibilitySummary{
			TotalServices:       1,
			PartiallyCompatible: 1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for missing unsupported_packages.reasoning, got nil")
	}
}

func TestValidateEligibilityReport_UnknownPackagesMissingReasoning(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusCompatible,
				StatusReasoning: "Valid reasoning.",
				Runtime:         RuntimeNodeJS,
				UnknownPackages: &PackageInfo{
					Packages:  []string{"lodash@4.17.21"},
					Reasoning: "", // Missing
				},
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 1,
			Compatible:    1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for missing unknown_packages.reasoning, got nil")
	}
}

func TestValidateEligibilityReport_SummaryTotalMismatch(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusCompatible,
				StatusReasoning: "Valid reasoning.",
				Runtime:         RuntimeNodeJS,
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 5, // Wrong - should be 1
			Compatible:    1,
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for total_services mismatch, got nil")
	}
}

func TestValidateEligibilityReport_SummaryCompatibleMismatch(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusCompatible,
				StatusReasoning: "Valid reasoning.",
				Runtime:         RuntimeNodeJS,
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 1,
			Compatible:    0, // Wrong - should be 1
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for compatible count mismatch, got nil")
	}
}

func TestValidateEligibilityReport_SummaryPartiallyCompatibleMismatch(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusPartiallyCompatible,
				StatusReasoning: "Valid reasoning.",
				Runtime:         RuntimeNodeJS,
			},
		},
		Summary: EligibilitySummary{
			TotalServices:       1,
			PartiallyCompatible: 0, // Wrong - should be 1
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for partially_compatible count mismatch, got nil")
	}
}

func TestValidateEligibilityReport_SummaryNotCompatibleMismatch(t *testing.T) {
	report := &EligibilityReport{
		Services: map[string]ServiceEligibility{
			"./backend": {
				Status:          StatusNotCompatible,
				StatusReasoning: "Valid reasoning.",
				Runtime:         RuntimeOther,
			},
		},
		Summary: EligibilitySummary{
			TotalServices: 1,
			NotCompatible: 0, // Wrong - should be 1
		},
	}

	err := ValidateEligibilityReport(report)
	if err == nil {
		t.Error("expected error for not_compatible count mismatch, got nil")
	}
}

func TestParseEligibilityReport_ValidJSON(t *testing.T) {
	jsonStr := `{
		"services": {
			"./backend": {
				"status": "compatible",
				"status_reasoning": "All dependencies supported.",
				"runtime": "nodejs",
				"framework": "express"
			}
		},
		"summary": {
			"total_services": 1,
			"compatible": 1,
			"partially_compatible": 0,
			"not_compatible": 0
		}
	}`

	report, err := ParseEligibilityReport(jsonStr)
	if err != nil {
		t.Errorf("expected valid JSON to parse, got error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Services["./backend"].Status != StatusCompatible {
		t.Errorf("expected status compatible, got %s", report.Services["./backend"].Status)
	}
	if report.Services["./backend"].Runtime != RuntimeNodeJS {
		t.Errorf("expected runtime nodejs, got %s", report.Services["./backend"].Runtime)
	}
}

func TestParseEligibilityReport_InvalidJSON(t *testing.T) {
	jsonStr := `{invalid json}`

	_, err := ParseEligibilityReport(jsonStr)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestParseEligibilityReport_ValidJSONButInvalidReport(t *testing.T) {
	jsonStr := `{
		"services": {
			"./backend": {
				"status": "invalid_status",
				"status_reasoning": "Some reasoning.",
				"runtime": "nodejs"
			}
		},
		"summary": {
			"total_services": 1,
			"compatible": 1
		}
	}`

	_, err := ParseEligibilityReport(jsonStr)
	if err == nil {
		t.Error("expected validation error, got nil")
	}
}

func TestParseEligibilityReport_OtherRuntime(t *testing.T) {
	jsonStr := `{
		"services": {
			"./backend": {
				"status": "not_compatible",
				"status_reasoning": "Go is not supported.",
				"runtime": "other",
				"framework": "gin"
			}
		},
		"summary": {
			"total_services": 1,
			"compatible": 0,
			"partially_compatible": 0,
			"not_compatible": 1
		}
	}`

	report, err := ParseEligibilityReport(jsonStr)
	if err != nil {
		t.Errorf("expected valid JSON to parse, got error: %v", err)
	}
	if report.Services["./backend"].Runtime != RuntimeOther {
		t.Errorf("expected runtime other, got %s", report.Services["./backend"].Runtime)
	}
}
