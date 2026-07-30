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
