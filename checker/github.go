package checker

import (
	"fmt"
	"sort"
	"strings"
)

const maxGitHubEvidenceFindings = 50

// RenderGitHubSummary renders bounded, human-readable conformance evidence for
// GitHub Actions step summaries.
func RenderGitHubSummary(result Result) []byte {
	var builder strings.Builder
	builder.WriteString("# Golden Path conformance\n\n")
	fmt.Fprintf(&builder, "- Standard: `%s`\n", markdownCell(result.StandardVersion))
	fmt.Fprintf(&builder, "- Checker: `%s`\n", markdownCell(result.CheckerVersion))
	fmt.Fprintf(&builder, "- Enforcement: `%s`\n", markdownCell(result.Enforcement))
	fmt.Fprintf(&builder, "- Evaluated at: `%s`\n", markdownCell(result.EvaluatedAt))
	fmt.Fprintf(&builder, "- Profiles: `%s`\n\n", markdownCell(strings.Join(result.Profiles, ", ")))
	builder.WriteString("| Pass | Fail | Warn | Skip | Waived | Error |\n")
	builder.WriteString("| ---: | ---: | ---: | ---: | ---: | ---: |\n")
	fmt.Fprintf(
		&builder,
		"| %d | %d | %d | %d | %d | %d |\n\n",
		result.Summary.Pass,
		result.Summary.Fail,
		result.Summary.Warn,
		result.Summary.Skip,
		result.Summary.Waived,
		result.Summary.Error,
	)
	skipped := skippedFindingBreakdown(result.Findings)
	fmt.Fprintf(
		&builder,
		"Skipped: %d total (%d not applicable, %d manual or hybrid, %d other).\n\n",
		result.Summary.Skip,
		skipped.NotApplicable,
		skipped.ManualOrHybrid,
		skipped.Other,
	)

	exceptions := make([]Finding, 0)
	for _, finding := range result.Findings {
		if finding.Status == "waived" {
			exceptions = append(exceptions, finding)
		}
	}
	if len(exceptions) > 0 {
		builder.WriteString("## Matched exceptions\n\n")
		builder.WriteString("| Rule | Exception | Expires |\n")
		builder.WriteString("| --- | --- | --- |\n")
		for _, finding := range exceptions {
			expiry, _ := finding.Extensions["exceptionExpiresAt"].(string)
			fmt.Fprintf(
				&builder,
				"| `%s` | `%s` | `%s` |\n",
				markdownCell(finding.RuleID),
				markdownCell(valueOrEmpty(finding.ExceptionID)),
				markdownCell(expiry),
			)
		}
		builder.WriteString("\n")
	}

	nonPassing := make([]Finding, 0)
	for _, finding := range result.Findings {
		if finding.Status != "pass" && finding.Status != "skip" {
			nonPassing = append(nonPassing, finding)
		}
	}
	if len(nonPassing) > 0 {
		builder.WriteString("## Findings requiring attention\n\n")
		builder.WriteString("| Status | Rule | Path | Message | Remediation |\n")
		builder.WriteString("| --- | --- | --- | --- | --- |\n")
		limit := min(len(nonPassing), maxGitHubEvidenceFindings)
		for _, finding := range nonPassing[:limit] {
			fmt.Fprintf(
				&builder,
				"| %s | `%s` | `%s` | %s | %s |\n",
				markdownCell(finding.Status),
				markdownCell(finding.RuleID),
				markdownCell(finding.Path),
				markdownCell(finding.Message),
				markdownCell(finding.Remediation),
			)
		}
		if len(nonPassing) > limit {
			fmt.Fprintf(&builder, "\n%d additional findings omitted from this bounded summary.\n", len(nonPassing)-limit)
		}
	}
	return []byte(builder.String())
}

// RenderGitHubAnnotations renders bounded GitHub workflow commands for
// actionable findings. JSON remains the complete machine-readable record.
func RenderGitHubAnnotations(result Result) []byte {
	findings := make([]Finding, 0)
	for _, finding := range result.Findings {
		switch finding.Status {
		case "error", "fail", "warn", "waived":
			findings = append(findings, finding)
		}
	}
	sort.SliceStable(findings, func(left, right int) bool {
		return findings[left].RuleID < findings[right].RuleID
	})
	if len(findings) > maxGitHubEvidenceFindings {
		findings = findings[:maxGitHubEvidenceFindings]
	}

	var builder strings.Builder
	for _, finding := range findings {
		level := "notice"
		switch finding.Status {
		case "error", "fail":
			level = "error"
		case "warn", "waived":
			level = "warning"
		}
		fmt.Fprintf(
			&builder,
			"::%s file=%s,title=%s::%s Remediation: %s\n",
			level,
			githubCommandProperty(finding.Path),
			githubCommandProperty(finding.RuleID+" "+finding.Status),
			githubCommandData(finding.Message),
			githubCommandData(finding.Remediation),
		)
	}
	return []byte(builder.String())
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func githubCommandData(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "\r", "%0D")
	value = strings.ReplaceAll(value, "\n", "%0A")
	return value
}

func githubCommandProperty(value string) string {
	value = githubCommandData(value)
	value = strings.ReplaceAll(value, ":", "%3A")
	value = strings.ReplaceAll(value, ",", "%2C")
	return value
}
