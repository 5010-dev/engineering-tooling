package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	skipCategoryExtension      = "skipCategory"
	skipCategoryNotApplicable  = "not-applicable"
	skipCategoryManualOrHybrid = "manual-or-hybrid"
	skipCategoryOther          = "other"
)

func RenderJSON(result Result) ([]byte, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode checker result: %w", err)
	}
	data = append(data, '\n')
	if err := validateJSON("schemas/golden-path-checker-output-v1.schema.json", data); err != nil {
		return nil, fmt.Errorf("validate checker result: %w", err)
	}
	return data, nil
}

func RenderText(result Result) []byte {
	return renderText(result, false)
}

// RenderTextAll renders the complete finding set for explicit diagnostics.
// RenderJSON remains the canonical complete machine output.
func RenderTextAll(result Result) []byte {
	return renderText(result, true)
}

func renderText(result Result, showAll bool) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "Golden Path Conformance")
	fmt.Fprintf(&output, "standard=%s contract=%s checker=%s enforcement=%s\n",
		result.StandardVersion, result.ContractVersion, result.CheckerVersion, result.Enforcement)
	fmt.Fprintf(&output, "evaluatedAt=%s catalog=%s\n", result.EvaluatedAt, result.CatalogDigest)
	fmt.Fprintf(&output, "profiles=%s complete=%t exitCode=%d\n",
		strings.Join(result.Profiles, ","), result.Complete, result.ExitCode)
	fmt.Fprintf(&output, "summary pass=%d fail=%d warn=%d skip=%d waived=%d error=%d\n",
		result.Summary.Pass, result.Summary.Fail, result.Summary.Warn,
		result.Summary.Skip, result.Summary.Waived, result.Summary.Error)
	skipped := skippedFindingBreakdown(result.Findings)
	fmt.Fprintf(&output, "skipped=%d (not-applicable=%d manual-or-hybrid=%d other=%d)\n",
		result.Summary.Skip, skipped.NotApplicable, skipped.ManualOrHybrid, skipped.Other)

	findings := textFindings(result.Findings, showAll)
	if len(findings) == 0 {
		fmt.Fprintln(&output, "Actionable findings: none")
		return output.Bytes()
	}
	if showAll {
		fmt.Fprintln(&output, "Findings:")
	} else {
		fmt.Fprintln(&output, "Actionable findings:")
	}
	for _, finding := range findings {
		fmt.Fprintf(&output, "[%s] %s %s", strings.ToUpper(finding.Status), finding.RuleID, finding.Path)
		if finding.Secondary != "" {
			fmt.Fprintf(&output, " (%s)", finding.Secondary)
		}
		fmt.Fprintf(&output, " - %s\n", finding.Message)
		if finding.Status != "pass" && finding.Status != "skip" {
			fmt.Fprintf(&output, "  remediation: %s\n", finding.Remediation)
		}
	}
	return output.Bytes()
}

type skipBreakdown struct {
	NotApplicable  int
	ManualOrHybrid int
	Other          int
}

func skippedFindingBreakdown(findings []Finding) skipBreakdown {
	var result skipBreakdown
	for _, finding := range findings {
		if finding.Status != "skip" {
			continue
		}
		category, _ := finding.Extensions[skipCategoryExtension].(string)
		switch category {
		case skipCategoryNotApplicable:
			result.NotApplicable++
		case skipCategoryManualOrHybrid:
			result.ManualOrHybrid++
		default:
			result.Other++
		}
	}
	return result
}

func textFindings(findings []Finding, showAll bool) []Finding {
	if showAll {
		return findings
	}
	result := make([]Finding, 0)
	for _, finding := range findings {
		switch finding.Status {
		case "error", "fail", "warn", "waived":
			result = append(result, finding)
		}
	}
	return result
}
