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
	"slices"
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
	for _, name := range []string{"single-go.yaml", "monorepo.yaml", "documentation.yaml", "all-profiles.yaml", "zig-profiles.yaml", "node-publisher.yaml", "adoption-go-service.yaml"} {
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

func TestExplicitCapabilitiesArePreservedAndActivateConditionalRules(t *testing.T) {
	t.Parallel()
	request := fixtureRequest(t, "node-publisher.yaml")
	files, requestDigest, err := Render(request, fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}

	var metadata struct {
		Capabilities []string `json:"capabilities"`
		Extensions   map[string]struct {
			Components []struct {
				Capabilities []string `json:"capabilities"`
			} `json:"components"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(fileContent(t, files, ".github/golden-path.yaml"), &metadata); err != nil {
		t.Fatal(err)
	}
	wantCapabilities := []string{
		"build", "cache", "dependency-automation", "format", "lint", "package", "publish",
		"released-artifact", "test", "typecheck",
	}
	if !reflect.DeepEqual(metadata.Capabilities, wantCapabilities) {
		t.Fatalf("aggregate capabilities = %v, want %v", metadata.Capabilities, wantCapabilities)
	}
	generated := metadata.Extensions["dev.fiftyten.generator"]
	if len(generated.Components) != 1 || !reflect.DeepEqual(generated.Components[0].Capabilities, wantCapabilities) {
		t.Fatalf("component capabilities = %+v, want %v", generated.Components, wantCapabilities)
	}

	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(t.TempDir(), "repository")
	if err := WriteStaging(repository, files, GeneratePlan(files, requestDigest, bundle, fixtureRelease(t))); err != nil {
		t.Fatal(err)
	}
	result := checker.Check(checker.Options{
		Root: repository, EvaluatedAt: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), Enforcement: "report-only",
	})
	for _, ruleID := range []string{
		"DT-NODE-004", "DT-NODE-005", "DT-SUPPLY-001", "DT-SUPPLY-002", "DT-SUPPLY-003", "DT-PLATFORM-001",
	} {
		finding := findingByRuleID(t, result, ruleID)
		if strings.Contains(finding.Message, "does not apply to the declared") {
			t.Fatalf("%s remained inapplicable after explicit capability generation: %s", ruleID, finding.Message)
		}
	}
}

func TestLegacyRequestDigestAndCanonicalShapeRemainStable(t *testing.T) {
	t.Parallel()
	files, requestDigest, err := Render(fixtureRequest(t, "single-go.yaml"), fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	if requestDigest != "0864564b0506ae9b1400c31eb7c0a946b61d379efcb43e9e654c6cb8701844e2" {
		t.Fatalf("legacy request digest = %q", requestDigest)
	}
	canonicalRequest := fileContent(t, files, ".github/golden-path-request.json")
	for _, field := range []string{`"capabilities"`, `"materializationMode"`, `"targets"`} {
		if bytes.Contains(canonicalRequest, []byte(field)) {
			t.Fatalf("legacy canonical request gained absent optional field %s", field)
		}
	}
}

func TestAdoptionMaterializesOnlyFixedControlPlaneAssets(t *testing.T) {
	t.Parallel()
	request := fixtureRequest(t, "adoption-go-service.yaml")
	if request.Components[0].ModulePath != "" {
		t.Fatalf("adoption request invented Go modulePath %q", request.Components[0].ModulePath)
	}
	files, requestDigest, err := Render(request, fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{
		".github/golden-path-assets.json",
		".github/golden-path-request.json",
		".github/golden-path.yaml",
		".github/workflows/developer-tooling.yml",
		"scripts/golden-path",
	}
	actualPaths := make([]string, 0, len(files))
	for _, file := range files {
		actualPaths = append(actualPaths, file.Path)
	}
	if !reflect.DeepEqual(actualPaths, wantPaths) {
		t.Fatalf("adoption paths = %v, want fixed control-plane set %v", actualPaths, wantPaths)
	}

	var metadata struct {
		Capabilities []string `json:"capabilities"`
		Targets      []Target `json:"targets"`
		Extensions   map[string]struct {
			MaterializationMode string `json:"materializationMode"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(fileContent(t, files, ".github/golden-path.yaml"), &metadata); err != nil {
		t.Fatal(err)
	}
	wantCapabilities := []string{"build", "format", "lint", "test"}
	if !reflect.DeepEqual(metadata.Capabilities, wantCapabilities) {
		t.Fatalf("adoption capabilities = %v, want only explicit declarations %v", metadata.Capabilities, wantCapabilities)
	}
	if len(metadata.Targets) != 1 || metadata.Targets[0].OS != "linux" || metadata.Targets[0].Architecture != "amd64" || metadata.Targets[0].Execution == nil || *metadata.Targets[0].Execution {
		t.Fatalf("adoption metadata targets = %+v", metadata.Targets)
	}
	if metadata.Extensions["dev.fiftyten.generator"].MaterializationMode != "adoption" {
		t.Fatalf("adoption metadata omits materialization mode: %+v", metadata.Extensions)
	}

	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(t.TempDir(), "repository")
	if err := WriteStaging(repository, files, GeneratePlan(files, requestDigest, bundle, fixtureRelease(t))); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string][]byte{
		"cmd/server/main.go": []byte("package main\n"),
		"go.mod":             []byte("module github.com/5010-dev/existing-go-service\n"),
		"justfile":           []byte("ci:\n    go test ./...\n"),
	} {
		absolute := filepath.Join(repository, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
			t.Fatal(err)
		}
		// #nosec G306 -- test-owned repository files use conventional source modes.
		if err := os.WriteFile(absolute, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result := checker.Check(checker.Options{
		Root: repository, EvaluatedAt: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), Enforcement: "report-only",
	})
	if !result.Complete || result.Summary.Error != 0 {
		t.Fatalf("adoption output produced incomplete checker evaluation: complete=%t summary=%+v", result.Complete, result.Summary)
	}
	if finding := findingByRuleID(t, result, "DT-META-001"); finding.Status != "pass" {
		t.Fatalf("adoption metadata did not pass schema validation: %+v", finding)
	}
	plan, err := UpgradePlan(repository, files, requestDigest, bundle, fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	if plan.ConflictCount != 0 {
		t.Fatalf("adoption baseline upgrade has %d conflicts", plan.ConflictCount)
	}
	for _, change := range plan.Changes {
		if change.Status != "unchanged" {
			t.Fatalf("adoption baseline path %s status = %s, want unchanged", change.Path, change.Status)
		}
	}
	for _, path := range []string{"cmd/server/main.go", "go.mod", "justfile"} {
		if _, err := os.Stat(filepath.Join(repository, filepath.FromSlash(path))); err != nil {
			t.Fatalf("repository-owned path %s was not preserved: %v", path, err)
		}
	}
}

func TestExplicitTargetsAreNormalizedAndPreserved(t *testing.T) {
	t.Parallel()
	request, err := DecodeRequest(strings.NewReader(`schemaVersion: golden-path-generator-request/v1
materializationMode: bootstrap
layout: single
projectName: Target Matrix
projectSlug: target-matrix
targets:
  - os: linux
    architecture: arm64
    runtime: container
    tier: secondary
  - os: linux
    architecture: amd64
    runtime: container
    tier: primary
    execution: false
components:
  - name: service
    path: .
    profiles: [go]
    artifactTypes: [service, container]
`))
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := Render(request, fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Targets) != 2 || request.Targets[0].Architecture != "amd64" || request.Targets[1].Architecture != "arm64" {
		t.Fatalf("normalized targets = %+v", request.Targets)
	}
	var metadata struct {
		Targets []Target `json:"targets"`
	}
	if err := json.Unmarshal(fileContent(t, files, ".github/golden-path.yaml"), &metadata); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metadata.Targets, request.Targets) {
		t.Fatalf("metadata targets = %+v, want request targets %+v", metadata.Targets, request.Targets)
	}
	var canonical Request
	if err := json.Unmarshal(fileContent(t, files, ".github/golden-path-request.json"), &canonical); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonical.Targets, request.Targets) {
		t.Fatalf("canonical request targets = %+v, want %+v", canonical.Targets, request.Targets)
	}
}

func TestRenderRejectsExplicitEmptyCapabilities(t *testing.T) {
	t.Parallel()
	request := fixtureRequest(t, "single-go.yaml")
	request.Components[0].Capabilities = []string{}
	if _, _, err := Render(request, fixtureRelease(t)); err == nil {
		t.Fatal("Render accepted explicitly empty capabilities")
	}
}

func TestAdoptionPreservesExplicitEmptyCapabilities(t *testing.T) {
	t.Parallel()
	request, err := DecodeRequest(strings.NewReader(`schemaVersion: golden-path-generator-request/v1
materializationMode: adoption
layout: single
projectName: Existing Documentation
projectSlug: existing-documentation
targets:
  - os: linux
    architecture: amd64
    tier: primary
components:
  - name: documentation
    path: .
    profiles: [documentation]
    artifactTypes: [documentation]
    capabilities: []
`))
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := Render(request, fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	canonicalRequest := fileContent(t, files, ".github/golden-path-request.json")
	if !bytes.Contains(canonicalRequest, []byte(`"capabilities": []`)) {
		t.Fatalf("adoption canonical request did not preserve explicit empty capabilities:\n%s", canonicalRequest)
	}
	validateAgainstSchema(t, "golden-path-generator-request-v1.schema.json", canonicalRequest)

	var metadata struct {
		Capabilities []string `json:"capabilities"`
		Extensions   map[string]struct {
			Components []struct {
				Capabilities []string `json:"capabilities"`
			} `json:"components"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(fileContent(t, files, ".github/golden-path.yaml"), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Capabilities == nil || len(metadata.Capabilities) != 0 {
		t.Fatalf("aggregate adoption capabilities = %#v, want explicit empty array", metadata.Capabilities)
	}
	components := metadata.Extensions["dev.fiftyten.generator"].Components
	if len(components) != 1 || components[0].Capabilities == nil || len(components[0].Capabilities) != 0 {
		t.Fatalf("component adoption capabilities = %#v, want explicit empty array", components)
	}
}

func TestGeneratorRejectsRepeatedTargetIdentityWithDifferentClaims(t *testing.T) {
	t.Parallel()
	_, err := DecodeRequest(strings.NewReader(`schemaVersion: golden-path-generator-request/v1
materializationMode: adoption
layout: single
projectName: Conflicting Target
projectSlug: conflicting-target
targets:
  - os: linux
    architecture: amd64
    runtime: container
    tier: primary
  - os: linux
    architecture: amd64
    runtime: container
    tier: secondary
    execution: false
components:
  - name: service
    path: .
    profiles: [go]
    artifactTypes: [service]
    capabilities: [build]
`))
	if err == nil || !strings.Contains(err.Error(), "same platform identity") {
		t.Fatalf("repeated target identity error = %v", err)
	}
}

func TestAdoptionAggregatesExactCapabilitiesAcrossComponents(t *testing.T) {
	t.Parallel()
	request, err := DecodeRequest(strings.NewReader(`schemaVersion: golden-path-generator-request/v1
materializationMode: adoption
layout: polyglot
projectName: Existing Polyglot Repository
projectSlug: existing-polyglot-repository
targets:
  - os: linux
    architecture: amd64
    tier: primary
components:
  - name: api
    path: services/api
    profiles: [go]
    artifactTypes: [service]
    capabilities: [test, build]
  - name: web
    path: apps/web
    profiles: [node-typescript]
    artifactTypes: [application]
    capabilities: [format, lint, test]
`))
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := Render(request, fixtureRelease(t))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Capabilities []string `json:"capabilities"`
		Extensions   map[string]struct {
			Components []struct {
				Path         string   `json:"path"`
				Capabilities []string `json:"capabilities"`
			} `json:"components"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(fileContent(t, files, ".github/golden-path.yaml"), &metadata); err != nil {
		t.Fatal(err)
	}
	if want := []string{"build", "format", "lint", "test"}; !reflect.DeepEqual(metadata.Capabilities, want) {
		t.Fatalf("aggregate adoption capabilities = %v, want %v", metadata.Capabilities, want)
	}
	components := metadata.Extensions["dev.fiftyten.generator"].Components
	if len(components) != 2 || components[0].Path != "apps/web" || !reflect.DeepEqual(components[0].Capabilities, []string{"format", "lint", "test"}) ||
		components[1].Path != "services/api" || !reflect.DeepEqual(components[1].Capabilities, []string{"build", "test"}) {
		t.Fatalf("component adoption capabilities = %+v", components)
	}
}

func TestRequestCapabilitiesMatchNormativeMetadataCatalog(t *testing.T) {
	t.Parallel()
	requestCapabilities := schemaStringEnum(t, filepath.Join("schemas", "golden-path-generator-request-v1.schema.json"),
		"properties", "components", "items", "properties", "capabilities", "items", "enum")
	metadataCapabilities := schemaStringEnum(t,
		filepath.Join("..", "standards", "snapshots", "2026.08", "schemas", "golden-path-metadata-v1.schema.json"),
		"$defs", "capability", "enum")
	want := append([]string(nil), supportedCapabilities...)
	slices.Sort(requestCapabilities)
	slices.Sort(metadataCapabilities)
	slices.Sort(want)
	if !reflect.DeepEqual(requestCapabilities, want) {
		t.Fatalf("request capability enum = %v, want generator catalog %v", requestCapabilities, want)
	}
	if !reflect.DeepEqual(metadataCapabilities, want) {
		t.Fatalf("metadata capability enum = %v, want generator catalog %v", metadataCapabilities, want)
	}
}

func TestRequestTargetsMatchNormativeMetadataSchema(t *testing.T) {
	t.Parallel()
	requestTarget := schemaValue(t, filepath.Join("schemas", "golden-path-generator-request-v1.schema.json"), "$defs", "target")
	metadataTarget := schemaValue(t,
		filepath.Join("..", "standards", "snapshots", "2026.08", "schemas", "golden-path-metadata-v1.schema.json"),
		"$defs", "target")
	if !reflect.DeepEqual(requestTarget, metadataTarget) {
		t.Fatalf("request target schema does not match normative metadata target schema\nrequest: %#v\nmetadata: %#v", requestTarget, metadataTarget)
	}
}

func TestUpgradeAddsExplicitCapabilitiesWithoutMutatingSource(t *testing.T) {
	t.Parallel()
	release := fixtureRelease(t)
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	request := fixtureRequest(t, "node-publisher.yaml")
	legacyRequest := cloneRequest(request)
	legacyRequest.Components[0].Capabilities = nil
	legacyFiles, legacyRequestDigest, err := Render(legacyRequest, release)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(t.TempDir(), "repository")
	if err := WriteStaging(repository, legacyFiles, GeneratePlan(legacyFiles, legacyRequestDigest, bundle, release)); err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(repository, ".github", "golden-path.yaml")
	before, err := os.ReadFile(metadataPath) // #nosec G304 -- test-owned temporary repository.
	if err != nil {
		t.Fatal(err)
	}

	files, requestDigest, err := Render(request, release)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := UpgradePlan(repository, files, requestDigest, bundle, release)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ConflictCount != 0 {
		t.Fatalf("explicit-capability upgrade has %d conflicts", plan.ConflictCount)
	}
	statuses := map[string]string{}
	for _, change := range plan.Changes {
		statuses[change.Path] = change.Status
	}
	for _, path := range []string{".github/golden-path.yaml", ".github/golden-path-request.json"} {
		if statuses[path] != "update" {
			t.Fatalf("%s status = %q, want update", path, statuses[path])
		}
	}
	after, err := os.ReadFile(metadataPath) // #nosec G304 -- test-owned temporary repository.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("upgrade planning mutated the source repository")
	}

	candidate := filepath.Join(t.TempDir(), "candidate")
	if err := WriteStaging(candidate, files, plan); err != nil {
		t.Fatal(err)
	}
	candidateMetadata, err := os.ReadFile(filepath.Join(candidate, ".github", "golden-path.yaml")) // #nosec G304 -- test-owned temporary candidate.
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(candidateMetadata, &metadata); err != nil {
		t.Fatal(err)
	}
	for _, capability := range request.Components[0].Capabilities {
		if !slices.Contains(metadata.Capabilities, capability) {
			t.Fatalf("candidate metadata omits explicit capability %q", capability)
		}
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

func TestBootstrapToAdoptionUpgradePlansRetirementAndProtectsCustomization(t *testing.T) {
	t.Parallel()
	bootstrap := fixtureRequest(t, "single-go.yaml")
	release := fixtureRelease(t)
	bundle, err := LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	bootstrapFiles, bootstrapDigest, err := Render(bootstrap, release)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(t.TempDir(), "repository")
	if err := WriteStaging(repository, bootstrapFiles, GeneratePlan(bootstrapFiles, bootstrapDigest, bundle, release)); err != nil {
		t.Fatal(err)
	}

	adoption := cloneRequest(bootstrap)
	adoption.MaterializationMode = "adoption"
	adoption.Targets = []Target{{OS: "linux", Architecture: "amd64", Tier: "primary"}}
	adoption.Components[0].Capabilities = []string{"build", "test"}
	adoption.Components[0].ModulePath = ""
	adoptionFiles, adoptionDigest, err := Render(adoption, release)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := UpgradePlan(repository, adoptionFiles, adoptionDigest, bundle, release)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ConflictCount != 0 {
		t.Fatalf("clean bootstrap-to-adoption transition has %d conflicts", plan.ConflictCount)
	}
	statuses := make(map[string]string, len(plan.Changes))
	for _, change := range plan.Changes {
		statuses[change.Path] = change.Status
	}
	for _, path := range []string{"go.mod", "justfile", "mise.toml", "cmd/example-go-service/main.go"} {
		if statuses[path] != "remove" {
			t.Fatalf("retired bootstrap asset %s status = %q, want remove", path, statuses[path])
		}
	}
	candidate := filepath.Join(t.TempDir(), "candidate")
	if err := WriteStaging(candidate, adoptionFiles, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, "go.mod")); err != nil {
		t.Fatalf("upgrade candidate construction mutated source go.mod: %v", err)
	}
	if _, err := os.Stat(filepath.Join(candidate, "go.mod")); !os.IsNotExist(err) {
		t.Fatalf("adoption candidate unexpectedly materialized retired go.mod: %v", err)
	}

	customized := []byte("module github.com/5010-dev/customized\n")
	// #nosec G306 -- the test intentionally models a repository-owned 0644 source file.
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), customized, 0o644); err != nil {
		t.Fatal(err)
	}
	conflicted, err := UpgradePlan(repository, adoptionFiles, adoptionDigest, bundle, release)
	if err != nil {
		t.Fatal(err)
	}
	if conflicted.ConflictCount != 1 {
		t.Fatalf("customized bootstrap-to-adoption transition has %d conflicts, want 1", conflicted.ConflictCount)
	}
	conflictStatuses := make(map[string]string, len(conflicted.Changes))
	for _, change := range conflicted.Changes {
		conflictStatuses[change.Path] = change.Status
	}
	if conflictStatuses["go.mod"] != "conflict" {
		t.Fatalf("customized retired go.mod status = %q, want conflict", conflictStatuses["go.mod"])
	}
	conflictedCandidate := filepath.Join(t.TempDir(), "conflicted-candidate")
	if err := WriteStaging(conflictedCandidate, adoptionFiles, conflicted); err == nil {
		t.Fatal("materialized a bootstrap-to-adoption candidate with an unresolved customization conflict")
	}
	if _, err := os.Stat(conflictedCandidate); !os.IsNotExist(err) {
		t.Fatalf("conflicted adoption staging directory exists: %v", err)
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
	programmatic := Request{
		SchemaVersion: RequestSchema, Layout: "single", ProjectName: "Reserved Module", ProjectSlug: "aux",
		Components: []Component{{Name: "service", Path: ".", Profiles: []string{"go"}, ArtifactTypes: []string{"service"}}},
	}
	if _, _, err := Render(programmatic, fixtureRelease(t)); err == nil {
		t.Fatal("Render accepted an invalid derived Go module path")
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
	if err := WriteStaging(staging, files, plan); err == nil {
		t.Fatal("materialized a candidate with unresolved conflicts")
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("conflicted staging directory exists: %v", err)
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
		"if: ${{ always() && steps.golden-path.outcome == 'success' }}",
	} {
		if !strings.Contains(string(reusable), expected) {
			t.Fatalf("reusable workflow does not enforce %q", expected)
		}
	}
	commonJust := string(fileContent(t, files, "just/common.just"))
	if !strings.Contains(commonJust, `MISE_CONFIG_DIR="$PWD/.cache/golden-path/mise-config" mise install --locked`) ||
		strings.Contains(commonJust, "MISE_GLOBAL_CONFIG_FILE") {
		t.Fatal("generated init does not isolate developer-global mise tools")
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

func TestGeneratorReleaseFixtureMatchesPublishedSchema(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile(filepath.Join("..", "testdata", "generator", "release-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	schemaData, err := os.ReadFile(filepath.Join("..", "release", "golden-path-release-manifest-v2.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJSONDocument(schemaData, fixture); err != nil {
		t.Fatalf("generator release fixture does not satisfy the published v2 schema: %v", err)
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
		{name: "unknown-capability", mutate: func(document map[string]any) {
			document["components"].([]any)[0].(map[string]any)["capabilities"] = []any{"release-everything"}
		}},
		{name: "duplicate-capability", mutate: func(document map[string]any) {
			document["components"].([]any)[0].(map[string]any)["capabilities"] = []any{"package", "package"}
		}},
		{name: "empty-capability", mutate: func(document map[string]any) {
			document["components"].([]any)[0].(map[string]any)["capabilities"] = []any{}
		}},
		{name: "null-capability", mutate: func(document map[string]any) {
			document["components"].([]any)[0].(map[string]any)["capabilities"] = nil
		}},
		{name: "unsupported-materialization-mode", mutate: func(document map[string]any) {
			document["materializationMode"] = "merge"
			document["targets"] = []any{map[string]any{"os": "linux", "architecture": "amd64", "tier": "primary"}}
		}},
		{name: "null-materialization-mode", mutate: func(document map[string]any) {
			document["materializationMode"] = nil
		}},
		{name: "empty-materialization-mode", mutate: func(document map[string]any) {
			document["materializationMode"] = ""
		}},
		{name: "explicit-mode-without-target", mutate: func(document map[string]any) {
			document["materializationMode"] = "bootstrap"
		}},
		{name: "adoption-without-explicit-capability", mutate: func(document map[string]any) {
			document["materializationMode"] = "adoption"
			document["targets"] = []any{map[string]any{"os": "linux", "architecture": "amd64", "tier": "primary"}}
		}},
		{name: "empty-targets", mutate: func(document map[string]any) {
			document["targets"] = []any{}
		}},
		{name: "null-targets", mutate: func(document map[string]any) {
			document["targets"] = nil
		}},
		{name: "duplicate-target", mutate: func(document map[string]any) {
			target := map[string]any{"os": "linux", "architecture": "amd64", "tier": "primary"}
			document["targets"] = []any{target, target}
		}},
		{name: "invalid-target-tier", mutate: func(document map[string]any) {
			document["targets"] = []any{map[string]any{"os": "linux", "architecture": "amd64", "tier": "unsupported"}}
		}},
		{name: "evaluation-only-target", mutate: func(document map[string]any) {
			document["targets"] = []any{map[string]any{"os": "linux", "architecture": "amd64", "tier": "evaluation"}}
		}},
		{name: "empty-target-runtime", mutate: func(document map[string]any) {
			document["targets"] = []any{map[string]any{"os": "linux", "architecture": "amd64", "runtime": "", "tier": "primary"}}
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
			if _, err := DecodeRequest(bytes.NewReader(candidate)); err == nil {
				t.Fatal("generator accepted an input outside its request boundary")
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

func findingByRuleID(t *testing.T, result checker.Result, ruleID string) checker.Finding {
	t.Helper()
	for _, finding := range result.Findings {
		if finding.RuleID == ruleID {
			return finding
		}
	}
	t.Fatalf("missing finding for %s", ruleID)
	return checker.Finding{}
}

func schemaStringEnum(t *testing.T, path string, keys ...string) []string {
	t.Helper()
	value := schemaValue(t, path, keys...)
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("schema path %v is not an array", keys)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(string)
		if !ok {
			t.Fatalf("schema path %v contains non-string value", keys)
		}
		result = append(result, entry)
	}
	return result
}

func schemaValue(t *testing.T, path string, keys ...string) any {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- source-controlled schema selected by the test.
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("schema path %v does not resolve through object key %q", keys, key)
		}
		value, ok = object[key]
		if !ok {
			t.Fatalf("schema path %v is missing key %q", keys, key)
		}
	}
	return value
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
	return validateJSONDocument(schemaData, documentData)
}

func validateJSONDocument(schemaData, documentData []byte) error {
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
