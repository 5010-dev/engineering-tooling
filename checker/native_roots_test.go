package checker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateNativeRootsSupportsPolyglotAndIndependentGraphs(t *testing.T) {
	metadata := Metadata{
		Profiles:      []string{"node-typescript", "python", "rust"},
		ArtifactTypes: []string{"application", "binary", "library"},
		Capabilities:  []string{"build", "lint", "test"},
	}
	valid := []MetadataNativeRoot{
		{ID: "python-workspace", Path: ".", Profiles: []string{"python"}},
		{ID: "rust-workspace", Path: ".", Profiles: []string{"rust"}},
		{ID: "web-ui", Path: "apps/web", Profiles: []string{"node-typescript"}},
		{ID: "repository-tools", Path: "tools/repository", Profiles: []string{"node-typescript"}},
	}
	if err := validateNativeRoots(metadata, valid); err != nil {
		t.Fatalf("valid native roots rejected: %v", err)
	}

	tests := []struct {
		name  string
		roots []MetadataNativeRoot
		want  string
	}{
		{name: "duplicate ID", roots: replaceNativeRoot(valid, 1, valid[0]), want: "IDs"},
		{name: "aggregate mismatch", roots: replaceNativeRoot(valid, 2, MetadataNativeRoot{ID: "web-ui", Path: "apps/web", Profiles: []string{"go"}}), want: "subset"},
		{name: "missing profile coverage", roots: valid[1:], want: "python"},
		{name: "same-profile overlap", roots: append(copyNativeRoots(valid), MetadataNativeRoot{ID: "nested-web", Path: "apps/web/internal", Profiles: []string{"node-typescript"}}), want: "overlap"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateNativeRoots(metadata, test.roots)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateNativeRoots() error = %v, want text %q", err, test.want)
			}
		})
	}

	err := validateNativeRoots(
		Metadata{Profiles: []string{"documentation"}},
		[]MetadataNativeRoot{{ID: "documentation-root", Path: ".", Profiles: []string{"documentation"}}},
	)
	if err == nil || !strings.Contains(err.Error(), "non-native profile") {
		t.Fatalf("non-native profile error = %v, want non-native profile rejection", err)
	}
}

func TestNativeRootProfileValidationUnionsProfilesAtOnePath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "pyproject.toml", "[project]\nname = \"polyglot\"\n")
	writeTestFile(t, root, "Cargo.toml", "[package]\nname = \"polyglot\"\n")
	writeTestFile(t, root, "apps/web/package.json", "{}\n")

	roots := []MetadataNativeRoot{
		{ID: "python-workspace", Path: ".", Profiles: []string{"python"}},
		{ID: "rust-workspace", Path: ".", Profiles: []string{"rust"}},
		{ID: "web-ui", Path: "apps/web", Profiles: []string{"node-typescript"}},
	}
	if findingPath, message := validateNativeRootProfileDeclarations(root, roots, nil); message != "" {
		t.Fatalf("valid profile roots = path %q message %q", findingPath, message)
	}
	components := []MetadataComponent{{Path: ".", Profiles: []string{"python", "rust"}}}
	if findingPath, message := validateNativeRootProfileDeclarations(root, roots, components); message != "" {
		t.Fatalf("valid cross-language artifact component = path %q message %q", findingPath, message)
	}
	components[0].Profiles = []string{"rust"}
	if findingPath, message := validateNativeRootProfileDeclarations(root, roots, components); findingPath != "pyproject.toml" || !strings.Contains(message, "requires one of the declared profiles: python") {
		t.Fatalf("under-declared cross-language artifact = path %q message %q", findingPath, message)
	}

	roots[2].Profiles = []string{"python"}
	if findingPath, message := validateNativeRootProfileDeclarations(root, roots, nil); findingPath != "apps/web/package.json" || message == "" {
		t.Fatalf("under-declared Node root = path %q message %q", findingPath, message)
	}
}

func TestNativeRootProfileValidationChecksRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/undeclared-root\n")
	writeTestFile(t, root, "apps/web/package.json", "{}\n")

	roots := []MetadataNativeRoot{
		{ID: "web-ui", Path: "apps/web", Profiles: []string{"node-typescript"}},
	}
	if findingPath, message := validateNativeRootProfileDeclarations(root, roots, nil); findingPath != "go.mod" || message == "" {
		t.Fatalf("undeclared repository-root marker = path %q message %q", findingPath, message)
	}
}

func TestNativeRootProfileValidationRequiresComponentMarkersToBeCovered(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "apps/web/package.json", "{}\n")
	writeTestFile(t, root, "packages/lib/package.json", "{}\n")
	components := []MetadataComponent{
		{Path: "apps/web", Profiles: []string{"node-typescript"}},
		{Path: "packages/lib", Profiles: []string{"node-typescript"}},
	}

	isolatedRoot := []MetadataNativeRoot{
		{ID: "web-ui", Path: "apps/web", Profiles: []string{"node-typescript"}},
	}
	if findingPath, message := validateNativeRootProfileDeclarations(root, isolatedRoot, components); findingPath != "packages/lib/package.json" || !strings.Contains(message, "not covered") {
		t.Fatalf("uncovered component marker = path %q message %q", findingPath, message)
	}

	workspaceRoot := []MetadataNativeRoot{
		{ID: "node-workspace", Path: ".", Profiles: []string{"node-typescript"}},
	}
	if findingPath, message := validateNativeRootProfileDeclarations(root, workspaceRoot, components); findingPath != "" || message != "" {
		t.Fatalf("workspace-covered component marker = path %q message %q", findingPath, message)
	}

	writeTestFile(t, root, "tools/compiler/build.zig", "const std = @import(\"std\");\n")
	zigToolchainComponent := append(copyMetadataComponents(components), MetadataComponent{
		Path: "tools/compiler", Profiles: []string{"zig-toolchain"},
	})
	wrongAlternativeRoot := append(copyNativeRoots(workspaceRoot), MetadataNativeRoot{
		ID: "zig-package", Path: ".", Profiles: []string{"zig"},
	})
	if findingPath, message := validateNativeRootProfileDeclarations(root, wrongAlternativeRoot, zigToolchainComponent); findingPath != "tools/compiler/build.zig" || !strings.Contains(message, "not covered") {
		t.Fatalf("wrong alternative profile root = path %q message %q", findingPath, message)
	}
}

func TestNativeRootRuleEvaluatesIndependentNodeGraphs(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "apps/web/package.json", "{}\n")
	writeTestFile(t, root, "apps/web/pnpm-lock.yaml", "lockfileVersion: '9.0'\n")
	writeTestFile(t, root, "tools/repository/package.json", "{}\n")

	metadata := Metadata{NativeRoots: []MetadataNativeRoot{
		{ID: "web-ui", Path: "apps/web", Profiles: []string{"node-typescript"}},
		{ID: "repository-tools", Path: "tools/repository", Profiles: []string{"node-typescript"}},
	}}
	rule := Rule{
		ID: "DT-DEP-001", Level: "MUST", Assessment: "automated", Severity: "error",
		Applicability: RuleApplicability{Profiles: []string{"*"}},
	}
	finding := evaluateNativeRootRule(root, metadata, rule, nil, false, fixtureTime, CompatibilityManifest{})
	if finding.Status != "fail" || finding.Path != "tools/repository/pnpm-lock.yaml" {
		t.Fatalf("independent Node graph failure = %+v", finding)
	}
	if finding.Extensions["nativeRootId"] != "repository-tools" || finding.Extensions["nativeRootPath"] != "tools/repository" {
		t.Fatalf("native root identity missing from finding: %+v", finding.Extensions)
	}
}

func TestNativeRootExceptionClassificationScopesDoNotMatch(t *testing.T) {
	metadata := Metadata{
		ArtifactTypes: []string{"tooling"},
		Capabilities:  []string{"build"},
		NativeRoots: []MetadataNativeRoot{
			{ID: "repository-tools", Path: "tools/repository", Profiles: []string{"node-typescript"}},
		},
	}
	finding := Finding{
		RuleID: "DT-DEP-001",
		Path:   "tools/repository/pnpm-lock.yaml",
		Extensions: map[string]any{
			"nativeRootId": "repository-tools",
		},
	}
	rule := Rule{ID: "DT-DEP-001", Waivable: true}
	nativeRootMetadata := exceptionMetadata(metadata, finding)

	tests := []struct {
		name  string
		scope ExceptionScope
	}{
		{name: "artifact type", scope: ExceptionScope{ArtifactTypes: []string{"tooling"}}},
		{name: "capability", scope: ExceptionScope{Capabilities: []string{"build"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exceptions := []Exception{{
				ID: "EXC-2026-TEST", Rules: []string{"DT-DEP-001"}, Scope: test.scope, ExpiresAt: "2026-08-31",
			}}
			if exception, _ := matchingException(exceptions, nativeRootMetadata, finding, rule, fixtureTime); exception != nil {
				t.Fatalf("%s-scoped exception matched native root: %+v", test.name, exception)
			}
		})
	}
}

func TestNativeRootPathExceptionDoesNotMatchDifferentRoot(t *testing.T) {
	metadata := Metadata{NativeRoots: []MetadataNativeRoot{
		{ID: "web-ui", Path: "apps/web", Profiles: []string{"node-typescript"}},
		{ID: "repository-tools", Path: "tools/repository", Profiles: []string{"node-typescript"}},
	}}
	finding := Finding{
		RuleID: "DT-DEP-001",
		Path:   "tools/repository/pnpm-lock.yaml",
		Extensions: map[string]any{
			"nativeRootId": "repository-tools",
		},
	}
	exceptions := []Exception{{
		ID: "EXC-2026-TEST", Rules: []string{"DT-DEP-001"}, ExpiresAt: "2026-08-31",
		Scope: ExceptionScope{Profiles: []string{"node-typescript"}, Paths: []string{"apps/web/pnpm-lock.yaml"}},
	}}

	if exception, _ := matchingException(
		exceptions,
		exceptionMetadata(metadata, finding),
		finding,
		Rule{ID: "DT-DEP-001", Waivable: true},
		fixtureTime,
	); exception != nil {
		t.Fatalf("exception for web-ui matched repository-tools finding: %+v", exception)
	}
}

func TestNativeRootRulePrefersAnUnwaivedFailure(t *testing.T) {
	metadata := Metadata{NativeRoots: []MetadataNativeRoot{
		{ID: "web-ui", Path: "apps/web", Profiles: []string{"node-typescript"}},
		{ID: "repository-tools", Path: "tools/repository", Profiles: []string{"node-typescript"}},
	}}
	rule := Rule{
		ID: "DT-DEP-001", Level: "MUST", Assessment: "automated", Severity: "error", Waivable: true,
		Applicability: RuleApplicability{Profiles: []string{"node-typescript"}},
	}
	exceptions := []Exception{{
		ID: "EXC-2026-TEST", Rules: []string{"DT-DEP-001"}, ExpiresAt: "2026-08-31",
		Scope: ExceptionScope{Profiles: []string{"node-typescript"}, Paths: []string{"apps/web/package.json"}},
	}}

	finding := evaluateNativeRootRule(t.TempDir(), metadata, rule, exceptions, true, fixtureTime, CompatibilityManifest{})
	if finding.Status != "fail" || finding.Path != "tools/repository/package.json" || finding.Extensions["nativeRootId"] != "repository-tools" {
		t.Fatalf("native root selection masked an unwaived failure: %+v", finding)
	}
}

func TestNativeRootExceptionDoesNotCrossProfilesAtOneRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "pyproject.toml", "[project]\nname = \"polyglot\"\n")
	writeTestFile(t, root, "uv.lock", "version = 1\nrevision = 3\n")

	metadata := Metadata{NativeRoots: []MetadataNativeRoot{{
		ID: "polyglot-workspace", Path: ".", Profiles: []string{"python", "rust"},
	}}}
	rule := Rule{
		ID: "DT-DEP-001", Level: "MUST", Assessment: "automated", Severity: "error", Waivable: true,
		Applicability: RuleApplicability{Profiles: []string{"*"}},
	}
	exceptions := []Exception{{
		ID: "EXC-2026-TEST", Rules: []string{"DT-DEP-001"}, ExpiresAt: "2026-08-31",
		Scope: ExceptionScope{Profiles: []string{"python"}},
	}}

	finding := evaluateNativeRootRule(root, metadata, rule, exceptions, true, fixtureTime, CompatibilityManifest{})
	if finding.Status != "fail" || finding.Path != "Cargo.toml" || finding.Extensions["nativeRootProfile"] != "rust" {
		t.Fatalf("Python exception masked Rust dependency failure: %+v", finding)
	}
	if exception, _ := matchingException(exceptions, exceptionMetadata(metadata, finding), finding, rule, fixtureTime); exception != nil {
		t.Fatalf("Python exception matched Rust dependency failure: %+v", exception)
	}
}

func TestGoWorkspaceNativeRootEvaluatesReferencedModules(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "mise.toml", "min_version = \"2026.7.18\"\n[tools]\ngo = \"1.26.5\"\n")
	writeTestFile(t, root, "mise.lock", "[tools]\ngo = [{ version = \"1.26.5\" }]\n")
	writeTestFile(t, root, "go.work", "go 1.26.0\ntoolchain go1.26.5\nuse (\n  ./modules/one\n  ./modules/two\n)\n")
	writeTestFile(t, root, "modules/one/go.mod", "module example.com/one\n\ngo 1.26.0\n\nrequire example.com/two v0.0.0\n")
	writeTestFile(t, root, "modules/two/go.mod", "module example.com/two\n\ngo 1.26.0\n\nrequire example.net/private/dependency v1.0.0\n")
	writeTestFile(t, root, "modules/two/go.sum", "example.net/private/dependency v1.0.0 h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n")

	metadata := Metadata{Profiles: []string{"go"}, ComponentPath: "."}
	dependencyRule := Rule{ID: "DT-DEP-001", Level: "MUST", Assessment: "automated", Severity: "error"}
	if finding := evaluateDependencyRecords(root, metadata, dependencyRule, false); finding.Status != "pass" {
		t.Fatalf("Go workspace dependency records = %+v", finding)
	}
	authorityRule := Rule{ID: "DT-GO-001", Level: "MUST", Assessment: "automated", Severity: "error"}
	if finding := evaluateGoAuthority(root, metadata, authorityRule, false); finding.Status != "pass" || finding.Path != "go.work" {
		t.Fatalf("Go workspace authority = %+v", finding)
	}
	dependencyIntegrityRule := Rule{ID: "DT-GO-002", Level: "MUST", Assessment: "automated", Severity: "error"}
	if finding := evaluateGoDependencies(root, metadata, dependencyIntegrityRule, false); finding.Status != "skip" || finding.Path != "go.work" {
		t.Fatalf("Go workspace module integrity = %+v", finding)
	}

	if err := os.Remove(filepath.Join(root, "modules", "two", "go.sum")); err != nil {
		t.Fatal(err)
	}
	if finding := evaluateGoDependencies(root, metadata, dependencyIntegrityRule, false); finding.Status != "fail" || finding.Path != "modules/two/go.sum" {
		t.Fatalf("Go workspace missing module sum = %+v", finding)
	}
}

func TestGoAuthorityAllowsDefaultButRejectsMismatchedPrereleaseToolchain(t *testing.T) {
	tests := []struct {
		name          string
		authorityPath string
		setup         map[string]string
	}{
		{
			name:          "module",
			authorityPath: "go.mod",
			setup: map[string]string{
				"go.mod": "module example.com/toolchain\n\ngo 1.26.0\ntoolchain %s\n",
			},
		},
		{
			name:          "workspace",
			authorityPath: "go.work",
			setup: map[string]string{
				"go.work":       "go 1.26.0\ntoolchain %s\nuse ./module\n",
				"module/go.mod": "module example.com/workspace-module\n\ngo 1.26.0\n",
			},
		},
	}
	rule := Rule{ID: "DT-GO-001", Level: "MUST", Assessment: "automated", Severity: "error"}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, root, "mise.toml", "min_version = \"2026.7.18\"\n[tools]\ngo = \"1.26.5\"\n")
			writeTestFile(t, root, "mise.lock", "[tools]\ngo = [{ version = \"1.26.5\" }]\n")
			for name, content := range test.setup {
				writeTestFile(t, root, name, strings.ReplaceAll(content, "%s", "default"))
			}
			metadata := Metadata{Profiles: []string{"go"}, ComponentPath: "."}
			if finding := evaluateGoAuthority(root, metadata, rule, false); finding.Status != "pass" {
				t.Fatalf("toolchain default = %+v", finding)
			}

			for name, content := range test.setup {
				writeTestFile(t, root, name, strings.ReplaceAll(content, "%s", "go1.27rc1"))
			}
			if finding := evaluateGoAuthority(root, metadata, rule, false); finding.Status != "fail" || finding.Path != test.authorityPath {
				t.Fatalf("mismatched prerelease toolchain = %+v", finding)
			}
		})
	}
}

func TestGoWorkspaceAuthorityChecksUnusedSiblingRootModule(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "mise.toml", "min_version = \"2026.7.18\"\n[tools]\ngo = \"1.26.5\"\n")
	writeTestFile(t, root, "mise.lock", "[tools]\ngo = [{ version = \"1.26.5\" }]\n")
	writeTestFile(t, root, "go.work", "go 1.26.0\ntoolchain go1.26.5\nuse ./modules/one\n")
	writeTestFile(t, root, "modules/one/go.mod", "module example.com/one\n\ngo 1.26.0\n")
	writeTestFile(t, root, "go.mod", "module example.com/root\n\ngo 1.99.0\ntoolchain go1.99.9\n")
	metadata := Metadata{Profiles: []string{"go"}, ComponentPath: "."}
	rule := Rule{ID: "DT-GO-001", Level: "MUST", Assessment: "automated", Severity: "error"}

	if finding := evaluateGoAuthority(root, metadata, rule, false); finding.Status != "fail" || finding.Path != "go.mod" {
		t.Fatalf("unused sibling root module = %+v", finding)
	}
}

func TestIaCNativeRootRequiresManifestLockAndImmutableModules(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "infra/main.tf", `terraform {
  required_version = "= 1.15.8"
}

module "network" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "6.0.1"
}
`)
	writeTestFile(t, root, "infra/.terraform.lock.hcl", "provider \"registry.terraform.io/hashicorp/aws\" {\n  version = \"6.10.0\"\n  hashes = [\"h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\"]\n}\n")

	metadata := Metadata{Profiles: []string{"infrastructure-terraform"}, ComponentPath: "infra"}
	dependencyRule := Rule{ID: "DT-DEP-001", Level: "MUST", Assessment: "automated", Severity: "error"}
	if finding := evaluateDependencyRecords(root, metadata, dependencyRule, false); finding.Status != "pass" {
		t.Fatalf("Terraform dependency records = %+v", finding)
	}
	directRule := Rule{ID: "DT-DEP-004", Level: "MUST", Assessment: "automated", Severity: "error"}
	if finding := evaluateDirectDependencies(root, metadata, directRule, false); finding.Status != "skip" {
		t.Fatalf("Terraform immutable module = %+v", finding)
	}

	writeTestFile(t, root, "infra/main.tf", `module "network" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 6.0"
}
`)
	if finding := evaluateDirectDependencies(root, metadata, directRule, false); finding.Status != "fail" || finding.Path != "infra/main.tf" {
		t.Fatalf("Terraform floating module version = %+v", finding)
	}

	if err := os.Remove(filepath.Join(root, "infra", ".terraform.lock.hcl")); err != nil {
		t.Fatal(err)
	}
	if finding := evaluateDependencyRecords(root, metadata, dependencyRule, false); finding.Status != "fail" || finding.Path != "infra/.terraform.lock.hcl" {
		t.Fatalf("Terraform missing lock = %+v", finding)
	}
}

func TestTerraformAcceptsExactEqualsAndPinnedBitbucketSources(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "infra/main.tf", `module "network" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "= 6.0.1"
}

module "consul" {
  source = "bitbucket.org/hashicorp/terraform-consul-aws?ref=0123456789abcdef0123456789abcdef01234567"
}

module "gitlab_registry" {
  source  = "gitlab.com/example/network/aws"
  version = "= 1.2.3"
}
`)
	metadata := Metadata{Profiles: []string{"infrastructure-terraform"}, ComponentPath: "infra"}
	rule := Rule{ID: "DT-DEP-004", Level: "MUST", Assessment: "automated", Severity: "error"}

	if finding := evaluateDirectDependencies(root, metadata, rule, false); finding.Status != "skip" || !strings.Contains(finding.Message, "Detected direct references") {
		t.Fatalf("exact registry and pinned Bitbucket sources = %+v", finding)
	}

	writeTestFile(t, root, "infra/main.tf", `module "consul" {
  source = "bitbucket.org/hashicorp/terraform-consul-aws"
}
`)
	if finding := evaluateDirectDependencies(root, metadata, rule, false); finding.Status != "fail" || finding.Secondary != "consul" {
		t.Fatalf("unpinned Bitbucket source = %+v", finding)
	}
}

func TestTerraformFollowsBoundedRepositoryLocalModuleGraph(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "infra/main.tf", `module "network" {
  source = "../modules/network"
}
`)
	writeTestFile(t, root, "modules/network/main.tf", `module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 6.0"
}
`)
	metadata := Metadata{Profiles: []string{"infrastructure-terraform"}, ComponentPath: "infra"}
	rule := Rule{ID: "DT-DEP-004", Level: "MUST", Assessment: "automated", Severity: "error"}

	if finding := evaluateDirectDependencies(root, metadata, rule, false); finding.Status != "fail" || finding.Path != "modules/network/main.tf" || finding.Secondary != "vpc" {
		t.Fatalf("floating dependency in repository-local module = %+v", finding)
	}

	writeTestFile(t, root, "modules/network/main.tf", `module "vpc" {
  source = "git::https://example.com/vpc.git"
}
`)
	if finding := evaluateDirectDependencies(root, metadata, rule, false); finding.Status != "fail" || finding.Path != "modules/network/main.tf" || finding.Secondary != "vpc" {
		t.Fatalf("unpinned VCS dependency in repository-local module = %+v", finding)
	}

	writeTestFile(t, root, "modules/network/main.tf", `module "vpc" {
  source = "git::https://example.com/vpc.git?ref=0123456789abcdef0123456789abcdef01234567"
}

module "cycle" {
  source = "../../infra"
}
`)
	if finding := evaluateDirectDependencies(root, metadata, rule, false); finding.Status != "skip" || !strings.Contains(finding.Message, "Detected direct references") {
		t.Fatalf("bounded local-module cycle with immutable dependency = %+v", finding)
	}
}

func TestTerraformJSONConfigurationIsAValidEvaluatedRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "infra/main.tf.json", `{
  "module": {
    "network": {
      "source": "terraform-aws-modules/vpc/aws",
      "version": "= 6.0.1"
    }
  }
}
`)
	writeTestFile(t, root, "infra/.terraform.lock.hcl", "# lock\n")
	metadata := Metadata{Profiles: []string{"infrastructure-terraform"}, ComponentPath: "infra"}
	manifestRule := Rule{ID: "DT-DEP-001", Level: "MUST", Assessment: "automated", Severity: "error"}
	if finding := evaluateDependencyRecords(root, metadata, manifestRule, false); finding.Status != "pass" {
		t.Fatalf("Terraform JSON manifest and lock = %+v", finding)
	}

	directRule := Rule{ID: "DT-DEP-004", Level: "MUST", Assessment: "automated", Severity: "error"}
	if finding := evaluateDirectDependencies(root, metadata, directRule, false); finding.Status != "skip" || !strings.Contains(finding.Message, "Detected direct references") {
		t.Fatalf("Terraform JSON exact module = %+v", finding)
	}

	writeTestFile(t, root, "infra/main.tf.json", `{
  "module": {
    "network": {
      "source": "terraform-aws-modules/vpc/aws",
      "version": "~> 6.0"
    }
  }
}
`)
	if finding := evaluateDirectDependencies(root, metadata, directRule, false); finding.Status != "fail" || finding.Path != "infra/main.tf.json" {
		t.Fatalf("Terraform JSON floating module = %+v", finding)
	}
}

func TestIaCNativeRootManifestRequirementsCoverEveryProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		files   map[string]string
	}{
		{name: "AWS CDK", profile: "infrastructure-aws-cdk", files: map[string]string{"cdk.json": "{}\n"}},
		{name: "Terraform", profile: "infrastructure-terraform", files: map[string]string{"main.tf": "terraform {}\n", ".terraform.lock.hcl": "# lock\n"}},
		{name: "OpenTofu", profile: "infrastructure-opentofu", files: map[string]string{"main.tf": "terraform {}\n", ".terraform.lock.hcl": "# lock\n"}},
		{name: "Pulumi", profile: "infrastructure-pulumi", files: map[string]string{"Pulumi.yaml": "name: example\nruntime: nodejs\n"}},
	}
	rule := Rule{ID: "DT-DEP-001", Level: "MUST", Assessment: "automated", Severity: "error"}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range test.files {
				writeTestFile(t, root, name, content)
			}
			metadata := Metadata{Profiles: []string{test.profile}, ComponentPath: "."}
			if finding := evaluateDependencyRecords(root, metadata, rule, false); finding.Status != "pass" {
				t.Fatalf("%s dependency records = %+v", test.profile, finding)
			}
			for name := range test.files {
				if err := os.Remove(filepath.Join(root, name)); err != nil {
					t.Fatal(err)
				}
				break
			}
			if finding := evaluateDependencyRecords(root, metadata, rule, false); finding.Status != "fail" {
				t.Fatalf("%s missing manifest or lock = %+v", test.profile, finding)
			}
		})
	}
}

func TestNativeRootRuleDoesNotMaskIaCDependencyFailure(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "apps/web/package.json", "{}\n")
	writeTestFile(t, root, "apps/web/pnpm-lock.yaml", "lockfileVersion: '9.0'\n")
	writeTestFile(t, root, "infra/main.tf", "terraform { required_version = \"= 1.15.8\" }\n")

	metadata := Metadata{NativeRoots: []MetadataNativeRoot{
		{ID: "web-ui", Path: "apps/web", Profiles: []string{"node-typescript"}},
		{ID: "infrastructure", Path: "infra", Profiles: []string{"infrastructure-terraform"}},
	}}
	rule := Rule{
		ID: "DT-DEP-001", Level: "MUST", Assessment: "automated", Severity: "error",
		Applicability: RuleApplicability{Profiles: []string{"*"}},
	}
	finding := evaluateNativeRootRule(root, metadata, rule, nil, false, fixtureTime, CompatibilityManifest{})
	if finding.Status != "fail" || finding.Path != "infra/.terraform.lock.hcl" || finding.Extensions["nativeRootProfile"] != "infrastructure-terraform" {
		t.Fatalf("language pass masked IaC dependency failure: %+v", finding)
	}
}

func TestCheckAcceptsPolyglotSamePathNativeRoots(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".github/golden-path.yaml", `schemaVersion: golden-path-metadata/v1
contractVersion: golden-path/v1
standardVersion: "2026.08.5"
assetBundleVersion: 1.1.0
profiles:
  - node-typescript
  - python
  - rust
artifactTypes:
  - application
capabilities:
  - build
`)
	writeTestFile(t, root, ".github/golden-path-native-roots.yaml", `schemaVersion: golden-path-native-roots/v1
roots:
  - id: python-workspace
    path: .
    profiles:
      - python
  - id: rust-workspace
    path: .
    profiles:
      - rust
  - id: web-ui
    path: apps/web
    profiles:
      - node-typescript
  - id: repository-tools
    path: tools/repository
    profiles:
      - node-typescript
`)
	writeTestFile(t, root, "pyproject.toml", "[project]\nname = \"polyglot\"\nversion = \"0.0.0\"\n")
	writeTestFile(t, root, "uv.lock", "version = 1\nrevision = 3\n")
	writeTestFile(t, root, "Cargo.toml", "[package]\nname = \"polyglot\"\nversion = \"0.0.0\"\nedition = \"2024\"\n")
	writeTestFile(t, root, "Cargo.lock", "version = 4\n")
	writeTestFile(t, root, "apps/web/package.json", "{}\n")
	writeTestFile(t, root, "apps/web/pnpm-lock.yaml", "lockfileVersion: '9.0'\n")
	writeTestFile(t, root, "tools/repository/package.json", "{}\n")
	writeTestFile(t, root, "tools/repository/pnpm-lock.yaml", "lockfileVersion: '9.0'\n")

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode == 2 || result.ExitCode == 3 || !result.Complete {
		t.Fatalf("valid native-root declaration was rejected: exit=%d complete=%t findings=%+v", result.ExitCode, result.Complete, result.Findings)
	}
	finding := findingByRule(result, "DT-DEP-001")
	if finding.Status != "pass" || finding.Extensions["nativeRootId"] == nil {
		t.Fatalf("native dependency roots were not evaluated independently: %+v", finding)
	}
}

func TestCheckRejectsSemanticNativeRootMismatchWithDetail(t *testing.T) {
	root := copyFixture(t, "positive-go")
	writeTestFile(t, root, ".github/golden-path-native-roots.yaml", `schemaVersion: golden-path-native-roots/v1
roots:
  - id: python-workspace
    path: .
    profiles:
      - python
`)

	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	finding := findingByRule(result, "DT-META-001")
	if result.ExitCode != 2 || result.Complete || finding.Path != ".github/golden-path-native-roots.yaml" || !strings.Contains(finding.Message, "subset of aggregate metadata") {
		t.Fatalf("semantic native-root mismatch did not fail closed with detail: exit=%d complete=%t finding=%+v", result.ExitCode, result.Complete, finding)
	}
}

func TestCheckWithoutNativeRootsPreservesComponentFallback(t *testing.T) {
	root := copyFixture(t, "positive-go")
	result := Check(Options{Root: root, EvaluatedAt: fixtureTime, Enforcement: "report-only"})
	if result.ExitCode != 0 || !result.Complete {
		t.Fatalf("no-sidecar fixture changed behavior: exit=%d complete=%t findings=%+v", result.ExitCode, result.Complete, result.Findings)
	}
	for _, finding := range result.Findings {
		if finding.Extensions["nativeRootId"] != nil {
			t.Fatalf("no-sidecar finding acquired native-root identity: %+v", finding)
		}
	}
}

func TestNativeRootScopedRulesRemainProfileScoped(t *testing.T) {
	catalog, _, err := loadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range catalog.Rules {
		if !componentScopedRules[rule.ID] {
			continue
		}
		if len(rule.Applicability.ArtifactTypes) != 0 || len(rule.Applicability.Capabilities) != 0 {
			t.Errorf("native-root rule %s requires artifact or capability classification", rule.ID)
		}
	}
}

func replaceNativeRoot(values []MetadataNativeRoot, index int, replacement MetadataNativeRoot) []MetadataNativeRoot {
	result := copyNativeRoots(values)
	result[index] = replacement
	return result
}

func copyNativeRoots(values []MetadataNativeRoot) []MetadataNativeRoot {
	return append([]MetadataNativeRoot(nil), values...)
}

func copyMetadataComponents(values []MetadataComponent) []MetadataComponent {
	return append([]MetadataComponent(nil), values...)
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
