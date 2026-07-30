package checker

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var fixtureTime = time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

func TestCheckFixtureContracts(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		wantExit   int
		wantStatus map[string]string
	}{
		{
			name:     "positive",
			fixture:  "positive-go",
			wantExit: 0,
			wantStatus: map[string]string{
				"DT-META-001": "pass",
				"DT-CMD-001":  "pass",
				"DT-GO-001":   "pass",
				"DT-GO-002":   "skip",
			},
		},
		{
			name:     "negative",
			fixture:  "negative-go",
			wantExit: 1,
			wantStatus: map[string]string{
				"DT-CMD-001":  "fail",
				"DT-TOOL-002": "fail",
				"DT-TOOL-003": "fail",
				"DT-GO-001":   "fail",
			},
		},
		{
			name:     "waived",
			fixture:  "waived-go",
			wantExit: 0,
			wantStatus: map[string]string{
				"DT-CMD-001": "waived",
				"DT-EXC-001": "pass",
				"DT-GO-001":  "pass",
			},
		},
		{
			name:     "node profile",
			fixture:  "positive-node",
			wantExit: 0,
			wantStatus: map[string]string{
				"DT-CMD-001":     "pass",
				"DT-DEP-001":     "pass",
				"DT-NODE-001":    "skip",
				"DT-RUNTIME-001": "pass",
			},
		},
		{
			name:     "python profile",
			fixture:  "positive-python",
			wantExit: 0,
			wantStatus: map[string]string{
				"DT-CMD-001":     "pass",
				"DT-DEP-001":     "pass",
				"DT-PY-001":      "skip",
				"DT-RUNTIME-001": "pass",
			},
		},
		{
			name:     "rust profile",
			fixture:  "positive-rust",
			wantExit: 0,
			wantStatus: map[string]string{
				"DT-CMD-001":     "pass",
				"DT-DEP-001":     "pass",
				"DT-RUST-001":    "pass",
				"DT-RUNTIME-001": "skip",
			},
		},
		{
			name:     "zig profile",
			fixture:  "positive-zig",
			wantExit: 0,
			wantStatus: map[string]string{
				"DT-CMD-001":     "pass",
				"DT-DEP-001":     "pass",
				"DT-ZIG-001":     "pass",
				"DT-RUNTIME-001": "pass",
			},
		},
		{
			name:     "infrastructure artifact",
			fixture:  "positive-infrastructure",
			wantExit: 0,
			wantStatus: map[string]string{
				"DT-META-001": "pass",
				"DT-CMD-001":  "pass",
				"DT-IAC-001":  "skip",
			},
		},
		{
			name:     "not applicable",
			fixture:  "not-applicable",
			wantExit: 0,
			wantStatus: map[string]string{
				"DT-META-001": "pass",
				"DT-CMD-001":  "skip",
			},
		},
		{
			name:     "malformed",
			fixture:  "malformed",
			wantExit: 2,
			wantStatus: map[string]string{
				"DT-META-001": "error",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join("..", "testdata", test.fixture)
			before := treeDigest(t, root)
			result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
			after := treeDigest(t, root)

			if result.ExitCode != test.wantExit {
				t.Fatalf("exit code = %d, want %d; findings=%+v", result.ExitCode, test.wantExit, result.Findings)
			}
			if before != after {
				t.Fatal("checker modified fixture files")
			}
			for ruleID, want := range test.wantStatus {
				if got := findingStatus(result, ruleID); got != want {
					t.Errorf("%s status = %q, want %q", ruleID, got, want)
				}
			}
			if _, err := RenderJSON(result); err != nil {
				t.Fatalf("result violates output schema: %v", err)
			}
		})
	}
}

func TestCheckIsDeterministic(t *testing.T) {
	options := Options{
		Root:        filepath.Join("..", "testdata", "positive-go"),
		EvaluatedAt: fixtureTime,
		Enforcement: "report-only",
	}
	first := Check(options)
	second := Check(options)
	if len(first.Findings) != 73 {
		t.Fatalf("finding count = %d, want one per catalog rule", len(first.Findings))
	}
	firstJSON, err := RenderJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := RenderJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("identical inputs produced different JSON")
	}
	if string(RenderText(first)) != string(RenderText(second)) {
		t.Fatal("identical inputs produced different text")
	}
	if !slices.IsSortedFunc(first.Findings, func(left, right Finding) int {
		leftKey := left.RuleID + "\x00" + left.Path + "\x00" + left.Secondary
		rightKey := right.RuleID + "\x00" + right.Path + "\x00" + right.Secondary
		return strings.Compare(leftKey, rightKey)
	}) {
		t.Fatal("findings are not in canonical order")
	}
}

func TestExpiredExceptionFails(t *testing.T) {
	result := Check(Options{
		Root:        filepath.Join("..", "testdata", "waived-go"),
		EvaluatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Enforcement: "report-only",
	})
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if got := findingStatus(result, "DT-CMD-001"); got != "fail" {
		t.Fatalf("DT-CMD-001 status = %q, want fail", got)
	}
}

func TestUnsupportedEnforcementIsConfigurationError(t *testing.T) {
	result := Check(Options{
		Root:        filepath.Join("..", "testdata", "positive-go"),
		EvaluatedAt: fixtureTime,
		Enforcement: "platform-enforced",
	})
	if result.ExitCode != 2 || result.Complete {
		t.Fatalf("got exit=%d complete=%t, want exit=2 complete=false", result.ExitCode, result.Complete)
	}
}

func TestUnsupportedStandardIsConfigurationError(t *testing.T) {
	root := copyFixture(t, "positive-go")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	data, err := rootFS.ReadFile(".github/golden-path.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"2026.07"`, `"2026.06"`, 1))
	if err := rootFS.WriteFile(".github/golden-path.yaml", data, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode != 2 || result.Complete {
		t.Fatalf("got exit=%d complete=%t, want exit=2 complete=false", result.ExitCode, result.Complete)
	}
}

func TestUnsafeInputIsIncompleteInternalEvaluation(t *testing.T) {
	root := copyFixture(t, "positive-go")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := rootFS.Remove("justfile"); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.Symlink("go.mod", "justfile"); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode != 3 || result.Complete {
		t.Fatalf("got exit=%d complete=%t, want exit=3 complete=false", result.ExitCode, result.Complete)
	}
	if got := findingStatus(result, "DT-CMD-001"); got != "error" {
		t.Fatalf("DT-CMD-001 status = %q, want error", got)
	}
}

func TestSnapshotCatalogIdentity(t *testing.T) {
	catalog, digest, err := loadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.StandardVersion != StandardVersion {
		t.Fatalf("standard = %q, want %q", catalog.StandardVersion, StandardVersion)
	}
	if len(catalog.Rules) != 73 {
		t.Fatalf("rule count = %d, want 73", len(catalog.Rules))
	}
	const want = "sha256:c2ec366495c5f2aa124a886152aadd1f4f0d1b7dcb34beb674c9dfa0db4b86ac"
	if digest != want {
		t.Fatalf("catalog digest = %q, want %q", digest, want)
	}
}

func TestEveryAutomatedRuleHasAnEvaluator(t *testing.T) {
	catalog, _, err := loadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	automated := 0
	for _, rule := range catalog.Rules {
		if rule.Assessment != "automated" {
			continue
		}
		automated++
		if rule.ID == "DT-RUNTIME-001" {
			continue
		}
		if _, exists := evaluators[rule.ID]; !exists {
			t.Errorf("automated rule %s has no evaluator", rule.ID)
		}
	}
	if automated != 19 {
		t.Fatalf("automated rule count = %d, want 19", automated)
	}
}

func TestRequirementLevelsMapToStableDeviationStatuses(t *testing.T) {
	tests := map[string]string{
		"MUST":       "fail",
		"MUST_NOT":   "fail",
		"SHOULD":     "warn",
		"SHOULD_NOT": "warn",
		"MAY":        "skip",
	}
	for level, expected := range tests {
		if got := deviationStatus(Rule{Level: level}); got != expected {
			t.Errorf("%s deviation = %q, want %q", level, got, expected)
		}
	}
}

func TestRuntimeSupportUsesExplicitEvaluationTime(t *testing.T) {
	rule := catalogRule(t, "DT-RUNTIME-001")
	root := t.TempDir()
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := rootFS.WriteFile(".python-version", []byte("3.10.19\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := Metadata{Profiles: []string{"python"}}

	supported := evaluateRuntimeDisposition(root, metadata, rule, time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC))
	if supported.Status != "pass" {
		t.Fatalf("deadline-day status = %q, want pass", supported.Status)
	}
	expired := evaluateRuntimeDisposition(root, metadata, rule, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if expired.Status != "fail" {
		t.Fatalf("post-deadline status = %q, want fail", expired.Status)
	}
}

func TestZigRuntimeRequiresApprovedExactBaseline(t *testing.T) {
	rule := catalogRule(t, "DT-RUNTIME-001")
	root := t.TempDir()
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := rootFS.WriteFile("mise.toml", []byte("[tools]\nzig = \"0.16.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := Metadata{Profiles: []string{"zig"}}
	if result := evaluateRuntimeDisposition(root, metadata, rule, fixtureTime); result.Status != "pass" {
		t.Fatalf("approved Zig status = %q, want pass", result.Status)
	}
	if err := rootFS.WriteFile("mise.toml", []byte("[tools]\nzig = \"0.15.2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := evaluateRuntimeDisposition(root, metadata, rule, fixtureTime); result.Status != "fail" {
		t.Fatalf("unapproved Zig status = %q, want fail", result.Status)
	}
}

func TestBundledContractExamplesValidate(t *testing.T) {
	tests := []struct {
		schema  string
		example string
	}{
		{
			schema:  "schemas/golden-path-metadata-v1.schema.json",
			example: "schemas/examples/golden-path-metadata-v1.valid.json",
		},
		{
			schema:  "schemas/golden-path-exceptions-v1.schema.json",
			example: "schemas/examples/golden-path-exceptions-v1.valid.json",
		},
		{
			schema:  "schemas/golden-path-checker-output-v1.schema.json",
			example: "schemas/examples/golden-path-checker-output-v1.valid.json",
		},
	}
	for _, test := range tests {
		data, err := readSnapshot(test.example)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateJSON(test.schema, data); err != nil {
			t.Errorf("%s does not validate: %v", test.example, err)
		}
	}
}

func catalogRule(t *testing.T, ruleID string) Rule {
	t.Helper()
	catalog, _, err := loadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range catalog.Rules {
		if rule.ID == ruleID {
			return rule
		}
	}
	t.Fatalf("rule %s not found", ruleID)
	return Rule{}
}

func findingStatus(result Result, ruleID string) string {
	for _, finding := range result.Findings {
		if finding.RuleID == ruleID {
			return finding.Status
		}
	}
	return ""
}

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	destination := t.TempDir()
	source := filepath.Join("..", "testdata", name)
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	return destination
}

func treeDigest(t *testing.T, root string) string {
	t.Helper()
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	hasher := sha256.New()
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		data, err := rootFS.ReadFile(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		_, _ = hasher.Write([]byte(filepath.ToSlash(relative)))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write(data)
		_, _ = hasher.Write([]byte{0})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
