package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/5010-dev/engineering-tooling/checker"
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
		"golden-path_1.2.3_darwin_amd64.tar.gz",
		"golden-path_1.2.3_darwin_arm64.tar.gz",
		"golden-path_1.2.3_linux_amd64.tar.gz",
		"golden-path_1.2.3_linux_arm64.tar.gz",
	} {
		archive := fixtureArchive(t, name)
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
	repository := filepath.Join("..", "..")
	for sourceName, distName := range map[string]string{
		"compatibility/manifest.json":                                       "compatibility-manifest.json",
		"compatibility/golden-path-checker-compatibility-v1.schema.json":    "golden-path-checker-compatibility-v1.schema.json",
		"standards/snapshots/2026.08/manifest.json":                         "standard-snapshot-manifest.json",
		"release/RELEASE_NOTES.md":                                          "RELEASE_NOTES.md",
		"release/golden-path-release-manifest-v1.schema.json":               "golden-path-release-manifest-v1.schema.json",
		"release/golden-path-release-manifest-v2.schema.json":               "golden-path-release-manifest-v2.schema.json",
		"release/golden-path-tooling-cutoff-v1.schema.json":                 "golden-path-tooling-cutoff-v1.schema.json",
		"release/tooling-cutoff-2026-08-04.json":                            "tooling-cutoff.json",
		"generator/schemas/golden-path-generated-assets-v1.schema.json":     "golden-path-generated-assets-v1.schema.json",
		"generator/schemas/golden-path-generator-request-v1.schema.json":    "golden-path-generator-request-v1.schema.json",
		"generator/schemas/golden-path-materialization-plan-v1.schema.json": "golden-path-materialization-plan-v1.schema.json",
	} {
		data, readErr := os.ReadFile(filepath.Join(repository, filepath.FromSlash(sourceName))) // #nosec G304 -- fixed test-owned source inventory.
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err := root.WriteFile(distName, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(directory, "release-manifest.json")
	var stderr bytes.Buffer
	if err := run([]string{
		"--source", repository,
		"--dist", directory,
		"--tag", "v1.2.3",
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
	releaseSchema, err := repositoryRoot.ReadFile("release/golden-path-release-manifest-v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	validateDocument(t, releaseSchema, data)
	if result.ReleaseVersion != "1.2.3" || result.Lifecycle != "stable" || len(result.Enforcement) != 1 || result.Enforcement[0] != "report-only" || len(result.Assets) != 4 || len(result.RuntimeSelections) == 0 || result.Components.AssetBundle.Version != "1.2.3" || result.Components.AssetBundle.Digest == "" || result.Components.Checker.Digest == "" || result.Components.Generator.Digest == "" || result.Components.Automation.Version != "1.2.3" || result.Components.Automation.Digest == "" {
		t.Fatalf("unexpected manifest: %+v", result)
	}
	if result.Snapshot.Source.Repository != "https://github.com/5010-dev/.github" || result.Snapshot.Source.Commit == "" || result.Snapshot.Source.GitTree != "ed2bf9ad2f5156c9195365274f0dded5a4b6f8c2" || result.Compatibility.File.SHA256 == "" || result.Snapshot.File.SHA256 == "" || result.ReleaseNotes.File.SHA256 == "" || len(result.Schemas) != 7 {
		t.Fatalf("release evidence is incomplete: %+v", result)
	}
	if !slices.Contains(result.SchemaVersions, "golden-path-native-roots/v1") {
		t.Fatalf("release manifest omits native-root schema compatibility: %+v", result.SchemaVersions)
	}
	if result.Build.CleanCheckout {
		t.Fatal("mismatched source commit was represented as a clean checkout")
	}
	for index := 1; index < len(result.Assets); index++ {
		if result.Assets[index-1].Name >= result.Assets[index].Name {
			t.Fatal("release assets are not sorted")
		}
	}
	for _, item := range result.Assets {
		if len(item.SHA256) != 64 || len(item.ExecutableSHA256) != 64 || item.Size == 0 || len(item.SBOM.SHA256) != 64 || item.SBOM.Size == 0 {
			t.Fatalf("asset identity is incomplete: %+v", item)
		}
	}
}

func TestSnapshotIdentityRejectsAWellFormedWrongSourceTree(t *testing.T) {
	standard := checker.CompatibleStandard{
		SourceCommit:            checker.SnapshotSourceCommit,
		SnapshotAggregateDigest: checker.SnapshotAggregateDigest,
	}
	snapshot := snapshotDocument{
		SchemaVersion:   "golden-path-standard-snapshot/v1",
		StandardVersion: checker.StandardVersion,
		ContractVersion: checker.ContractVersion,
		Source: snapshotSource{
			Repository: "https://github.com/5010-dev/.github",
			Commit:     checker.SnapshotSourceCommit,
			Path:       "docs/standards/developer-tooling",
			GitTree:    checker.SnapshotSourceTree,
		},
	}
	snapshot.Aggregate.Algorithm = "sha256"
	snapshot.Aggregate.Digest = strings.TrimPrefix(checker.SnapshotAggregateDigest, "sha256:")
	if !validSnapshotIdentity(snapshot, standard) {
		t.Fatal("exact snapshot identity was rejected")
	}
	snapshot.Source.GitTree = strings.Repeat("a", 40)
	if validSnapshotIdentity(snapshot, standard) {
		t.Fatal("well-formed but incorrect source tree was accepted")
	}
}

func fixtureArchive(t *testing.T, name string) []byte {
	t.Helper()
	var output bytes.Buffer
	compressed := gzip.NewWriter(&output)
	archive := tar.NewWriter(compressed)
	content := []byte("fixture executable:" + name)
	header := &tar.Header{Name: "golden-path/golden-path", Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := archive.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
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
		"--tag", "v0.2.0",
		"--source-commit", strings.Repeat("a", 40),
		"--output", filepath.Join(t.TempDir(), "release-manifest.json"),
	}, &stderr)
	if err == nil {
		t.Fatal("run succeeded with a mismatched tag")
	}
}

func TestRunRequiresExactCleanCheckoutWhenRequested(t *testing.T) {
	var stderr bytes.Buffer
	err := run([]string{
		"--source", filepath.Join("..", ".."),
		"--tag", "v1.2.3",
		"--source-commit", strings.Repeat("a", 40),
		"--output", filepath.Join(t.TempDir(), "release-manifest.json"),
		"--require-clean-checkout",
	}, &stderr)
	if err == nil || !strings.Contains(err.Error(), "release source must be a clean checkout") {
		t.Fatalf("required mismatched checkout error = %v", err)
	}
}

func TestMatchingEvidenceRejectsReleaseDrift(t *testing.T) {
	sourceDirectory := t.TempDir()
	distDirectory := t.TempDir()
	sourceRoot, err := os.OpenRoot(sourceDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sourceRoot.Close() }()
	distRoot, err := os.OpenRoot(distDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = distRoot.Close() }()
	if err := sourceRoot.WriteFile("evidence.json", []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := distRoot.WriteFile("evidence.json", []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := matchingEvidence(sourceRoot, distRoot, "evidence.json", "evidence.json"); err == nil {
		t.Fatal("accepted release evidence that did not match source")
	}
}

func TestDigestSourceTreeNormalizesCheckoutPermissions(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("plain.txt", []byte("plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("script.sh", []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	baseline, err := digestSourceTree(root, []string{"."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Chmod("plain.txt", 0o664); err != nil {
		t.Fatal(err)
	}
	if err := root.Chmod("script.sh", 0o775); err != nil {
		t.Fatal(err)
	}
	nonDefaultUmask, err := digestSourceTree(root, []string{"."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if baseline != nonDefaultUmask {
		t.Fatalf("digest changed with clone umask: %s != %s", baseline, nonDefaultUmask)
	}
	if err := root.Chmod("script.sh", 0o664); err != nil {
		t.Fatal(err)
	}
	nonExecutable, err := digestSourceTree(root, []string{"."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if baseline == nonExecutable {
		t.Fatal("digest ignored the Git executable bit")
	}
}

func TestCleanSourceCheckoutBindsHeadAndWorktree(t *testing.T) {
	repository := t.TempDir()
	for _, arguments := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "golden-path-test"},
		{"config", "user.email", "golden-path-test@users.noreply.github.com"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := gitOutput(repository, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(repository)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("evidence.txt", []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"add", "evidence.txt"}, {"commit", "-m", "test: add evidence"}} {
		if _, err := gitOutput(repository, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	head, err := gitOutput(repository, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(head))
	clean, err := cleanSourceCheckout(repository, commit)
	if err != nil || !clean {
		t.Fatalf("clean checkout = %v, err=%v", clean, err)
	}
	clean, err = cleanSourceCheckout(repository, strings.Repeat("a", 40))
	if err != nil || clean {
		t.Fatalf("mismatched commit checkout = %v, err=%v", clean, err)
	}
	if err := root.WriteFile("untracked.txt", []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clean, err = cleanSourceCheckout(repository, commit)
	if err != nil || clean {
		t.Fatalf("dirty checkout = %v, err=%v", clean, err)
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
