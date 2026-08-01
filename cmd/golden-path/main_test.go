package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesJSONAndText(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "positive-go")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check",
		"--root", root,
		"--evaluated-at", "2026-07-31T00:00:00Z",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schemaVersion": "golden-path-checker-output/v1"`) {
		t.Fatal("stdout does not contain JSON output")
	}
	if !strings.Contains(stderr.String(), "Golden Path Conformance") {
		t.Fatal("stderr does not contain text output")
	}
}

func TestRunChecksThinCallerProfiles(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "positive-go")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check",
		"--root", root,
		"--evaluated-at", "2026-07-31T00:00:00Z",
		"--expected-profiles", `["python"]`,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want report-only finding exit 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "caller-profile-contract") {
		t.Fatal("result does not contain thin-caller profile mismatch evidence")
	}
}

func TestRunPreservesConfigurationExitWhenExpectedProfilesAreUnavailable(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "malformed")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check",
		"--root", root,
		"--evaluated-at", "2026-07-31T00:00:00Z",
		"--expected-profiles", `["go"]`,
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want configuration exit 2; stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "caller-profile-contract") {
		t.Fatal("incomplete metadata produced a caller-profile mismatch finding")
	}
}

func TestRunGeneratesOnlyIntoExplicitStaging(t *testing.T) {
	request := filepath.Join("..", "..", "testdata", "generator", "requests", "single-go.yaml")
	release := filepath.Join("..", "..", "testdata", "generator", "release-manifest.json")
	var preview, stderr bytes.Buffer
	code := run([]string{
		"generate", "--request", request, "--release-manifest", release,
	}, &preview, &stderr)
	if code != 0 {
		t.Fatalf("preview exit = %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(preview.String(), `"operation": "generate"`) {
		t.Fatal("preview does not contain a generation plan")
	}
	output := filepath.Join(t.TempDir(), "candidate")
	var materialized bytes.Buffer
	code = run([]string{
		"generate", "--request", request, "--release-manifest", release,
		"--write", "--output", output,
	}, &materialized, &stderr)
	if code != 0 {
		t.Fatalf("write exit = %d; stderr=%s", code, stderr.String())
	}
	for _, name := range []string{"justfile", "go.mod", "go.sum", "mise.lock", "golden-path-plan.json", ".github/golden-path-assets.json", ".github/golden-path-request.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing generated %s: %v", name, err)
		}
	}
	info, err := os.Stat(filepath.Join(output, "scripts", "golden-path"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("generated bootstrap mode = %o", info.Mode().Perm())
	}
}

func TestRunDoesNotWriteUpgradeCandidateWithConflicts(t *testing.T) {
	request := filepath.Join("..", "..", "testdata", "generator", "requests", "single-go.yaml")
	release := filepath.Join("..", "..", "testdata", "generator", "release-manifest.json")
	repository := filepath.Join(t.TempDir(), "repository")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"generate", "--request", request, "--release-manifest", release,
		"--write", "--output", repository,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("generate exit = %d; stderr=%s", code, stderr.String())
	}
	customized := []byte("# consumer-owned customization\n")
	// #nosec G306 -- the test intentionally models a conventional repository file.
	if err := os.WriteFile(filepath.Join(repository, "justfile"), customized, 0o644); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(t.TempDir(), "candidate")
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"upgrade", "--root", repository,
		"--request", request, "--release-manifest", release,
		"--write", "--output", candidate,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("conflicted upgrade exit = %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"conflictCount": 1`) {
		t.Fatalf("conflicted upgrade plan is missing: %s", stdout.String())
	}
	if _, err := os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatalf("conflicted upgrade materialized candidate: %v", err)
	}
	// #nosec G304 -- repository is a test-owned temporary directory.
	after, err := os.ReadFile(filepath.Join(repository, "justfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, customized) {
		t.Fatal("conflicted upgrade mutated the source repository")
	}
}

func TestRunRejectsRepositoryOutputPath(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "positive-go")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check",
		"--root", root,
		"--evaluated-at", "2026-07-31T00:00:00Z",
		"--json-output", filepath.Join(root, "result.json"),
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(root, "result.json")); !os.IsNotExist(err) {
		t.Fatal("checker wrote inside the repository")
	}
}

func TestRunWritesExplicitExternalOutput(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "positive-go")
	output := filepath.Join(t.TempDir(), "result.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check",
		"--root", root,
		"--evaluated-at", "2026-07-31T00:00:00+00:00",
		"--json-output", output,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	// #nosec G304 -- output is created inside the test-owned temporary directory.
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"evaluatedAt": "2026-07-31T00:00:00Z"`) {
		t.Fatal("JSON evaluatedAt is not canonical UTC Z")
	}
	if !strings.Contains(stdout.String(), "Golden Path Conformance") {
		t.Fatal("stdout does not contain text output")
	}
}

func TestRunRejectsSymlinkedParentResolvingIntoRepository(t *testing.T) {
	source := filepath.Join("..", "..", "testdata", "positive-go")
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	insideDirectory := filepath.Join(root, "evidence")
	if err := os.Mkdir(insideDirectory, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	alias := filepath.Join(outside, "alias")
	if err := os.Symlink(insideDirectory, alias); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(alias, "result.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check",
		"--root", root,
		"--evaluated-at", "2026-07-31T00:00:00Z",
		"--json-output", output,
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(insideDirectory, "result.json")); !os.IsNotExist(err) {
		t.Fatal("checker wrote through a symlinked parent into the repository")
	}
}

func TestRunWritesGitHubSummaryAndAnnotations(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "waived-go")
	outputDirectory := t.TempDir()
	jsonOutput := filepath.Join(outputDirectory, "result.json")
	summaryOutput := filepath.Join(outputDirectory, "summary.md")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"check",
		"--root", root,
		"--evaluated-at", "2026-07-31T00:00:00Z",
		"--json-output", jsonOutput,
		"--github-summary-output", summaryOutput,
		"--github-annotations",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	// #nosec G304 -- summaryOutput is inside a test-owned temporary directory.
	summary, err := os.ReadFile(summaryOutput)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"# Golden Path conformance", "Matched exceptions", "2026-08-31", "Remediation"} {
		if !strings.Contains(string(summary), expected) {
			t.Errorf("summary does not contain %q", expected)
		}
	}
	if !strings.Contains(stderr.String(), "::warning") {
		t.Fatal("waived finding did not produce a warning annotation")
	}
}
