package generator

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowContractDocument struct {
	Jobs map[string]workflowContractJob `yaml:"jobs"`
}

type workflowContractJob struct {
	If             string                 `yaml:"if"`
	Uses           string                 `yaml:"uses"`
	TimeoutMinutes int                    `yaml:"timeout-minutes"`
	Steps          []workflowContractStep `yaml:"steps"`
}

type workflowContractStep struct {
	Name string `yaml:"name"`
	ID   string `yaml:"id"`
	If   string `yaml:"if"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
}

func TestReusableConformanceExecutesCandidateAndBoundsLegacyCost(t *testing.T) {
	reusable := readWorkflowContract(t, filepath.Join("..", ".github", "workflows", "golden-path-quality.yml"))
	job := reusable.Jobs["conformance"]
	if job.TimeoutMinutes != 10 {
		t.Fatalf("conformance timeout = %d, want measured initial bound 10", job.TimeoutMinutes)
	}
	upload := workflowContractStepNamed(t, job, "Upload complete conformance result")
	if upload.If != "${{ always() && steps.check.outputs.status != '' && steps.check.outputs.status != '0' }}" {
		t.Fatalf("artifact retention condition = %q, want non-passing evidence only", upload.If)
	}
	preserve := workflowContractStepNamed(t, job, "Preserve report-only result semantics")
	if preserve.If != "${{ always() }}" {
		t.Fatalf("missing checker status can bypass the final guard: %q", preserve.If)
	}

	ci := readWorkflowContract(t, filepath.Join("..", ".github", "workflows", "ci.yml"))
	if uses := ci.Jobs["reusable-conformance"].Uses; uses != "./.github/workflows/golden-path-quality.yml" {
		t.Fatalf("CI does not execute the exact reusable conformance workflow: %q", uses)
	}
	if _, exists := ci.Jobs["legacy-reusable-quality"]; exists {
		t.Fatal("CI still contains the duplicate legacy reusable-quality matrix")
	}
}

func TestReusableConformanceCallerContractScript(t *testing.T) {
	document := readWorkflowContract(t, filepath.Join("..", ".github", "workflows", "golden-path-quality.yml"))
	script := workflowContractStepNamed(t, document.Jobs["conformance"], "Validate immutable caller contract").Run
	sha := strings.Repeat("a", 40)
	workflowPath := "5010-dev/engineering-tooling/.github/workflows/golden-path-quality.yml@"

	for _, test := range []struct {
		name          string
		callerRepo    string
		callerSHA     string
		workflowRef   string
		wantCandidate string
		wantSuccess   bool
	}{
		{name: "external immutable consumer", callerRepo: "5010-dev/consumer", callerSHA: strings.Repeat("b", 40), workflowRef: workflowPath + sha, wantCandidate: "false", wantSuccess: true},
		{name: "source candidate", callerRepo: "5010-dev/engineering-tooling", callerSHA: sha, workflowRef: workflowPath + "refs/pull/1/merge", wantCandidate: "true", wantSuccess: true},
		{name: "external mutable ref", callerRepo: "5010-dev/consumer", callerSHA: strings.Repeat("b", 40), workflowRef: workflowPath + "refs/heads/main", wantSuccess: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "github-output")
			command := exec.Command("bash", "-c", script) // #nosec G204 -- repository-owned fixed workflow script under test.
			command.Env = append(os.Environ(),
				"EXPECTED_PROFILES=[\"go\"]",
				"CALLER_REPOSITORY="+test.callerRepo,
				"CALLER_SHA="+test.callerSHA,
				"WORKFLOW_REF="+test.workflowRef,
				"WORKFLOW_REPOSITORY=5010-dev/engineering-tooling",
				"WORKFLOW_SHA="+sha,
				"GITHUB_OUTPUT="+output,
			)
			err := command.Run()
			if test.wantSuccess && err != nil {
				t.Fatalf("caller contract failed: %v", err)
			}
			if !test.wantSuccess {
				if err == nil {
					t.Fatal("mutable external workflow ref was accepted")
				}
				return
			}
			data, readErr := os.ReadFile(output) // #nosec G304 -- test-owned output path.
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(data) != "candidate-mode="+test.wantCandidate+"\n" {
				t.Fatalf("caller mode output = %q", data)
			}
		})
	}
}

func TestReusableConformancePreservesExactExitSemantics(t *testing.T) {
	document := readWorkflowContract(t, filepath.Join("..", ".github", "workflows", "golden-path-quality.yml"))
	script := workflowContractStepNamed(t, document.Jobs["conformance"], "Preserve report-only result semantics").Run
	for _, test := range []struct {
		status   string
		wantExit int
	}{
		{status: "0", wantExit: 0},
		{status: "1", wantExit: 0},
		{status: "2", wantExit: 2},
		{status: "3", wantExit: 3},
		{status: "", wantExit: 3},
		{status: "invalid", wantExit: 3},
	} {
		command := exec.Command("bash", "-c", script) // #nosec G204 -- repository-owned fixed workflow script under test.
		command.Env = append(os.Environ(), "CHECK_STATUS="+test.status)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		err := command.Run()
		actual := 0
		if err != nil {
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				t.Fatalf("status %s returned non-exit error: %v", test.status, err)
			}
			actual = exitError.ExitCode()
		}
		if actual != test.wantExit {
			t.Fatalf("status %s maps to exit %d, want %d; stderr=%s", test.status, actual, test.wantExit, stderr.String())
		}
	}
}

func TestReusableConformanceCapturesCheckerStatus(t *testing.T) {
	document := readWorkflowContract(t, filepath.Join("..", ".github", "workflows", "golden-path-quality.yml"))
	script := workflowContractStepNamed(t, document.Jobs["conformance"], "Run structural conformance").Run
	for _, status := range []string{"0", "1", "2", "3"} {
		t.Run(status, func(t *testing.T) {
			directory := t.TempDir()
			binary := filepath.Join(directory, "golden-path")
			stub := "#!/bin/bash\nset -eu\nwhile test $# -gt 0; do\n  if test \"$1\" = --json-output; then shift; printf '{}\\n' >\"$1\"; fi\n  shift\ndone\nexit \"$FAKE_STATUS\"\n"
			if err := os.WriteFile(binary, []byte(stub), 0o700); err != nil { // #nosec G306 -- test-owned executable fixture.
				t.Fatal(err)
			}
			output := filepath.Join(directory, "github-output")
			command := exec.Command("bash", "-c", script) // #nosec G204 -- repository-owned fixed workflow script under test.
			command.Env = append(os.Environ(),
				"EXPECTED_PROFILES=[\"go\"]",
				"GOLDEN_PATH_BIN="+binary,
				"GOLDEN_PATH_EVALUATED_AT=2026-08-06T00:00:00Z",
				"RUNNER_TEMP="+directory,
				"GITHUB_STEP_SUMMARY="+filepath.Join(directory, "summary"),
				"GITHUB_OUTPUT="+output,
				"FAKE_STATUS="+status,
			)
			err := command.Run()
			if status == "0" && err != nil {
				t.Fatalf("successful checker script failed: %v", err)
			}
			if status != "0" {
				var exitError *exec.ExitError
				if !errors.As(err, &exitError) || exitError.ExitCode() != int(status[0]-'0') {
					t.Fatalf("checker status %s returned %v", status, err)
				}
			}
			data, readErr := os.ReadFile(output) // #nosec G304 -- test-owned output path.
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(data) != "status="+status+"\n" {
				t.Fatalf("captured checker status = %q", data)
			}
		})
	}
}

func readWorkflowContract(t *testing.T, name string) workflowContractDocument {
	t.Helper()
	data, err := os.ReadFile(name) // #nosec G304 -- fixed repository-owned workflow path supplied by tests.
	if err != nil {
		t.Fatal(err)
	}
	var document workflowContractDocument
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func workflowContractStepNamed(t *testing.T, job workflowContractJob, name string) workflowContractStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("workflow step %q is missing", name)
	return workflowContractStep{}
}
