package checker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
				"DT-RUNTIME-001": "pass",
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

func TestRenewedExceptionTakesPrecedenceOverExpiredEntry(t *testing.T) {
	root := copyFixture(t, "waived-go")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	exceptions, err := rootFS.ReadFile(".github/golden-path-exceptions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	exceptions = append(exceptions, []byte(`  - id: GPE-2026-002
    rules:
      - DT-CMD-001
    scope:
      profiles:
        - go
      paths:
        - justfile
    reason: The command facade migration remains active under a renewed approval.
    owner: Platform Dev
    riskClass: standard
    approval:
      authorities:
        - role: Platform maintainer
          reference: ENG-182
          approvedAt: "2026-08-15"
    expiresAt: "2026-12-31"
    trackingIssue: ENG-182
    renewedFrom: GPE-2026-001
`)...)
	if err := rootFS.WriteFile(".github/golden-path-exceptions.yaml", exceptions, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{
		Root:        root,
		EvaluatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Enforcement: "report-only",
	})
	finding := findingByRule(result, "DT-CMD-001")
	if result.ExitCode != 0 || finding.Status != "waived" ||
		finding.ExceptionID == nil || *finding.ExceptionID != "GPE-2026-002" {
		t.Fatalf("renewed exception was not selected: exit=%d finding=%+v", result.ExitCode, finding)
	}
}

func TestApproachingExceptionExpiryWarns(t *testing.T) {
	result := Check(Options{
		Root:        filepath.Join("..", "testdata", "waived-go"),
		EvaluatedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Enforcement: "report-only",
	})
	if result.ExitCode != 0 || findingStatus(result, "DT-CMD-001") != "waived" {
		t.Fatalf("waiver changed exit semantics: exit=%d status=%q", result.ExitCode, findingStatus(result, "DT-CMD-001"))
	}
	finding := findingByRule(result, "DT-EXC-001")
	if finding.Status != "warn" {
		t.Fatalf("DT-EXC-001 status = %q, want warn", finding.Status)
	}
	if finding.Extensions["exceptionExpiryWarningDays"] != 30 {
		t.Fatalf("warning window extension = %#v, want 30", finding.Extensions["exceptionExpiryWarningDays"])
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

func TestMalformedNativeManifestIsConfigurationError(t *testing.T) {
	root := copyFixture(t, "positive-node")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := rootFS.WriteFile("package.json", []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode != 2 || result.Complete {
		t.Fatalf("got exit=%d complete=%t, want exit=2 complete=false", result.ExitCode, result.Complete)
	}
}

func TestMalformedMiseLockIsConfigurationError(t *testing.T) {
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
	if err := rootFS.WriteFile("mise.lock", []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode != 2 || result.Complete {
		t.Fatalf("got exit=%d complete=%t, want exit=2 complete=false", result.ExitCode, result.Complete)
	}
}

func TestRuntimeRequiresExactCompatibilityMapping(t *testing.T) {
	root := copyFixture(t, "positive-node")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	miseData, err := rootFS.ReadFile("mise.toml")
	if err != nil {
		t.Fatal(err)
	}
	miseData = []byte(strings.Replace(string(miseData), "24.18.1", "24.18.0", 1))
	if err := rootFS.WriteFile("mise.toml", miseData, 0o600); err != nil {
		t.Fatal(err)
	}
	lockData, err := rootFS.ReadFile("mise.lock")
	if err != nil {
		t.Fatal(err)
	}
	lockData = []byte(strings.Replace(string(lockData), "24.18.1", "24.18.0", 1))
	if err := rootFS.WriteFile("mise.lock", lockData, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode != 1 || findingStatus(result, "DT-RUNTIME-001") != "fail" {
		t.Fatalf("unexpected runtime mapping result: exit=%d finding=%q", result.ExitCode, findingStatus(result, "DT-RUNTIME-001"))
	}
}

func TestMiseMinorSelectorUsesSingleExactLockResolution(t *testing.T) {
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
	miseData, err := rootFS.ReadFile("mise.toml")
	if err != nil {
		t.Fatal(err)
	}
	miseData = []byte(strings.ReplaceAll(string(miseData), `min_version = "2026.7.17"`, `min_version = "2026.7"`))
	miseData = []byte(strings.ReplaceAll(string(miseData), `go = "1.26.5"`, `go = "1.26"`))
	if err := rootFS.WriteFile("mise.toml", miseData, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	for _, ruleID := range []string{"DT-TOOL-002", "DT-TOOL-003", "DT-GO-001", "DT-RUNTIME-001"} {
		if got := findingStatus(result, ruleID); got != "pass" {
			t.Errorf("%s status = %q, want pass", ruleID, got)
		}
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestUnpinnedWorkflowContainerFailsRemoteAssetRule(t *testing.T) {
	root := copyFixture(t, "positive-go")
	workflowDirectory := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDirectory, 0o750); err != nil {
		t.Fatal(err)
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	workflow := []byte(`name: test
on: push
jobs:
  test:
    runs-on: ubuntu-24.04
    container:
      image: node:latest
    steps:
      - uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09
`)
	if err := rootFS.WriteFile(".github/workflows/test.yml", workflow, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode != 1 || findingStatus(result, "DT-ASSET-002") != "fail" {
		t.Fatalf("unexpected remote asset result: exit=%d finding=%q", result.ExitCode, findingStatus(result, "DT-ASSET-002"))
	}
}

func TestUnpinnedWorkflowListItemActionFailsRemoteAssetRule(t *testing.T) {
	root := copyFixture(t, "positive-go")
	workflowDirectory := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDirectory, 0o750); err != nil {
		t.Fatal(err)
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	workflow := []byte(`name: test
on: push
jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v5
`)
	if err := rootFS.WriteFile(".github/workflows/test.yml", workflow, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	finding := findingByRule(result, "DT-ASSET-002")
	if result.ExitCode != 1 || finding.Status != "fail" || finding.Secondary != "actions/checkout@v5" {
		t.Fatalf("unexpected remote action result: exit=%d finding=%+v", result.ExitCode, finding)
	}
}

func TestJustImportTraversalHasAggregateFileBound(t *testing.T) {
	root := t.TempDir()
	importDirectory := filepath.Join(root, "just")
	if err := os.MkdirAll(importDirectory, 0o750); err != nil {
		t.Fatal(err)
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	var justfile strings.Builder
	justfile.WriteString("init:\n    true\ncheck:\n    true\nci:\n    true\n")
	for index := range maxJustImportFiles {
		name := fmt.Sprintf("just/%03d.just", index)
		fmt.Fprintf(&justfile, "import %q\n", name)
		if err := rootFS.WriteFile(name, []byte(fmt.Sprintf("recipe_%d:\n    true\n", index)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := collectJustRecipes(
		root,
		"justfile",
		[]byte(justfile.String()),
		&justTraversal{seen: map[string]bool{}},
		0,
	); err == nil {
		t.Fatal("aggregate Just import file bound was not enforced")
	}
}

func TestJustRecipeParserAcceptsDefaultParametersWithoutTreatingAssignmentsAsRecipes(t *testing.T) {
	recipes := parseJustRecipes(`evaluation := "2026-07-31T00:00:00Z"
init:
    true
check evaluated_at=evaluation:
    true
ci evaluated_at=evaluation: (check evaluated_at)
    true
`)
	for _, expected := range []string{"init", "check", "ci"} {
		if !recipes[expected] {
			t.Errorf("recipe %q was not detected", expected)
		}
	}
	if recipes["evaluation"] {
		t.Fatal("assignment was treated as a recipe")
	}
}

func TestJustImportAcceptsTrailingComment(t *testing.T) {
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
	if err := rootFS.WriteFile("justfile", []byte("import 'just/base.just' # shared recipes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if got := findingStatus(result, "DT-CMD-001"); got != "pass" {
		t.Fatalf("DT-CMD-001 status = %q, want pass", got)
	}
}

func TestNativeMarkersRequireMatchingDeclaredProfiles(t *testing.T) {
	tests := []struct {
		name    string
		marker  string
		profile string
	}{
		{name: "node", marker: "package.json", profile: "node-typescript"},
		{name: "python", marker: "pyproject.toml", profile: "python"},
		{name: "go", marker: "go.mod", profile: "go"},
		{name: "rust", marker: "Cargo.toml", profile: "rust"},
		{name: "zig", marker: "build.zig", profile: "zig"},
		{name: "zig toolchain", marker: "build.zig.zon", profile: "zig-toolchain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
			if err := rootFS.WriteFile(test.marker, []byte("marker\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if findingPath, message := validateProfileDeclarations(root, Metadata{Profiles: []string{"documentation"}}); findingPath != test.marker || message == "" {
				t.Fatalf("missing profile conflict = path %q message %q", findingPath, message)
			}
			if findingPath, message := validateProfileDeclarations(root, Metadata{Profiles: []string{test.profile}}); findingPath != "" || message != "" {
				t.Fatalf("matching profile conflict = path %q message %q", findingPath, message)
			}
		})
	}
}

func TestUnderDeclaredNativeProfileIsConfigurationError(t *testing.T) {
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
	metadata, err := rootFS.ReadFile(".github/golden-path.yaml")
	if err != nil {
		t.Fatal(err)
	}
	metadata = []byte(strings.Replace(string(metadata), "  - go\n", "  - documentation\n", 1))
	if err := rootFS.WriteFile(".github/golden-path.yaml", metadata, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	finding := findingByRule(result, "DT-META-001")
	if result.ExitCode != 2 || result.Complete || finding.Path != "go.mod" {
		t.Fatalf("profile drift did not fail closed: exit=%d complete=%t finding=%+v", result.ExitCode, result.Complete, finding)
	}
}

func TestNotApplicableRepositoryMayRetainNativeProfileMarkers(t *testing.T) {
	root := copyFixture(t, "not-applicable")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := rootFS.WriteFile("go.mod", []byte("module example.com/generated-mirror\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode != 0 || !result.Complete {
		t.Fatalf("not-applicable repository with native marker = exit %d complete %t, want exit 0 complete true", result.ExitCode, result.Complete)
	}
	for ruleID, want := range map[string]string{
		"DT-META-001": "pass",
		"DT-META-002": "skip",
		"DT-CMD-001":  "skip",
		"DT-GO-001":   "skip",
	} {
		if got := findingStatus(result, ruleID); got != want {
			t.Errorf("%s status = %q, want %q", ruleID, got, want)
		}
	}
}

func TestGoDirectiveMayBeLowerThanSelectedToolchain(t *testing.T) {
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
	goMod, err := rootFS.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	goMod = []byte(strings.Replace(string(goMod), "go 1.26", "go 1.25", 1))
	if err := rootFS.WriteFile("go.mod", goMod, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if got := findingStatus(result, "DT-GO-001"); got != "pass" {
		t.Fatalf("DT-GO-001 status = %q, want pass", got)
	}
}

func TestMultilineCargoDirectDependencyUsesStructuredIntegrity(t *testing.T) {
	root := copyFixture(t, "positive-rust")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rootFS.Close(); err != nil {
			t.Error(err)
		}
	}()
	cargo, err := rootFS.ReadFile("Cargo.toml")
	if err != nil {
		t.Fatal(err)
	}
	cargo = append(cargo, []byte(`
[dependencies.example]
git = "https://github.com/example/example"
rev = "0123456789abcdef0123456789abcdef01234567"
`)...)
	if err := rootFS.WriteFile("Cargo.toml", cargo, 0o600); err != nil {
		t.Fatal(err)
	}

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if got := findingStatus(result, "DT-DEP-004"); got != "skip" {
		t.Fatalf("DT-DEP-004 status = %q, want skip", got)
	}
}

func TestUnsafeInputIsConfigurationError(t *testing.T) {
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
	if result.ExitCode != 2 || result.Complete {
		t.Fatalf("got exit=%d complete=%t, want exit=2 complete=false", result.ExitCode, result.Complete)
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
		if rule.ID == "DT-RUNTIME-001" || rule.ID == "DT-ZIG-001" {
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
	if err := rootFS.WriteFile(".python-version", []byte("3.10.20\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := Metadata{Profiles: []string{"python"}}
	compatibility, err := loadCompatibility()
	if err != nil {
		t.Fatal(err)
	}

	supported := evaluateRuntimeDisposition(root, metadata, rule, time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC), compatibility)
	if supported.Status != "pass" {
		t.Fatalf("deadline-day status = %q, want pass", supported.Status)
	}
	expired := evaluateRuntimeDisposition(root, metadata, rule, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), compatibility)
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
	if err := rootFS.WriteFile("mise.lock", []byte("[[tools.zig]]\nversion = \"0.16.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := Metadata{Profiles: []string{"zig"}}
	compatibility, err := loadCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	if result := evaluateRuntimeDisposition(root, metadata, rule, fixtureTime, compatibility); result.Status != "pass" {
		t.Fatalf("approved Zig status = %q, want pass", result.Status)
	}
	if err := rootFS.WriteFile("mise.toml", []byte("[tools]\nzig = \"0.15.2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.WriteFile("mise.lock", []byte("[[tools.zig]]\nversion = \"0.15.2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := evaluateRuntimeDisposition(root, metadata, rule, fixtureTime, compatibility); result.Status != "fail" {
		t.Fatalf("unapproved Zig status = %q, want fail", result.Status)
	}
}

func TestZigProfileUsesCompatibilityManifestInsteadOfLiteralVersion(t *testing.T) {
	rule := catalogRule(t, "DT-ZIG-001")
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
	if err := rootFS.WriteFile("mise.toml", []byte("[tools]\nzig = \"0.17\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rootFS.WriteFile("mise.lock", []byte("[[tools.zig]]\nversion = \"0.17.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	compatibility, err := loadCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	for index := range compatibility.RuntimeSelections {
		if compatibility.RuntimeSelections[index].Profile == "zig" {
			compatibility.RuntimeSelections[index].Versions = []RuntimeSelectionVersion{{
				Version:     "0.17.0",
				Disposition: "preferred",
			}}
		}
	}
	finding := evaluateZigProfile(root, Metadata{Profiles: []string{"zig"}}, rule, compatibility)
	if finding.Status != "pass" {
		t.Fatalf("manifest-selected Zig status = %q, want pass", finding.Status)
	}
}

func TestCompatibilityManifestRetainsSupportDeadlines(t *testing.T) {
	compatibility, err := loadCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range compatibility.RuntimeSelections {
		if selection.Profile != "python" {
			continue
		}
		for _, version := range selection.Versions {
			if version.Version == "3.11.15" && version.SupportEndsAt == "2027-10-31" {
				return
			}
		}
	}
	t.Fatal("Python 3.11 support deadline is missing from compatibility manifest")
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

func findingByRule(result Result, ruleID string) Finding {
	for _, finding := range result.Findings {
		if finding.RuleID == ruleID {
			return finding
		}
	}
	return Finding{}
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
