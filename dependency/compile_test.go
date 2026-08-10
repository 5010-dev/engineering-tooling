package dependency

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(name string) string { return filepath.Join("..", "testdata", "dependency", name) }

func TestSyntheticRepositoryShapesCompileDeterministically(t *testing.T) {
	tests := map[string]struct {
		classified int
		pending    int
		budget     int
	}{
		"polyglot-multi-unit":         {classified: 3, budget: 3},
		"package-workspace-publisher": {classified: 1, budget: 2},
		"single-service-oci":          {classified: 1, pending: 1, budget: 3},
	}
	for name, expected := range tests {
		t.Run(name, func(t *testing.T) {
			root := fixture(name)
			first := Evaluate(root)
			second := Evaluate(root)
			if first.ExitCode != 0 || !first.Complete {
				t.Fatalf("evaluation = exit %d complete %v findings=%+v", first.ExitCode, first.Complete, first.Findings)
			}
			firstJSON, _ := json.Marshal(first)
			secondJSON, _ := json.Marshal(second)
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatal("identical inputs produced different evaluation output")
			}
			classified, pending := 0, 0
			for _, root := range first.Roots {
				if root.Classification == "classified" {
					classified++
					if root.RoutinePRBudget != expected.budget {
						t.Errorf("root %s budget=%d want=%d", root.RootID, root.RoutinePRBudget, expected.budget)
					}
				} else {
					pending++
				}
			}
			if classified != expected.classified || pending != expected.pending {
				t.Fatalf("classified=%d pending=%d want=%d/%d", classified, pending, expected.classified, expected.pending)
			}
			candidate, _, err := Preview(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			candidateJSON, _ := json.Marshal(candidate)
			candidate2, _, err := Preview(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			candidateJSON2, _ := json.Marshal(candidate2)
			if !bytes.Equal(candidateJSON, candidateJSON2) {
				t.Fatal("preview is not deterministic")
			}
		})
	}
}

func TestRoutineBudgetBypassFailsWithoutBlockingSecurityRouter(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	path := filepath.Join(root, ".github", "dependabot.yml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("open-pull-requests-limit: 3"), []byte("open-pull-requests-limit: 8"), 1)
	// #nosec G703 -- path is inside a test-owned temporary fixture.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 1 || !hasRuleStatus(evaluation.Findings, "DT-DEP-007", "fail") {
		t.Fatalf("budget bypass = exit %d findings=%+v", evaluation.ExitCode, evaluation.Findings)
	}
	if _, exists := evaluation.RenderedFiles[".github/workflows/dependency-security-router.yml"]; !exists {
		t.Fatal("security router disappeared behind a routine budget violation")
	}
}

func TestMissingSecurityClosureReferenceFailsWithoutRemovingSecurityRouter(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	path := filepath.Join(root, ".github", "golden-path-dependency-policy.yaml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("      ciEvidence:\n        - {kind: github-actions-job, workflow: .github/workflows/security-remediation.yml, job: closure}\n"), nil, 1)
	// #nosec G703 -- path is inside the test-owned temporary fixture.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 1 || !evaluation.Complete || !hasRuleStatus(evaluation.Findings, "DT-DEP-012", "fail") {
		t.Fatalf("missing security closure reference = exit %d complete=%v findings=%+v", evaluation.ExitCode, evaluation.Complete, evaluation.Findings)
	}
	if _, exists := evaluation.RenderedFiles[".github/workflows/dependency-security-router.yml"]; !exists {
		t.Fatal("security router disappeared behind a security closure finding")
	}
}

func TestSecurityClosureReferenceIsNotEvaluatedWithoutDependencyAutomationCapability(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	metadataPath := filepath.Join(root, ".github", "golden-path.yaml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata = bytes.Replace(metadata, []byte(", dependency-automation"), nil, 1)
	// #nosec G703 -- path is inside the test-owned temporary fixture.
	if err := os.WriteFile(metadataPath, metadata, 0o600); err != nil {
		t.Fatal(err)
	}

	policyPath := filepath.Join(root, ".github", "golden-path-dependency-policy.yaml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	policy, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	policy = bytes.Replace(policy, []byte("      ciEvidence:\n        - {kind: github-actions-job, workflow: .github/workflows/security-remediation.yml, job: closure}\n"), nil, 1)
	// #nosec G703 -- path is inside the test-owned temporary fixture.
	if err := os.WriteFile(policyPath, policy, 0o600); err != nil {
		t.Fatal(err)
	}

	evaluation := Evaluate(root)
	if evaluation.ExitCode != 0 || !evaluation.Complete || !hasRuleStatus(evaluation.Findings, "DT-DEP-005", "skip") {
		t.Fatalf("capability-less evaluation = exit %d complete=%v findings=%+v", evaluation.ExitCode, evaluation.Complete, evaluation.Findings)
	}
	for _, finding := range evaluation.Findings {
		if finding.RuleID == "DT-DEP-012" {
			t.Fatalf("inapplicable security closure finding = %+v", evaluation.Findings)
		}
	}
}

func TestInvalidSecurityClosureReferenceIsConfigurationError(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	path := filepath.Join(root, ".github", "golden-path-dependency-policy.yaml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(".github/workflows/security-remediation.yml"), []byte(".github/workflows/missing-security-remediation.yml"), 1)
	// #nosec G703 -- path is inside the test-owned temporary fixture.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 2 || evaluation.Complete || !hasRuleStatus(evaluation.Findings, "DT-DEP-012", "error") {
		t.Fatalf("invalid security closure reference = exit %d complete=%v findings=%+v", evaluation.ExitCode, evaluation.Complete, evaluation.Findings)
	}
}

func TestOmittedDependabotLimitUsesNativeDefaultInsteadOfFreeze(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	path := filepath.Join(root, ".github", "dependabot.yml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("    open-pull-requests-limit: 0\n"), nil, 1)
	// #nosec G703 -- path is inside a test-owned temporary fixture.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 1 || !hasRuleStatus(evaluation.Findings, "DT-DEP-005", "fail") {
		t.Fatalf("omitted pending limit = exit %d findings=%+v", evaluation.ExitCode, evaluation.Findings)
	}
}

func TestRoutineTargetMustMatchCompiledIntegrationBranch(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	path := filepath.Join(root, ".github", "dependabot.yml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("target-branch: dev"), []byte("target-branch: main"), 1)
	// #nosec G703 -- path is inside a test-owned temporary fixture.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 1 || !hasRuleStatus(evaluation.Findings, "DT-DEP-005", "fail") {
		t.Fatalf("wrong routine target = exit %d findings=%+v", evaluation.ExitCode, evaluation.Findings)
	}
	if !bytes.Contains(evaluation.RenderedFiles[".github/dependabot.yml"], []byte("target-branch: dev")) {
		t.Fatal("candidate did not restore the compiled integration branch")
	}
}

func TestRenovateSelectionIsNotInterpretedAsDependabot(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	path := filepath.Join(root, ".github", "golden-path-dependency-policy.yaml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("adapter: dependabot"), []byte("adapter: renovate"), 1)
	// #nosec G703 -- path is inside a test-owned temporary fixture.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 2 || evaluation.Complete || !strings.Contains(evaluation.Findings[0].Message, "must not be interpreted as Dependabot") {
		t.Fatalf("renovate selection = exit %d complete=%v findings=%+v", evaluation.ExitCode, evaluation.Complete, evaluation.Findings)
	}
}

func TestOneNativeRootCannotMultiplyBudgetAcrossAdapterEcosystems(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	path := filepath.Join(root, ".github", "golden-path-native-roots.yaml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("profiles: [go]"), []byte("profiles: [go, rust]"), 1)
	// #nosec G703 -- path is inside a test-owned temporary fixture.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 2 || evaluation.Complete || len(evaluation.Findings) != 1 {
		t.Fatalf("multi-ecosystem root = exit %d complete=%v findings=%+v", evaluation.ExitCode, evaluation.Complete, evaluation.Findings)
	}
	finding := evaluation.Findings[0]
	if finding.Path != ".github/golden-path-native-roots.yaml" || !strings.Contains(finding.Message, "multiple adapter ecosystems (cargo, gomod)") || !strings.Contains(finding.Message, "separate existing native roots") {
		t.Fatalf("multi-ecosystem root finding = %+v", finding)
	}
}

func TestDuplicateRenovateManagerFailsConformance(t *testing.T) {
	root := copyFixture(t, "single-service-oci")
	// #nosec G703 -- path is inside a test-owned temporary fixture.
	if err := os.WriteFile(filepath.Join(root, "renovate.json"), []byte(`{"enabledManagers":["gomod"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 1 || !hasRuleStatus(evaluation.Findings, "DT-DEP-010", "fail") {
		t.Fatalf("duplicate manager = exit %d findings=%+v", evaluation.ExitCode, evaluation.Findings)
	}
}

func TestInvalidReleaseUnitReferenceIsConfigurationError(t *testing.T) {
	root := copyFixture(t, "package-workspace-publisher")
	path := filepath.Join(root, ".github", "golden-path-dependency-policy.yaml")
	// #nosec G304 -- path is inside a test-owned temporary fixture.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("design-package"), []byte("missing-package"), 1)
	// #nosec G703 -- path is inside a test-owned temporary fixture.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluate(root)
	if evaluation.ExitCode != 2 || evaluation.Complete {
		t.Fatalf("invalid reference = exit %d complete=%v", evaluation.ExitCode, evaluation.Complete)
	}
}

func TestPreviewWritesOnlyToSeparateEmptyStaging(t *testing.T) {
	root := fixture("single-service-oci")
	candidate, evaluation, err := Preview(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "candidate")
	if err := WriteStaging(root, output, candidate, evaluation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "dependency-candidate.json")); err != nil {
		t.Fatal(err)
	}
	if err := WriteStaging(root, output, candidate, evaluation); err == nil {
		t.Fatal("non-empty output was accepted")
	}
	for _, change := range candidate.Changes {
		if change.Ownership != "repository-owned" {
			t.Fatalf("compiler claimed repository path %s as %s", change.Path, change.Ownership)
		}
	}
}

func TestObservationIdentityAndReportAreSourceBound(t *testing.T) {
	observedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	preCandidate, _, err := Preview(fixture("single-service-oci"), nil)
	if err != nil {
		t.Fatal(err)
	}
	observation := Observation{SchemaVersion: ObservationSchema, ObservedAt: observedAt}
	observation.Scope.Organization = "5010-dev"
	observation.Scope.Query = "org:5010-dev is:pr is:open author:app/dependabot"
	observation.Source.Kind = "github-dependabot-api"
	observation.Source.APIVersion = "2022-11-28"
	observation.Repositories = []ObservedRepository{{
		Repository: "5010-dev/single-service-oci", RepositoryNodeID: "R_1", DefaultBranch: "main", DefaultBranchSHA: strings.Repeat("a", 40),
		DependencyPolicySHA256: strings.TrimPrefix(preCandidate.PolicySHA256, "sha256:"),
	}}
	observation.PullRequests = []ObservedPullRequest{{
		Repository: "5010-dev/single-service-oci", RepositoryNodeID: "R_1", Number: 7, NodeID: "PR_7", URL: "https://github.com/5010-dev/single-service-oci/pull/7",
		Base: RefIdentity{Ref: "dev", SHA: strings.Repeat("b", 40)}, Head: RefIdentity{Ref: "dependabot/go/example", SHA: strings.Repeat("c", 40)},
		Classification: "security-remediation", State: "open", CreatedAt: "2026-08-08T00:00:00Z", UpdatedAt: "2026-08-09T00:00:00Z",
		CheckRollup: CheckRollup{Status: "completed", Conclusion: "success"}, NativeRootRef: "go-service", OwnerRoute: "collector-operations",
		SecurityClosureEvidence: []SecurityClosureEvidence{{
			Kind: "github-actions-job-run", Workflow: ".github/workflows/security-remediation.yml", Job: "closure",
			RunID: 1234567890, RunAttempt: 1, RunURL: "https://github.com/5010-dev/single-service-oci/actions/runs/1234567890",
			HeadSHA: strings.Repeat("c", 40), Status: "completed", Conclusion: "success", ObservedAt: "2026-08-09T00:00:00Z",
		}},
	}}
	observation.Alerts = []ObservedAlert{{
		Repository: "5010-dev/single-service-oci", Number: 3, AdvisoryIdentity: "GHSA-test", State: "open", Severity: "high",
		Ecosystem: "gomod", Dependency: "example", Relationship: "unknown", SecurityUpdatePullRequest: "https://github.com/5010-dev/single-service-oci/pull/7",
	}}
	sealed, err := SealObservation(observation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeObservation(bytes.NewReader(sealed))
	if err != nil {
		t.Fatal(err)
	}
	candidate, _, err := Preview(fixture("single-service-oci"), &decoded)
	if err != nil {
		t.Fatal(err)
	}
	report, err := GenerateReport(decoded, map[string]Candidate{"single-service-oci": candidate}, nil, nil, observedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.ObservationIdentity != decoded.Source.Identity || len(report.Repositories) != 1 || report.Repositories[0].OpenAlerts != 1 || report.Repositories[0].OwnerRoutingCoverage != 1 {
		t.Fatalf("report lost source evidence: %+v", report)
	}
	if len(report.SecurityAdvisories) != 1 || report.SecurityAdvisories[0].Ecosystem != "gomod" || len(report.SecurityAdvisories[0].SecurityClosureEvidence) != 1 || report.SecurityAdvisories[0].SecurityClosureEvidence[0].HeadSHA != strings.Repeat("c", 40) {
		t.Fatalf("report lost exact-head security closure evidence: %+v", report.SecurityAdvisories)
	}
	tampered := bytes.Replace(sealed, []byte("PR_7"), []byte("PR_8"), 1)
	if _, err := DecodeObservation(bytes.NewReader(tampered)); err == nil {
		t.Fatal("tampered observation retained a valid identity")
	}
}

func TestSyntheticPartialSecurityRemediationReport(t *testing.T) {
	data, err := os.ReadFile(fixture("security-remediation-partial/observation.unsealed.json"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := DecodeUnsealedObservation(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealObservation(observation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeObservation(bytes.NewReader(sealed))
	if err != nil {
		t.Fatal(err)
	}
	report, err := GenerateReport(decoded, nil, nil, nil, decoded.ObservedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != ReportSchema || len(report.Repositories) != 1 || len(report.SecurityAdvisories) != 2 {
		t.Fatalf("partial remediation report shape = %+v", report)
	}
	row := report.Repositories[0]
	if row.OpenAlerts != 3 || row.OpenAdvisoryGroups != 2 || row.PartiallyLinkedAdvisoryGroups != 1 {
		t.Fatalf("partial remediation counts = %+v", row)
	}
	partial := report.SecurityAdvisories[0]
	if partial.AdvisoryIdentity != "GHSA-synthetic-same-advisory" || partial.Ecosystem != "npm" || partial.RemediationCoverage != "partial" || len(partial.OpenAlertInstances) != 2 {
		t.Fatalf("same-advisory coverage = %+v", partial)
	}
	if partial.OpenAlertInstances[0].Number != 6 || partial.OpenAlertInstances[0].Relationship != "direct" || partial.OpenAlertInstances[1].Number != 10 || partial.OpenAlertInstances[1].Relationship != "transitive" {
		t.Fatalf("same-advisory instances = %+v", partial.OpenAlertInstances)
	}
	unrelated := report.SecurityAdvisories[1]
	if unrelated.AdvisoryIdentity != "GHSA-synthetic-unrelated" || unrelated.Ecosystem != "npm" || unrelated.RemediationCoverage != "none" || unrelated.OpenAlertInstances[0].Relationship != "unknown" {
		t.Fatalf("unrelated advisory was coupled to the remediation: %+v", unrelated)
	}
}

func TestReportRejectsUnboundCandidate(t *testing.T) {
	observedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	candidate, _, err := Preview(fixture("single-service-oci"), nil)
	if err != nil {
		t.Fatal(err)
	}
	observation := Observation{SchemaVersion: ObservationSchema, ObservedAt: observedAt}
	observation.Scope.Organization = "5010-dev"
	observation.Scope.Query = "org:5010-dev is:pr is:open author:app/dependabot"
	observation.Source.Kind = "github-dependabot-api"
	observation.Source.APIVersion = "2022-11-28"
	observation.Repositories = []ObservedRepository{{Repository: "5010-dev/single-service-oci", RepositoryNodeID: "R_1", DefaultBranch: "main", DefaultBranchSHA: strings.Repeat("a", 40), DependencyPolicySHA256: strings.Repeat("b", 64)}}
	sealed, err := SealObservation(observation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeObservation(bytes.NewReader(sealed))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateReport(decoded, map[string]Candidate{"single-service-oci": candidate}, nil, nil, observedAt); err == nil {
		t.Fatal("report accepted a candidate from a different policy digest")
	}
}

func TestDefersDecodeAndReportBinding(t *testing.T) {
	data := []byte("schemaVersion: golden-path-dependency-defers/v1\ndefers:\n  - ecosystem: gomod\n    nativeRootRef: go-service\n    dependency: example.org/module\n    versionRange: v2.x\n    currentVersion: v1.0.0\n    availableVersion: v2.0.0\n    riskClass: major\n    reason: breaking API review\n    owner: collector-operations\n    observedAt: 2026-08-09T00:00:00Z\n    reviewedAt: 2026-08-09\n    reviewAfter: 2026-08-16\n")
	defers, err := DecodeDefers(bytes.NewReader(data))
	if err != nil || defers.SHA256 == "" {
		t.Fatalf("decode defers: %v %+v", err, defers)
	}
	observedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	observation := Observation{SchemaVersion: ObservationSchema, ObservedAt: observedAt}
	observation.Scope.Organization = "5010-dev"
	observation.Scope.Query = "org:5010-dev is:pr is:open author:app/dependabot"
	observation.Source.Kind = "github-dependabot-api"
	observation.Source.APIVersion = "2022-11-28"
	observation.Repositories = []ObservedRepository{{Repository: "5010-dev/single-service-oci", RepositoryNodeID: "R_1", DefaultBranch: "main", DefaultBranchSHA: strings.Repeat("a", 40), DefersSHA256: defers.SHA256}}
	sealed, err := SealObservation(observation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeObservation(bytes.NewReader(sealed))
	if err != nil {
		t.Fatal(err)
	}
	report, err := GenerateReport(decoded, nil, map[string]DefersFile{"single-service-oci": defers}, nil, observedAt)
	if err != nil || report.Repositories[0].Deferred != 1 {
		t.Fatalf("bound defers report: %v %+v", err, report)
	}
}

func hasRuleStatus(findings []Finding, ruleID, status string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Status == status {
			return true
		}
	}
	return false
}

func TestInternalEvaluationUsesExitThree(t *testing.T) {
	evaluation := internalEvaluation(Evaluation{}, "synthetic internal fault")
	if evaluation.ExitCode != 3 || evaluation.Complete || len(evaluation.Findings) != 1 || evaluation.Findings[0].Status != "error" {
		t.Fatalf("internal evaluation = %+v", evaluation)
	}
}

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	source := fixture(name)
	target := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o750)
		}
		// #nosec G304,G122 -- path is enumerated from a fixed test fixture copied into a new temporary tree.
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// #nosec G703 -- destination is inside the test-owned temporary copy root.
		return os.WriteFile(destination, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}
