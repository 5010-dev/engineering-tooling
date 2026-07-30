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
