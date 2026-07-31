package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestRunBuildsDeterministicReleaseManifest(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()

	for _, name := range []string{
		"golden-path_0.2.0_darwin_amd64.tar.gz",
		"golden-path_0.2.0_darwin_arm64.tar.gz",
		"golden-path_0.2.0_linux_amd64.tar.gz",
		"golden-path_0.2.0_linux_arm64.tar.gz",
	} {
		archive := []byte("fixture:" + name)
		if err := root.WriteFile(name, archive, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(archive)
		sbomName := strings.TrimSuffix(name, ".tar.gz") + ".cdx.json"
		sbom := fmt.Sprintf(
			`{"bomFormat":"CycloneDX","specVersion":"1.6","metadata":{"component":{"name":%q,"hashes":[{"alg":"SHA-256","content":"%x"}]}}}`,
			name,
			sum,
		)
		if err := root.WriteFile(sbomName, []byte(sbom), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(directory, "release-manifest.json")
	var stderr bytes.Buffer
	if err := run([]string{
		"--dist", directory,
		"--tag", "v0.2.0",
		"--source-commit", "0123456789abcdef0123456789abcdef01234567",
		"--output", output,
	}, &stderr); err != nil {
		t.Fatalf("run failed: %v; stderr=%s", err, stderr.String())
	}

	data, err := root.ReadFile("release-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var result manifest
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	repositoryRoot, err := os.OpenRoot(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := repositoryRoot.Close(); err != nil {
			t.Error(err)
		}
	}()
	releaseSchema, err := repositoryRoot.ReadFile("release/golden-path-release-manifest-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	validateDocument(t, releaseSchema, data)
	if result.ReleaseVersion != "0.2.0" || len(result.Assets) != 4 || len(result.RuntimeSelections) == 0 || result.Components.TemplateBundle.Version != "0.2.0" || result.Components.Automation.Version != "0.2.0" {
		t.Fatalf("unexpected manifest: %+v", result)
	}
	for index := 1; index < len(result.Assets); index++ {
		if result.Assets[index-1].Name >= result.Assets[index].Name {
			t.Fatal("release assets are not sorted")
		}
	}
	for _, item := range result.Assets {
		if len(item.SHA256) != 64 || item.Size == 0 || len(item.SBOM.SHA256) != 64 || item.SBOM.Size == 0 {
			t.Fatalf("asset identity is incomplete: %+v", item)
		}
	}
}

func TestCompatibilityManifestSatisfiesContract(t *testing.T) {
	root, err := os.OpenRoot(filepath.Join("..", "..", "compatibility"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	schema, err := root.ReadFile("golden-path-checker-compatibility-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	document, err := root.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	validateDocument(t, schema, document)
}

func TestRunRejectsTagVersionMismatch(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{
		"--tag", "v0.1.0",
		"--source-commit", strings.Repeat("a", 40),
		"--output", filepath.Join(t.TempDir(), "release-manifest.json"),
	}, &stderr)
	if err == nil {
		t.Fatal("run succeeded with a mismatched tag")
	}
}

func validateDocument(t *testing.T, schemaData, documentData []byte) {
	t.Helper()
	var schemaDocument any
	if err := json.Unmarshal(schemaData, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(documentData, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:test:schema", schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:test:schema")
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatal(err)
	}
}
