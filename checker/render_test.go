package checker

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTextAndJSONDescribeSameFindingSet(t *testing.T) {
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
	for _, finding := range result.Findings {
		if !strings.Contains(text, finding.RuleID) || !strings.Contains(jsonText, `"`+finding.RuleID+`"`) {
			t.Errorf("finding %s is not represented in both outputs", finding.RuleID)
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
