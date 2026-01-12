package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EligibilityStatus represents the compatibility status of a service
type EligibilityStatus string

const (
	StatusCompatible          EligibilityStatus = "compatible"
	StatusPartiallyCompatible EligibilityStatus = "partially_compatible"
	StatusNotCompatible       EligibilityStatus = "not_compatible"
)

// Runtime represents a detected runtime environment
type Runtime string

const (
	RuntimeNodeJS Runtime = "nodejs"
	RuntimePython Runtime = "python"
	RuntimeOther  Runtime = "other"
)

// PackageInfo contains information about packages in a category
type PackageInfo struct {
	Packages  []string `json:"packages"`
	Reasoning string   `json:"reasoning"`
}

// ServiceEligibility contains eligibility information for a single service
type ServiceEligibility struct {
	Status              EligibilityStatus `json:"status"`
	StatusReasoning     string            `json:"status_reasoning"`
	Runtime             Runtime           `json:"runtime"`
	Framework           string            `json:"framework,omitempty"`
	SupportedPackages   *PackageInfo      `json:"supported_packages,omitempty"`
	UnsupportedPackages *PackageInfo      `json:"unsupported_packages,omitempty"`
	UnknownPackages     *PackageInfo      `json:"unknown_packages,omitempty"`
}

// EligibilitySummary contains aggregate statistics
type EligibilitySummary struct {
	TotalServices       int `json:"total_services"`
	Compatible          int `json:"compatible"`
	PartiallyCompatible int `json:"partially_compatible"`
	NotCompatible       int `json:"not_compatible"`
}

// EligibilityReport is the complete output structure
type EligibilityReport struct {
	Services map[string]ServiceEligibility `json:"services"`
	Summary  EligibilitySummary            `json:"summary"`
}

// ValidateEligibilityReport validates the structure and content of an eligibility report
// Returns an error if validation fails, nil if valid
func ValidateEligibilityReport(report *EligibilityReport) error {
	if report.Services == nil {
		return fmt.Errorf("services map is required")
	}

	// Validate each service
	compatibleCount := 0
	partialCount := 0
	notCompatibleCount := 0

	for path, service := range report.Services {
		if path == "" {
			return fmt.Errorf("service path cannot be empty")
		}

		// Validate status
		switch service.Status {
		case StatusCompatible:
			compatibleCount++
		case StatusPartiallyCompatible:
			partialCount++
		case StatusNotCompatible:
			notCompatibleCount++
		default:
			return fmt.Errorf("invalid status '%s' for service at path '%s'. Must be one of: compatible, partially_compatible, not_compatible", service.Status, path)
		}

		// Validate status reasoning is provided
		if service.StatusReasoning == "" {
			return fmt.Errorf("status_reasoning is required for service at path '%s'", path)
		}

		// Validate runtime
		switch service.Runtime {
		case RuntimeNodeJS, RuntimePython, RuntimeOther:
			// Valid
		default:
			return fmt.Errorf("invalid runtime '%s' for service at path '%s'. Must be one of: nodejs, python, other", service.Runtime, path)
		}

		// Validate package info if present
		if service.SupportedPackages != nil {
			if service.SupportedPackages.Reasoning == "" {
				return fmt.Errorf("supported_packages.reasoning is required for service at path '%s'", path)
			}
		}

		if service.UnsupportedPackages != nil {
			if service.UnsupportedPackages.Reasoning == "" {
				return fmt.Errorf("unsupported_packages.reasoning is required for service at path '%s'", path)
			}
		}

		if service.UnknownPackages != nil {
			if service.UnknownPackages.Reasoning == "" {
				return fmt.Errorf("unknown_packages.reasoning is required for service at path '%s'", path)
			}
		}
	}

	// Validate summary matches services
	if report.Summary.TotalServices != len(report.Services) {
		return fmt.Errorf("summary.total_services (%d) does not match actual service count (%d)", report.Summary.TotalServices, len(report.Services))
	}

	if report.Summary.Compatible != compatibleCount {
		return fmt.Errorf("summary.compatible (%d) does not match actual compatible count (%d)", report.Summary.Compatible, compatibleCount)
	}

	if report.Summary.PartiallyCompatible != partialCount {
		return fmt.Errorf("summary.partially_compatible (%d) does not match actual count (%d)", report.Summary.PartiallyCompatible, partialCount)
	}

	if report.Summary.NotCompatible != notCompatibleCount {
		return fmt.Errorf("summary.not_compatible (%d) does not match actual count (%d)", report.Summary.NotCompatible, notCompatibleCount)
	}

	return nil
}

// ParseEligibilityReport parses a JSON string into an EligibilityReport
// Returns an error if parsing or validation fails
func ParseEligibilityReport(jsonStr string) (*EligibilityReport, error) {
	var report EligibilityReport
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		return nil, fmt.Errorf("failed to parse eligibility report JSON: %w", err)
	}

	if err := ValidateEligibilityReport(&report); err != nil {
		return nil, err
	}

	return &report, nil
}

// WriteEligibilityReport writes the report to the .tusk directory
func WriteEligibilityReport(workDir string, report *EligibilityReport) error {
	tuskDir := filepath.Join(workDir, ".tusk")
	if err := os.MkdirAll(tuskDir, 0o750); err != nil {
		return fmt.Errorf("failed to create .tusk directory: %w", err)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal eligibility report: %w", err)
	}

	reportPath := filepath.Join(tuskDir, "eligibility-report.json")
	if err := os.WriteFile(reportPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write eligibility report: %w", err)
	}

	return nil
}
