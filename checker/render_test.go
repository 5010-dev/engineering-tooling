package checker

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultTextIsConciseAndJSONRemainsComplete(t *testing.T) {
	result := Check(Options{
		Root:        filepath.Join("..", "testdata", "positive-go"),
		EvaluatedAt: fixtureTime,
		Enforcement: "report-only",
	})
	text := string(RenderText(result))
	jsonData, err := RenderJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(jsonData)
	for _, expected := range []string{
		"summary pass=",
		"skipped=",
		"not-applicable=",
		"manual-or-hybrid=",
		"Actionable findings: none",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("default text does not contain %q", expected)
		}
	}
	for _, finding := range result.Findings {
		if !strings.Contains(jsonText, `"`+finding.RuleID+`"`) {
			t.Errorf("JSON omits finding %s", finding.RuleID)
		}
		if finding.Status == "pass" || finding.Status == "skip" {
			if strings.Contains(text, finding.RuleID) {
				t.Errorf("default text includes non-actionable finding %s", finding.RuleID)
			}
		}
	}
	breakdown := skippedFindingBreakdown(result.Findings)
	if breakdown != (skipBreakdown{NotApplicable: 34, ManualOrHybrid: 25, Other: 5}) {
		t.Fatalf("positive fixture skip categories changed: %+v", breakdown)
	}
	if breakdown.NotApplicable+breakdown.ManualOrHybrid+breakdown.Other != result.Summary.Skip {
		t.Fatalf("skip breakdown = %+v, want total %d", breakdown, result.Summary.Skip)
	}
	for index := range result.Findings {
		finding := &result.Findings[index]
		if finding.Status != "skip" {
			continue
		}
		category, _ := finding.Extensions[skipCategoryExtension].(string)
		if category != skipCategoryNotApplicable && category != skipCategoryManualOrHybrid && category != skipCategoryOther {
			t.Fatalf("skip finding %s has no explicit category: %#v", finding.RuleID, finding.Extensions)
		}
		finding.Message = "wording changed without changing semantics"
	}
	if after := skippedFindingBreakdown(result.Findings); after != breakdown {
		t.Fatalf("skip breakdown depends on human wording: before=%+v after=%+v", breakdown, after)
	}
}

func TestDefaultTextIncludesEveryActionableStatus(t *testing.T) {
	result := Result{
		StandardVersion: StandardVersion, ContractVersion: ContractVersion,
		CheckerVersion: Version, Enforcement: "report-only", Complete: true,
		Findings: []Finding{
			{RuleID: "DT-TEST-PASS", Status: "pass", Path: ".", Message: "pass"},
			{RuleID: "DT-TEST-SKIP", Status: "skip", Path: ".", Message: "skip", Extensions: map[string]any{skipCategoryExtension: skipCategoryOther}},
			{RuleID: "DT-TEST-ERROR", Status: "error", Path: "error", Message: "configuration error", Remediation: "fix configuration"},
			{RuleID: "DT-TEST-FAIL", Status: "fail", Path: "fail", Message: "contract failed", Remediation: "fix contract"},
			{RuleID: "DT-TEST-EXPIRED", Status: "fail", Path: "expired", Message: "Matching exception expired before evaluation.", Remediation: "remove or renew exception"},
			{RuleID: "DT-TEST-WARN", Status: "warn", Path: "warn", Message: "warning", Remediation: "review warning"},
			{RuleID: "DT-TEST-WAIVED", Status: "waived", Path: "waived", Message: "waived", Remediation: "remove waiver"},
		},
	}
	result.Summary = summarize(result.Findings)
	text := string(RenderText(result))
	for _, finding := range result.Findings {
		contained := strings.Contains(text, finding.RuleID)
		if finding.Status == "pass" || finding.Status == "skip" {
			if contained {
				t.Errorf("default text includes %s finding %s", finding.Status, finding.RuleID)
			}
			continue
		}
		if !contained || !strings.Contains(text, finding.Remediation) {
			t.Errorf("default text hides actionable finding %s or its remediation", finding.RuleID)
		}
	}
}

func TestExhaustiveTextDescribesCompleteFindingSet(t *testing.T) {
	result := Check(Options{
		Root:        filepath.Join("..", "testdata", "positive-go"),
		EvaluatedAt: fixtureTime,
		Enforcement: "report-only",
	})
	text := string(RenderTextAll(result))
	for _, finding := range result.Findings {
		if !strings.Contains(text, finding.RuleID) {
			t.Errorf("exhaustive text omits finding %s", finding.RuleID)
		}
	}
}

func TestGitHubAnnotationsEscapePropertiesAndMessagesSeparately(t *testing.T) {
	result := Result{Findings: []Finding{{
		RuleID:      "DT-TEST-001",
		Status:      "fail",
		Path:        "path:with,delimiters",
		Message:     "message: keep, punctuation",
		Remediation: "retry: once, then stop",
	}}}
	annotation := string(RenderGitHubAnnotations(result))
	for _, expected := range []string{
		"file=path%3Awith%2Cdelimiters",
		"message: keep, punctuation",
		"Remediation: retry: once, then stop",
	} {
		if !strings.Contains(annotation, expected) {
			t.Errorf("annotation %q does not contain %q", annotation, expected)
		}
	}
	for _, forbidden := range []string{"message%3A", "keep%2C", "retry%3A", "once%2C"} {
		if strings.Contains(annotation, forbidden) {
			t.Errorf("annotation %q contains over-escaped message fragment %q", annotation, forbidden)
		}
	}
}
