package generator

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/5010-dev/engineering-tooling/checker"
	"github.com/pelletier/go-toml/v2"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func TestSyntheticRequestMatrixRendersDeterministically(t *testing.T) {
	t.Parallel()
	release := fixtureRelease(t)
	for _, name := range []string{"single-go.yaml", "monorepo.yaml", "documentation.yaml", "all-profiles.yaml", "zig-profiles.yaml"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := fixtureRequest(t, name)
			first, firstDigest, err := Render(request, release)
			if err != nil {
				t.Fatalf("render first candidate: %v", err)
			}
			second, secondDigest, err := Render(request, release)
			if err != nil {
				t.Fatalf("render second candidate: %v", err)
			}
			if firstDigest != secondDigest || len(first) != len(second) {
				t.Fatal("render is not deterministic")
			}
			for index := range first {
				if first[index].Path != second[index].Path || first[index].Mode != second[index].Mode || !bytes.Equal(first[index].Content, second[index].Content) {
					t.Fatalf("rendered file %d differs between runs", index)
				}
				for _, delimiter := range []string{"[[.", "[[if", "[[range"} {
					if bytes.Contains(first[index].Content, []byte(delimiter)) {
						t.Fatalf("rendered file %q retains template delimiter %q", first[index].Path, delimiter)
					}
				}
			}
			canonicalRequest, err := DecodeRequest(bytes.NewReader(fileContent(t, first, ".github/golden-path-request.json")))
			if err != nil {
				t.Fatalf("decode generated canonical request: %v", err)
			}
			if !reflect.DeepEqual(canonicalRequest, request) {
				t.Fatal("generated canonical request does not reproduce the normalized input")
			}
		})
	}
}

func TestDocumentationDoesNotExposeSuccessfulNoOpCommands(t *testing.T) {
	t.Parallel()
	files, _, err := Render(fixtureRequest(t, "documentation.yaml"), fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	justfile := string(fileContent(t, files, "justfile"))
	for _, command := range []string{"\nformat:\n", "\nlint:\n", "\ntypecheck:\n", "\nbuild:\n"} {
		if strings.Contains(justfile, command) {
			t.Fatalf("documentation template exposes inapplicable command %q", command)
		}
	}
	for _, command := range []string{"\nformat-check:", "\ntest:", "\ncheck:", "\nci:"} {
		if !strings.Contains(justfile, command) {
			t.Fatalf("documentation template omits applicable command %q", command)
		}
	}
}

func TestAllAcceptedProfilesCarryNativeAuthorities(t *testing.T) {
	t.Parallel()
	files, _, err := Render(fixtureRequest(t, "all-profiles.yaml"), fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		".github/golden-path-request.json",
		"components/cdk/cdk.json", "components/cdk/pnpm-lock.yaml",
		"components/go-service/go.mod", "components/go-service/go.sum", "components/go-service/cmd/go-service/main.go",
		"components/node-app/pnpm-lock.yaml", "components/opentofu/.terraform.lock.hcl",
		"components/pulumi/Pulumi.yaml", "components/python-service/uv.lock",
		"components/rust-cli/Cargo.lock", "components/rust-cli/rust-toolchain.toml",
		"components/terraform/.terraform.lock.hcl", "components/zig-cli/build.zig.zon",
		"components/zig-cli/zig-targets.json", "components/zig-cc/zig-toolchain.json",
		"mise.lock", ".github/workflows/developer-tooling.yml",
	} {
		_ = fileContent(t, files, path)
	}
	mise := string(fileContent(t, files, "mise.toml"))
	for _, pin := range []string{"github-cli = \"2.97.0\"", "go = \"1.26.5\"", "node = \"24.18.1\"", "uv = \"0.12.1\"", "zig = \"0.16.0\"", "terraform = \"1.15.8\"", "opentofu = \"1.12.5\"", "pulumi = \"3.255.0\""} {
		if !strings.Contains(mise, pin) {
			t.Errorf("mise.toml missing %s", pin)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", fileContent(t, files, "components/go-service/cmd/go-service/main.go"), parser.AllErrors); err != nil {
		t.Fatalf("generated Go source does not escape projectName: %v", err)
	}
	var pythonProject map[string]any
	if err := toml.Unmarshal(fileContent(t, files, "components/python-service/pyproject.toml"), &pythonProject); err != nil {
		t.Fatalf("generated Python TOML does not escape projectName: %v", err)
	}
	var pulumiProject map[string]any
	if err := yaml.Unmarshal(fileContent(t, files, "components/pulumi/Pulumi.yaml"), &pulumiProject); err != nil {
		t.Fatalf("generated Pulumi YAML does not escape projectName: %v", err)
	}
	for path, expected := range map[string]string{
		"components/node-app/src/index.ts":         "export function main(): void",
		"components/python-service/pyproject.toml": "[project.scripts]",
		"just/components/rust_cli.just":            "target/release/rust-cli",
	} {
		if !strings.Contains(string(fileContent(t, files, path)), expected) {
			t.Fatalf("generated artifact contract %q does not contain %q", path, expected)
		}
	}
	pnpmBootstrap := string(fileContent(t, files, "components/node-app/scripts/pnpm"))
	for _, expected := range []string{
		"archive_sha256='29c35ca8d2a287988fdee3e0f36e07d9b93783f567b579b7fd5b798a4563dd81'",
		"executable_sha256='d34a7b439643e7b8680a817387ec3692c7097ae7a85865c2c15ad6211143d506'",
		`sha256 "$binary"`,
		`sha256 "$candidate"`,
	} {
		if !strings.Contains(pnpmBootstrap, expected) {
			t.Fatalf("generated pnpm bootstrap does not enforce %q", expected)
		}
	}
	goldenPathBootstrap := string(fileContent(t, files, "scripts/golden-path"))
	for _, expected := range []string{
		"expected_executable='1111111111111111111111111111111111111111111111111111111111111111'",
		`sha256 "$binary"`,
		`sha256 "$candidate"`,
	} {
		if !strings.Contains(goldenPathBootstrap, expected) {
			t.Fatalf("generated Golden Path bootstrap does not enforce %q", expected)
		}
	}
}

func TestGeneratedPolyglotRepositoryPassesCheckerAndParsesJust(t *testing.T) {
	request := fixtureRequest(t, "all-profiles.yaml")
	release := fixtureRelease(t)
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	files, requestDigest, err := Render(request, release)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(t.TempDir(), "repository")
	if err := WriteStaging(repository, files, GeneratePlan(files, requestDigest, bundle, release)); err != nil {
		t.Fatal(err)
	}
	result := checker.Check(checker.Options{
		Root: repository, EvaluatedAt: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), Enforcement: "report-only",
	})
	if result.ExitCode != 0 || !result.Complete || result.Summary.Fail != 0 || result.Summary.Error != 0 {
		t.Fatalf("generated polyglot repository is not conformant: exit=%d complete=%t summary=%+v", result.ExitCode, result.Complete, result.Summary)
	}
	// #nosec G204 -- the command and all arguments are fixed except for the test-owned temporary path.
	command := exec.Command("just", "--justfile", filepath.Join(repository, "justfile"), "--list")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated Just graph does not parse: %v\n%s", err, output)
	}
	justfile := string(fileContent(t, files, "justfile"))
	for _, expected := range []string{"_go_service_check", "_python_service_check"} {
		if !strings.Contains(justfile, expected) {
			t.Fatalf("root check aggregation omits %q", expected)
		}
	}
	dependabot := string(fileContent(t, files, ".github/dependabot.yml"))
	if !strings.Contains(dependabot, "package-ecosystem: uv") || strings.Contains(dependabot, "package-ecosystem: pip") {
		t.Fatal("generated Dependabot configuration does not use the uv ecosystem")
	}
}

func TestFormatterPreferredQuotingAndLanguageSafeIdentifiers(t *testing.T) {
	for _, test := range []struct{ value, expected string }{
		{value: "Plain Project", expected: `"Plain Project"`},
		{value: "Acme's Project", expected: `"Acme's Project"`},
		{value: `Acme "Project"`, expected: `'Acme "Project"'`},
		{value: `Acme's "Project"`, expected: `'Acme\'s "Project"'`},
		{value: `Acme \\ Project`, expected: `"Acme \\\\ Project"`},
	} {
		if actual := formatterPreferredQuotedString(test.value); actual != test.expected {
			t.Errorf("formatterPreferredQuotedString(%q) = %q, want %q", test.value, actual, test.expected)
		}
	}
	if safeGoIdentifier("func") != "func_pkg" || safePythonIdentifier("class") != "class_pkg" || zigEnumLiteral("test") != `.@"test"` {
		t.Fatal("language-specific reserved identifiers are not rendered safely")
	}
}

func TestMixedLibraryAndExecutableArtifactsRenderBothSurfaces(t *testing.T) {
	request, err := DecodeRequest(strings.NewReader(`schemaVersion: golden-path-generator-request/v1
layout: polyglot
projectName: Mixed Artifacts
projectSlug: mixed-artifacts
components:
  - name: func
    path: components/go
    profiles: [go]
    artifactTypes: [cli, library]
  - name: match
    path: components/rust
    profiles: [rust]
    artifactTypes: [cli, library]
  - name: class
    path: components/python
    profiles: [python]
    artifactTypes: [library]
  - name: test
    path: components/zig
    profiles: [zig]
    artifactTypes: [cli]
`))
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := Render(request, fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"components/go/library.go", "components/go/cmd/func/main.go",
		"components/rust/src/lib.rs", "components/rust/src/main.rs",
		"components/python/src/class_pkg/__init__.py",
	} {
		_ = fileContent(t, files, name)
	}
	if !strings.Contains(string(fileContent(t, files, "components/go/library.go")), "package func_pkg") {
		t.Fatal("Go keyword component did not receive a safe package identifier")
	}
	if !strings.Contains(string(fileContent(t, files, "components/zig/build.zig.zon")), `.name = .@"test"`) {
		t.Fatal("Zig keyword component did not receive an escaped enum literal")
	}
}

func TestGoAndRustArtifactPowerSetRejectsOrRendersSource(t *testing.T) {
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	artifactCatalog := []string{
		"application", "service", "library", "cli", "package", "binary", "container", "tooling",
		"infrastructure", "documentation",
	}
	const sourceArtifactMask = 1<<8 - 1
	const serviceAndInfrastructureMask = 1<<1 | 1<<8
	for _, profile := range []string{"go", "rust"} {
		for mask := 1; mask < 1<<len(artifactCatalog); mask++ {
			artifacts := make([]string, 0, len(artifactCatalog))
			for index, artifact := range artifactCatalog {
				if mask&(1<<index) != 0 {
					artifacts = append(artifacts, artifact)
				}
			}
			request := Request{
				SchemaVersion: RequestSchema, Layout: "single", ProjectName: "Artifact Matrix", ProjectSlug: "artifact-matrix",
				Components: []Component{{Name: "sample", Path: ".", Profiles: []string{profile}, ArtifactTypes: artifacts}},
			}
			validationErr := validateRequest(&request)
			if mask&sourceArtifactMask == 0 {
				if validationErr == nil {
					t.Fatalf("%s accepted artifact set without a source surface: %v", profile, artifacts)
				}
				continue
			}
			if validationErr != nil {
				t.Fatalf("%s rejected source-bearing artifact set %v: %v", profile, artifacts, validationErr)
			}
			files, renderErr := renderComponent(request.Components[0], request, bundle)
			if renderErr != nil {
				t.Fatalf("render %s artifact set %v: %v", profile, artifacts, renderErr)
			}
			suffix := ".go"
			if profile == "rust" {
				suffix = ".rs"
			}
			hasSource := false
			for _, file := range files {
				if strings.HasSuffix(file.Path, suffix) {
					hasSource = true
					break
				}
			}
			if !hasSource {
				t.Fatalf("%s rendered no %s source for accepted artifact set %v", profile, suffix, artifacts)
			}
			if mask == serviceAndInfrastructureMask {
				executable := "cmd/sample/main.go"
				if profile == "rust" {
					executable = "src/main.rs"
				}
				_ = fileContent(t, files, executable)
			}
		}
	}
}

func TestUpgradeTreatsDeletedAndModeChangedGeneratedAssetsAsConflicts(t *testing.T) {
	t.Parallel()
	request := fixtureRequest(t, "single-go.yaml")
	release := fixtureRelease(t)
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	files, requestDigest, err := Render(request, release)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name   string
		path   string
		mutate func(string) error
	}{
		{name: "deleted", path: "justfile", mutate: os.Remove},
		{name: "mode-changed", path: "scripts/golden-path", mutate: func(name string) error {
			// #nosec G302 -- the fixture intentionally models executable-bit drift.
			return os.Chmod(name, 0o644)
		}},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			repository := filepath.Join(t.TempDir(), "repository")
			if err := WriteStaging(repository, files, GeneratePlan(files, requestDigest, bundle, release)); err != nil {
				t.Fatal(err)
			}
			if err := scenario.mutate(filepath.Join(repository, filepath.FromSlash(scenario.path))); err != nil {
				t.Fatal(err)
			}
			plan, err := UpgradePlan(repository, files, requestDigest, bundle, release)
			if err != nil {
				t.Fatal(err)
			}
			if plan.ConflictCount != 1 {
				t.Fatalf("conflict count = %d, want 1", plan.ConflictCount)
			}
			for _, change := range plan.Changes {
				if change.Path == scenario.path && change.Status != "conflict" {
					t.Fatalf("%s status = %q, want conflict", scenario.path, change.Status)
				}
			}
		})
	}
}

func TestGoModulePathsUseProjectBoundaryAndOfficialValidation(t *testing.T) {
	t.Parallel()
	valid := `schemaVersion: golden-path-generator-request/v1
layout: polyglot
projectName: Module Boundary
projectSlug: module-boundary
components:
  - name: api
    path: components/api
    profiles: [go]
    artifactTypes: [service]
  - name: docs
    path: components/docs
    profiles: [documentation]
    artifactTypes: [documentation]
`
	request, err := DecodeRequest(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	if request.Components[0].ModulePath != "github.com/5010-dev/module-boundary/components/api" {
		t.Fatalf("default modulePath = %q", request.Components[0].ModulePath)
	}
	invalid := strings.Replace(valid, "artifactTypes: [service]", "artifactTypes: [service]\n    modulePath: /bad", 1)
	if _, err := DecodeRequest(strings.NewReader(invalid)); err == nil {
		t.Fatal("accepted an invalid Go module path")
	}
}

func TestUpgradeClassifiesLocalCustomizationWithoutMutatingSource(t *testing.T) {
	t.Parallel()
	request := fixtureRequest(t, "single-go.yaml")
	release := fixtureRelease(t)
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	files, requestDigest, err := Render(request, release)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(t.TempDir(), "repository")
	if err := WriteStaging(repository, files, GeneratePlan(files, requestDigest, bundle, release)); err != nil {
		t.Fatal(err)
	}
	customized := []byte("# consumer-owned customization\n")
	// #nosec G306 -- the test intentionally models a repository-owned 0644 source file.
	if err := os.WriteFile(filepath.Join(repository, "justfile"), customized, 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := UpgradePlan(repository, files, requestDigest, bundle, release)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ConflictCount != 1 {
		t.Fatalf("conflict count = %d, want 1", plan.ConflictCount)
	}
	for _, change := range plan.Changes {
		if change.Path == "justfile" && change.Status != "conflict" {
			t.Fatalf("customized justfile status = %q", change.Status)
		}
	}
	// #nosec G304 -- repository is a test-owned temporary directory.
	after, err := os.ReadFile(filepath.Join(repository, "justfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, customized) {
		t.Fatal("upgrade planning mutated the source repository")
	}
	staging := filepath.Join(t.TempDir(), "candidate")
	if err := ValidateSeparateOutput(repository, staging); err != nil {
		t.Fatal(err)
	}
	if err := WriteStaging(staging, files, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(staging, "golden-path-plan.json")); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradePlansRetiredGeneratedAssetsWithoutMutatingSource(t *testing.T) {
	t.Parallel()
	request := fixtureRequest(t, "single-go.yaml")
	release := fixtureRelease(t)
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	files, requestDigest, err := Render(request, release)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(t.TempDir(), "repository")
	if err := WriteStaging(repository, files, GeneratePlan(files, requestDigest, bundle, release)); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repository, ".github", "golden-path-assets.json")
	manifestData, err := os.ReadFile(manifestPath) // #nosec G304 -- test-owned temporary repository.
	if err != nil {
		t.Fatal(err)
	}
	var manifest AssetManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	cleanContent := []byte("retired generated file\n")
	originalCustomizedContent := []byte("retired generated default\n")
	manifest.Files = append(manifest.Files,
		GeneratedAsset{Path: "retired-clean.txt", Mode: "0644", SHA256: digest(cleanContent), Source: "retired/template"},
		GeneratedAsset{Path: "retired-customized.txt", Mode: "0644", SHA256: digest(originalCustomizedContent), Source: "retired/template"},
	)
	updatedManifest, err := indentJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G306 -- test-owned repository files intentionally model conventional generated files.
	if err := os.WriteFile(manifestPath, updatedManifest, 0o644); err != nil {
		t.Fatal(err)
	}
	// #nosec G306 -- test-owned repository files intentionally model conventional generated files.
	if err := os.WriteFile(filepath.Join(repository, "retired-clean.txt"), cleanContent, 0o644); err != nil {
		t.Fatal(err)
	}
	customizedContent := []byte("consumer customization\n")
	// #nosec G306 -- test-owned repository files intentionally model conventional generated files.
	if err := os.WriteFile(filepath.Join(repository, "retired-customized.txt"), customizedContent, 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := UpgradePlan(repository, files, requestDigest, bundle, release)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ConflictCount != 1 {
		t.Fatalf("conflict count = %d, want 1", plan.ConflictCount)
	}
	statuses := map[string]string{}
	for _, change := range plan.Changes {
		statuses[change.Path] = change.Status
	}
	if statuses["retired-clean.txt"] != "remove" {
		t.Fatalf("clean retired file status = %q, want remove", statuses["retired-clean.txt"])
	}
	if statuses["retired-customized.txt"] != "conflict" {
		t.Fatalf("customized retired file status = %q, want conflict", statuses["retired-customized.txt"])
	}
	planData, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstSchema(t, "golden-path-materialization-plan-v1.schema.json", planData)
	for path, expected := range map[string][]byte{
		"retired-clean.txt":      cleanContent,
		"retired-customized.txt": customizedContent,
	} {
		actual, readErr := os.ReadFile(filepath.Join(repository, path)) // #nosec G304 -- test-owned temporary repository.
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(actual, expected) {
			t.Fatalf("upgrade planning mutated %s", path)
		}
	}
}

func TestGeneratedAutomationPinsReleaseIdentityAndVerifierPolicy(t *testing.T) {
	t.Parallel()
	files, _, err := Render(fixtureRequest(t, "single-go.yaml"), fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(fileContent(t, files, ".github/workflows/developer-tooling.yml"))
	commit := fixtureRelease(t).Source.Commit
	if !strings.Contains(workflow, "uses: 5010-dev/engineering-tooling/.github/workflows/golden-path-quality.yml@"+commit) {
		t.Fatal("generated caller does not pin the reusable workflow to the release source commit")
	}
	for _, expected := range []string{
		"source-commit: '" + commit + "'",
		"github-cli-version: '2.97.0'",
	} {
		if !strings.Contains(workflow, expected) {
			t.Fatalf("generated caller does not contain %q", expected)
		}
	}
	for _, forbidden := range []string{"pull_request_target:", "secrets:", "environment:", "contents: write"} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("generated GitHub Free baseline caller contains %q", forbidden)
		}
	}
	reusable, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "golden-path-quality.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reusable), "permissions:\n  contents: read") {
		t.Fatal("reusable workflow does not declare the read-only baseline permission")
	}
	for _, expected := range []string{
		"test \"$(gh version | awk 'NR == 1 { print $3 }')\" = \"$GITHUB_CLI_VERSION\"",
		"gh attestation verify",
		"--source-digest \"$SOURCE_COMMIT\"",
		"--signer-digest \"$SOURCE_COMMIT\"",
		"--source-ref \"refs/tags/v$CHECKER_VERSION\"",
		"--deny-self-hosted-runners",
		"check_help=\"$(\"$GOLDEN_PATH_BIN\" check --help 2>&1 || true)\"",
		"check_arguments+=(--expected-profiles \"$EXPECTED_PROFILES\")",
	} {
		if !strings.Contains(string(reusable), expected) {
			t.Fatalf("reusable workflow does not enforce %q", expected)
		}
	}
	for _, forbidden := range []string{"pull_request_target:", "secrets:", "environment:", "contents: write"} {
		if strings.Contains(string(reusable), forbidden) {
			t.Fatalf("reusable GitHub Free baseline workflow contains %q", forbidden)
		}
	}
	action, readErr := os.ReadFile(filepath.Join("..", "actions", "setup-golden-path", "action.yml"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, expected := range []string{"gh attestation verify", "--source-digest \"$SOURCE_COMMIT\"", "--deny-self-hosted-runners"} {
		if !strings.Contains(string(action), expected) {
			t.Fatalf("setup action does not enforce %q", expected)
		}
	}
	script := string(fileContent(t, files, "scripts/golden-path"))
	for _, expected := range []string{"github_cli_version='2.97.0'", "gh attestation verify", "--source-digest \"$source_commit\"", "--deny-self-hosted-runners"} {
		if !strings.Contains(script, expected) {
			t.Fatalf("generated bootstrap does not enforce %q", expected)
		}
	}
}

func TestRejectsCodeFirstInfrastructureWithoutHostProfile(t *testing.T) {
	t.Parallel()
	input := `schemaVersion: golden-path-generator-request/v1
layout: single
projectName: Invalid CDK Project
projectSlug: invalid-cdk-project
components:
  - name: infra
    path: .
    profiles: [infrastructure-aws-cdk]
    artifactTypes: [infrastructure]
`
	if _, err := DecodeRequest(strings.NewReader(input)); err == nil {
		t.Fatal("accepted CDK without node-typescript")
	}
}

func TestRequestDecodingRejectsTrailingDocumentsAndOverlappingComponents(t *testing.T) {
	t.Parallel()
	valid := `schemaVersion: golden-path-generator-request/v1
layout: single
projectName: Valid Project
projectSlug: valid-project
components:
  - name: app
    path: .
    profiles: [go]
    artifactTypes: [application]
`
	if _, err := DecodeRequest(strings.NewReader(valid + "---\n{}\n")); err == nil {
		t.Fatal("accepted a trailing YAML document")
	}
	overlapping := `schemaVersion: golden-path-generator-request/v1
layout: monorepo
projectName: Invalid Monorepo
projectSlug: invalid-monorepo
components:
  - name: parent
    path: components/parent
    profiles: [go]
    artifactTypes: [service]
  - name: child
    path: components/parent/child
    profiles: [python]
    artifactTypes: [service]
`
	if _, err := DecodeRequest(strings.NewReader(overlapping)); err == nil {
		t.Fatal("accepted overlapping component paths")
	}
}

func TestLibraryEntryPointsRejectUnsafeProgrammaticPaths(t *testing.T) {
	t.Parallel()
	request := fixtureRequest(t, "single-go.yaml")
	request.Components[0].Path = "../escape"
	if _, _, err := Render(request, fixtureRelease(t)); err == nil {
		t.Fatal("Render accepted an unsafe programmatic component path")
	}
	parent := t.TempDir()
	output := filepath.Join(parent, "candidate")
	unsafe := []File{{Path: "../escape", Mode: 0o644, Content: []byte("unsafe\n"), Source: "test"}}
	if err := WriteStaging(output, unsafe, Plan{}); err == nil {
		t.Fatal("WriteStaging accepted an unsafe programmatic file path")
	}
	if _, err := os.Stat(filepath.Join(parent, "escape")); !os.IsNotExist(err) {
		t.Fatal("unsafe staging path escaped the candidate directory")
	}
}

func TestPublishedGeneratorSchemasAcceptGeneratedContracts(t *testing.T) {
	t.Parallel()
	request := fixtureRequest(t, "single-go.yaml")
	release := fixtureRelease(t)
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	files, requestDigest, err := Render(request, release)
	if err != nil {
		t.Fatal(err)
	}
	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstSchema(t, "golden-path-generator-request-v1.schema.json", requestData)
	validateAgainstSchema(t, "golden-path-generated-assets-v1.schema.json", fileContent(t, files, ".github/golden-path-assets.json"))
	planData, err := json.Marshal(GeneratePlan(files, requestDigest, bundle, release))
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstSchema(t, "golden-path-materialization-plan-v1.schema.json", planData)
}

func TestPublishedRequestSchemaRejectsInputsRejectedByGeneratorSemantics(t *testing.T) {
	base := map[string]any{
		"schemaVersion": "golden-path-generator-request/v1",
		"layout":        "single",
		"projectName":   "Schema Boundary",
		"projectSlug":   "schema-boundary",
		"components": []any{map[string]any{
			"name": "app", "path": ".", "profiles": []any{"go"}, "artifactTypes": []any{"service"},
			"modulePath": "github.com/5010-dev/schema-boundary",
		}},
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "leading-project-whitespace", mutate: func(document map[string]any) { document["projectName"] = " Schema Boundary" }},
		{name: "overlong-component-name", mutate: func(document map[string]any) {
			document["components"].([]any)[0].(map[string]any)["name"] = strings.Repeat("a", 33)
		}},
		{name: "unclean-component-path", mutate: func(document map[string]any) {
			document["components"].([]any)[0].(map[string]any)["path"] = "components//app"
		}},
		{name: "non-domain-module-path", mutate: func(document map[string]any) {
			document["components"].([]any)[0].(map[string]any)["modulePath"] = "local/module"
		}},
		{name: "go-without-source-artifact", mutate: func(document map[string]any) {
			document["components"].([]any)[0].(map[string]any)["artifactTypes"] = []any{"infrastructure"}
		}},
		{name: "rust-without-source-artifact", mutate: func(document map[string]any) {
			component := document["components"].([]any)[0].(map[string]any)
			component["profiles"] = []any{"rust"}
			component["artifactTypes"] = []any{"documentation"}
			delete(component, "modulePath")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			test.mutate(document)
			candidate, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if schemaValidationError(t, "golden-path-generator-request-v1.schema.json", candidate) == nil {
				t.Fatal("published request schema accepted an input outside the generator boundary")
			}
		})
	}
	validateAgainstSchema(t, "golden-path-generator-request-v1.schema.json", []byte(`{
  "schemaVersion": "golden-path-generator-request/v1",
  "layout": "single",
  "projectName": "Mixed Artifact Service",
  "projectSlug": "mixed-artifact-service",
  "components": [{
    "name": "service",
    "path": ".",
    "profiles": ["rust"],
    "artifactTypes": ["service", "infrastructure"]
  }]
}`))
}

func fixtureRequest(t *testing.T, name string) Request {
	t.Helper()
	// #nosec G304 -- name is selected from the source-controlled test fixture matrix.
	file, err := os.Open(filepath.Join("..", "testdata", "generator", "requests", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	request, err := DecodeRequest(file)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func fixtureRelease(t *testing.T) ReleaseManifest {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "testdata", "generator", "release-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	release, err := DecodeReleaseManifest(file)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func fileContent(t *testing.T, files []File, path string) []byte {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			return file.Content
		}
	}
	t.Fatalf("rendered file %q not found", path)
	return nil
}

func validateAgainstSchema(t *testing.T, name string, documentData []byte) {
	t.Helper()
	if err := schemaValidationError(t, name, documentData); err != nil {
		t.Fatal(err)
	}
}

func schemaValidationError(t *testing.T, name string, documentData []byte) error {
	t.Helper()
	// #nosec G304 -- name is selected from source-controlled schema fixtures.
	schemaData, err := os.ReadFile(filepath.Join("schemas", name))
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument any
	if err := json.Unmarshal(schemaData, &schemaDocument); err != nil {
		return err
	}
	var document any
	if err := json.Unmarshal(documentData, &document); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:test:schema", schemaDocument); err != nil {
		return err
	}
	schema, err := compiler.Compile("urn:test:schema")
	if err != nil {
		return err
	}
	return schema.Validate(document)
}
