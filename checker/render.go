package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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
	for _, finding := range result.Findings {
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
