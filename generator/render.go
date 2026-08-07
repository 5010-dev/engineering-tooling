package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"text/template"

	bundlefs "github.com/5010-dev/engineering-tooling/templates"
)

type recipeModel struct {
	Components         []recipeComponent
	InitRecipes        []string
	FormatRecipes      []string
	FormatCheckRecipes []string
	LintRecipes        []string
	TypecheckRecipes   []string
	TestRecipes        []string
	BuildRecipes       []string
	CheckRecipes       []string
	CIRecipes          []string
	HasFormat          bool
	HasFormatCheck     bool
	HasLint            bool
	HasTypecheck       bool
	HasTest            bool
	HasBuild           bool
}

type recipeComponent struct {
	RecipeFile string
}

type componentModel struct {
	ProjectName                     string
	ProjectNameQuoted               string
	ProjectNameTypeScriptQuoted     string
	ProjectNamePythonQuoted         string
	ProjectNameJustShellQuoted      string
	StarterDescriptionQuoted        string
	StarterDescriptionPythonQuoted  string
	InfrastructureDescriptionQuoted string
	ProjectSlug                     string
	PackageName                     string
	GoPackage                       string
	PythonModule                    string
	PythonVersion                   string
	GoVersion                       string
	GoLanguageVersion               string
	GoImportsVersion                string
	NodeVersion                     string
	NodeNextMajor                   string
	RustVersion                     string
	ZigVersion                      string
	ModulePath                      string
	RecipePrefix                    string
	ShellPath                       string
	ZigPackageName                  string
	ZigFingerprint                  string
	StackName                       string
	Engine                          string
	EngineVersion                   string
	PNPMVersion                     string
	PNPMArchiveSHA256               string
	PNPMExecutableSHA256            string
	ESLintJSVersion                 string
	NodeTypesVersion                string
	ESLintVersion                   string
	PrettierVersion                 string
	TypeScriptVersion               string
	TypeScriptESLintVersion         string
	VitestVersion                   string
	UVBuildVersion                  string
	MypyVersion                     string
	PytestVersion                   string
	RuffVersion                     string
	AWSCDKVersion                   string
	AWSCDKLibVersion                string
	ConstructsVersion               string
	PulumiSDKVersion                string
	AWSCDK                          bool
	Pulumi                          bool
	GoExecutable                    bool
	GoLibrary                       bool
	NodeExecutable                  bool
	PythonExecutable                bool
	RustExecutable                  bool
	RustLibrary                     bool
	HasNodeRuntimeDependencies      bool
}

type automationModel struct {
	AutomationSourceCommit      string
	CheckerVersion              string
	GitHubCLIVersion            string
	ProfilesJSON                string
	DarwinAMD64SHA256           string
	DarwinAMD64ExecutableSHA256 string
	DarwinARM64SHA256           string
	DarwinARM64ExecutableSHA256 string
	LinuxAMD64SHA256            string
	LinuxAMD64ExecutableSHA256  string
	LinuxARM64SHA256            string
	LinuxARM64ExecutableSHA256  string
}

type dependabotEntry struct {
	Ecosystem string
	Directory string
}

type dependabotModel struct {
	Dependabot []dependabotEntry
}

type onboardingModel struct {
	ProjectName string
}

func Render(request Request, release ReleaseManifest) ([]File, string, error) {
	request = cloneRequest(request)
	if err := validateRequest(&request); err != nil {
		return nil, "", fmt.Errorf("validate generator request: %w", err)
	}
	bundle, err := LoadBundle()
	if err != nil {
		return nil, "", err
	}
	if err := ValidateRelease(bundle, release); err != nil {
		return nil, "", err
	}
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, "", fmt.Errorf("encode canonical request: %w", err)
	}
	requestDigest := digest(requestData)
	mode := materializationMode(request)
	profiles, artifactTypes, capabilities := aggregateMetadata(request, mode)
	componentMetadata := make([]map[string]any, 0, len(request.Components))
	for _, component := range request.Components {
		componentMetadata = append(componentMetadata, map[string]any{
			"path": component.Path, "profiles": component.Profiles, "artifactTypes": component.ArtifactTypes,
			"capabilities": metadataCapabilities(component, mode),
		})
	}
	automation, err := buildAutomationModel(release, profiles)
	if err != nil {
		return nil, "", err
	}

	var files []File
	addTemplate := func(target, source string, mode fs.FileMode, data any) error {
		content, renderErr := renderTemplate(source, data)
		if renderErr != nil {
			return renderErr
		}
		files = append(files, File{Path: target, Mode: mode, Content: content, Source: source})
		return nil
	}
	type templateAsset struct {
		target string
		source string
		mode   fs.FileMode
		data   any
	}
	commonTemplates := []templateAsset{
		{".github/workflows/developer-tooling.yml", "common/.github/workflows/developer-tooling.yml.tmpl", 0o644, automation},
		{"scripts/golden-path", "common/scripts/golden-path.tmpl", 0o755, automation},
	}
	if mode == "bootstrap" {
		recipes := buildRecipeModel(request)
		dependabot := buildDependabotModel(request)
		onboarding := onboardingModel{ProjectName: request.ProjectName}
		commonTemplates = append(commonTemplates,
			templateAsset{"README.md", "common/README.md.tmpl", 0o644, onboarding},
			templateAsset{"justfile", "common/justfile.tmpl", 0o644, recipes},
			templateAsset{"just/common.just", "common/just/common.just.tmpl", 0o644, recipes},
			templateAsset{".github/dependabot.yml", "common/.github/dependabot.yml.tmpl", 0o644, dependabot},
			templateAsset{".github/golden-path-exceptions.yaml", "common/.github/golden-path-exceptions.yaml.tmpl", 0o644, nil},
			templateAsset{".github/workflows/quality.yml", "common/.github/workflows/quality.yml.tmpl", 0o644, nil},
			templateAsset{".gitignore", "common/.gitignore.tmpl", 0o644, nil},
		)
	}
	for _, entry := range commonTemplates {
		if err := addTemplate(entry.target, entry.source, entry.mode, entry.data); err != nil {
			return nil, "", err
		}
	}

	if mode == "bootstrap" {
		selectedTools := selectedMiseTools(profiles)
		miseTOML, miseErr := renderMiseTOML(bundle, selectedTools)
		if miseErr != nil {
			return nil, "", miseErr
		}
		files = append(files, File{Path: "mise.toml", Mode: 0o644, Content: miseTOML, Source: "tooling/mise.toml"})
		miseLock, lockErr := filterMiseLock(selectedTools)
		if lockErr != nil {
			return nil, "", lockErr
		}
		files = append(files, File{Path: "mise.lock", Mode: 0o644, Content: miseLock, Source: "tooling/mise.lock"})

		for _, component := range request.Components {
			componentFiles, componentErr := renderComponent(component, request, bundle)
			if componentErr != nil {
				return nil, "", componentErr
			}
			files = append(files, componentFiles...)
		}
	}

	generatorExtension := map[string]any{
		"layout": request.Layout, "requestSHA256": requestDigest, "components": componentMetadata,
	}
	if request.MaterializationMode != "" {
		generatorExtension["materializationMode"] = mode
	}
	metadata := map[string]any{
		"schemaVersion":      "golden-path-metadata/v1",
		"contractVersion":    bundle.ContractVersion,
		"standardVersion":    bundle.StandardVersion,
		"assetBundleVersion": bundle.AssetBundleVersion,
		"profiles":           profiles,
		"artifactTypes":      artifactTypes,
		"capabilities":       capabilities,
		"extensions": map[string]any{
			"dev.fiftyten.generator": generatorExtension,
		},
	}
	if len(request.Targets) > 0 {
		metadata["targets"] = request.Targets
	}
	metadataData, err := indentJSON(metadata)
	if err != nil {
		return nil, "", err
	}
	files = append(files, File{Path: ".github/golden-path.yaml", Mode: 0o644, Content: metadataData, Source: "generator/metadata"})
	canonicalRequest, err := indentJSON(request)
	if err != nil {
		return nil, "", err
	}
	files = append(files, File{Path: ".github/golden-path-request.json", Mode: 0o644, Content: canonicalRequest, Source: "generator/request"})

	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	if err := validateRenderedFiles(files); err != nil {
		return nil, "", err
	}
	assetManifest := AssetManifest{
		SchemaVersion:      AssetsSchema,
		StandardVersion:    bundle.StandardVersion,
		AssetBundleVersion: bundle.AssetBundleVersion,
		ReleaseVersion:     release.ReleaseVersion,
		Source:             release.Source,
		RequestSHA256:      requestDigest,
		Files:              make([]GeneratedAsset, 0, len(files)),
	}
	for _, file := range files {
		if !isManagedAssetPath(file.Path) {
			continue
		}
		assetManifest.Files = append(assetManifest.Files, GeneratedAsset{
			Path: file.Path, Mode: fmt.Sprintf("%04o", file.Mode.Perm()), SHA256: digest(file.Content), Source: file.Source,
		})
	}
	assetData, err := indentJSON(assetManifest)
	if err != nil {
		return nil, "", err
	}
	files = append(files, File{Path: ".github/golden-path-assets.json", Mode: 0o644, Content: assetData, Source: "generator/assets-manifest"})
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	return files, requestDigest, nil
}

func renderComponent(component Component, request Request, bundle Bundle) ([]File, error) {
	profiles := stringSet(component.Profiles)
	executableArtifact := hasExecutableArtifact(component)
	applicationExecutable := executableArtifact && !hasArtifact(component, "infrastructure")
	model := componentModel{
		ProjectName: request.ProjectName, ProjectNameQuoted: strconv.Quote(request.ProjectName),
		ProjectNameTypeScriptQuoted:     formatterPreferredQuotedString(request.ProjectName),
		ProjectNamePythonQuoted:         formatterPreferredQuotedString(request.ProjectName),
		ProjectNameJustShellQuoted:      justInterpolationSafe(shellQuote(request.ProjectName)),
		StarterDescriptionQuoted:        strconv.Quote("Generated Golden Path starter for " + request.ProjectName),
		StarterDescriptionPythonQuoted:  formatterPreferredQuotedString("Generated Golden Path starter for " + request.ProjectName),
		InfrastructureDescriptionQuoted: strconv.Quote(request.ProjectName + " infrastructure"),
		ProjectSlug:                     request.ProjectSlug, PackageName: component.Name,
		GoPackage: safeGoIdentifier(component.Name), PythonModule: safePythonIdentifier(component.Name),
		PythonVersion: bundle.Tools["python"], GoVersion: bundle.Tools["go"],
		GoLanguageVersion: majorMinor(bundle.Tools["go"]), GoImportsVersion: bundle.Tools["goimports"],
		NodeVersion: bundle.Tools["node"], NodeNextMajor: nextMajor(bundle.Tools["node"]),
		RustVersion: bundle.Tools["rust"], ZigVersion: bundle.Tools["zig"], ModulePath: component.ModulePath,
		RecipePrefix: strings.ReplaceAll(component.Name, "-", "_"), ShellPath: component.Path,
		ZigPackageName: zigEnumLiteral(component.Name), ZigFingerprint: zigFingerprint(component.Name),
		StackName:   strings.Join(titleWords(component.Name), ""),
		PNPMVersion: bundle.Tools["pnpm"], PNPMArchiveSHA256: bundle.PNPMArchiveSHA256,
		PNPMExecutableSHA256: bundle.PNPMExecutableSHA256, ESLintJSVersion: bundle.Tools["eslint-js"],
		NodeTypesVersion: bundle.Tools["node-types"], ESLintVersion: bundle.Tools["eslint"],
		PrettierVersion: bundle.Tools["prettier"], TypeScriptVersion: bundle.Tools["typescript"],
		TypeScriptESLintVersion: bundle.Tools["typescript-eslint"], VitestVersion: bundle.Tools["vitest"],
		UVBuildVersion: bundle.Tools["uv-build"], MypyVersion: bundle.Tools["mypy"],
		PytestVersion: bundle.Tools["pytest"], RuffVersion: bundle.Tools["ruff"],
		AWSCDKVersion: bundle.Tools["aws-cdk"], AWSCDKLibVersion: bundle.Tools["aws-cdk-lib"],
		ConstructsVersion: bundle.Tools["constructs"], PulumiSDKVersion: bundle.Tools["pulumi-sdk"],
		AWSCDK: profiles["infrastructure-aws-cdk"], Pulumi: profiles["infrastructure-pulumi"],
		GoExecutable:               profiles["go"] && executableArtifact,
		GoLibrary:                  profiles["go"] && (hasArtifact(component, "library") || hasArtifact(component, "package")),
		NodeExecutable:             profiles["node-typescript"] && applicationExecutable,
		PythonExecutable:           profiles["python"] && applicationExecutable,
		RustExecutable:             profiles["rust"] && executableArtifact,
		RustLibrary:                profiles["rust"] && (hasArtifact(component, "library") || hasArtifact(component, "package")),
		HasNodeRuntimeDependencies: profiles["infrastructure-aws-cdk"] || profiles["infrastructure-pulumi"],
	}
	prefix := func(relative string) string {
		if component.Path == "." {
			return relative
		}
		return path.Join(component.Path, relative)
	}
	var files []File
	add := func(target, source string, mode fs.FileMode) error {
		content, err := renderTemplate(source, model)
		if err != nil {
			return err
		}
		files = append(files, File{Path: prefix(target), Mode: mode, Content: content, Source: source})
		return nil
	}
	var recipeSources []string
	for _, profile := range component.Profiles {
		switch profile {
		case "documentation", "go", "node-typescript", "python", "rust", "zig", "zig-toolchain":
			recipeSources = append(recipeSources, "profiles/"+profile+"/component.just.tmpl")
		case "infrastructure-terraform", "infrastructure-opentofu":
			recipeSources = append(recipeSources, "profiles/infrastructure-hcl/component.just.tmpl")
		case "infrastructure-aws-cdk":
			recipeSources = append(recipeSources, "profiles/infrastructure-aws-cdk/component.just.tmpl")
		}
	}
	var recipe bytes.Buffer
	for _, source := range recipeSources {
		content, err := renderTemplate(source, modelWithEngine(model, component.Profiles, bundle))
		if err != nil {
			return nil, err
		}
		if recipe.Len() != 0 {
			recipe.WriteByte('\n')
		}
		recipe.Write(content)
	}
	files = append(files, File{Path: "just/components/" + strings.ReplaceAll(component.Name, "-", "_") + ".just", Mode: 0o644, Content: recipe.Bytes(), Source: strings.Join(recipeSources, "+")})

	if profiles["go"] {
		for _, item := range []struct{ target, source string }{{"go.mod", "profiles/go/go.mod.tmpl"}, {"go.sum", "profiles/go/go.sum.tmpl"}} {
			if err := add(item.target, item.source, 0o644); err != nil {
				return nil, err
			}
		}
		if model.GoLibrary {
			if err := add("library.go", "profiles/go/library.go.tmpl", 0o644); err != nil {
				return nil, err
			}
			if err := add("library_test.go", "profiles/go/library_test.go.tmpl", 0o644); err != nil {
				return nil, err
			}
		}
		if model.GoExecutable {
			if err := add(path.Join("cmd", component.Name, "main.go"), "profiles/go/main.go.tmpl", 0o644); err != nil {
				return nil, err
			}
			if err := add(path.Join("cmd", component.Name, "main_test.go"), "profiles/go/main_test.go.tmpl", 0o644); err != nil {
				return nil, err
			}
		}
	}
	if profiles["node-typescript"] {
		lockSource := "profiles/node-typescript/pnpm-lock.base.yaml.tmpl"
		if profiles["infrastructure-aws-cdk"] {
			lockSource = "profiles/node-typescript/pnpm-lock.cdk.yaml.tmpl"
		}
		if profiles["infrastructure-pulumi"] {
			lockSource = "profiles/node-typescript/pnpm-lock.pulumi.yaml.tmpl"
		}
		for _, item := range []struct{ target, source string }{
			{"package.json", "profiles/node-typescript/package.json.tmpl"}, {"pnpm-lock.yaml", lockSource},
			{"pnpm-workspace.yaml", "profiles/node-typescript/pnpm-workspace.yaml.tmpl"},
			{"tsconfig.json", "profiles/node-typescript/tsconfig.json.tmpl"}, {"tsconfig.build.json", "profiles/node-typescript/tsconfig.build.json.tmpl"},
			{"eslint.config.js", "profiles/node-typescript/eslint.config.js.tmpl"}, {".prettierignore", "profiles/node-typescript/.prettierignore.tmpl"},
			{".prettierrc.json", "profiles/node-typescript/.prettierrc.json.tmpl"},
			{"scripts/pnpm", "profiles/node-typescript/scripts/pnpm.tmpl"}, {"src/index.ts", "profiles/node-typescript/index.ts.tmpl"},
			{"tests/index.test.ts", "profiles/node-typescript/index.test.ts.tmpl"},
		} {
			mode := fs.FileMode(0o644)
			if item.target == "scripts/pnpm" {
				mode = 0o755
			}
			if err := add(item.target, item.source, mode); err != nil {
				return nil, err
			}
		}
	}
	if profiles["python"] {
		for _, item := range []struct{ target, source string }{
			{".python-version", "profiles/python/.python-version.tmpl"}, {"pyproject.toml", "profiles/python/pyproject.toml.tmpl"},
			{"uv.lock", "profiles/python/uv.lock.tmpl"}, {path.Join("src", model.PythonModule, "__init__.py"), "profiles/python/__init__.py.tmpl"},
			{"tests/test_project.py", "profiles/python/test_project.py.tmpl"},
		} {
			if err := add(item.target, item.source, 0o644); err != nil {
				return nil, err
			}
		}
	}
	if profiles["rust"] {
		for _, item := range []struct{ target, source string }{
			{"Cargo.toml", "profiles/rust/Cargo.toml.tmpl"}, {"Cargo.lock", "profiles/rust/Cargo.lock.tmpl"},
			{"rust-toolchain.toml", "profiles/rust/rust-toolchain.toml.tmpl"}, {"rustfmt.toml", "profiles/rust/rustfmt.toml.tmpl"},
		} {
			if err := add(item.target, item.source, 0o644); err != nil {
				return nil, err
			}
		}
		if model.RustLibrary {
			if err := add("src/lib.rs", "profiles/rust/lib.rs.tmpl", 0o644); err != nil {
				return nil, err
			}
		}
		if model.RustExecutable {
			if err := add("src/main.rs", "profiles/rust/main.rs.tmpl", 0o644); err != nil {
				return nil, err
			}
		}
	}
	if profiles["zig"] {
		for _, item := range []struct{ target, source string }{
			{"build.zig", "profiles/zig/build.zig.tmpl"}, {"build.zig.zon", "profiles/zig/build.zig.zon.tmpl"},
			{"zig-targets.json", "profiles/zig/zig-targets.json.tmpl"}, {"src/main.zig", "profiles/zig/main.zig.tmpl"},
		} {
			if err := add(item.target, item.source, 0o644); err != nil {
				return nil, err
			}
		}
		if err := add("scripts/zig-target", "profiles/zig/scripts/zig-target.tmpl", 0o755); err != nil {
			return nil, err
		}
	}
	if profiles["zig-toolchain"] {
		for _, item := range []struct{ target, source string }{
			{"src/main.c", "profiles/zig-toolchain/main.c.tmpl"},
			{"zig-toolchain.json", "profiles/zig-toolchain/zig-toolchain.json.tmpl"},
		} {
			if err := add(item.target, item.source, 0o644); err != nil {
				return nil, err
			}
		}
		if err := add("scripts/zig-target", "profiles/zig/scripts/zig-target.tmpl", 0o755); err != nil {
			return nil, err
		}
	}
	if profiles["documentation"] && component.Path != "." {
		if err := add("README.md", "profiles/documentation/README.md.tmpl", 0o644); err != nil {
			return nil, err
		}
	}
	if profiles["infrastructure-terraform"] || profiles["infrastructure-opentofu"] {
		model = modelWithEngine(model, component.Profiles, bundle)
		for _, item := range []struct{ target, source string }{{"versions.tf", "profiles/infrastructure-hcl/versions.tf.tmpl"}, {"main.tf", "profiles/infrastructure-hcl/main.tf.tmpl"}, {".terraform.lock.hcl", "profiles/infrastructure-hcl/.terraform.lock.hcl.tmpl"}} {
			content, err := renderTemplate(item.source, model)
			if err != nil {
				return nil, err
			}
			files = append(files, File{Path: prefix(item.target), Mode: 0o644, Content: content, Source: item.source})
		}
	}
	if profiles["infrastructure-aws-cdk"] {
		if err := add("cdk.json", "profiles/infrastructure-aws-cdk/cdk.json.tmpl", 0o644); err != nil {
			return nil, err
		}
	}
	if profiles["infrastructure-pulumi"] {
		if err := add("Pulumi.yaml", "profiles/infrastructure-pulumi/Pulumi.yaml.tmpl", 0o644); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func renderTemplate(name string, data any) ([]byte, error) {
	source, err := bundlefs.Files.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read embedded template %q: %w", name, err)
	}
	if bytes.Contains(source, []byte("[[package]]")) {
		model, ok := data.(componentModel)
		if !ok || !bytes.Contains(source, []byte("__GOLDEN_PATH_PACKAGE_NAME__")) {
			return nil, fmt.Errorf("render lock template %q with an invalid data model", name)
		}
		return bytes.ReplaceAll(source, []byte("__GOLDEN_PATH_PACKAGE_NAME__"), []byte(model.PackageName)), nil
	}
	if !bytes.Contains(source, []byte("[[.")) && !bytes.Contains(source, []byte("[[if")) && !bytes.Contains(source, []byte("[[range")) {
		return source, nil
	}
	parsed, err := template.New(path.Base(name)).Delims("[[", "]]").Option("missingkey=error").Parse(string(source))
	if err != nil {
		return nil, fmt.Errorf("parse embedded template %q: %w", name, err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("render embedded template %q: %w", name, err)
	}
	return output.Bytes(), nil
}

func buildRecipeModel(request Request) recipeModel {
	model := recipeModel{}
	for _, component := range request.Components {
		prefix := "_" + strings.ReplaceAll(component.Name, "-", "_")
		flags := componentCapabilities(component)
		model.Components = append(model.Components, recipeComponent{RecipeFile: strings.ReplaceAll(component.Name, "-", "_") + ".just"})
		if flags["init"] {
			model.InitRecipes = append(model.InitRecipes, prefix+"_init")
		}
		if flags["format"] {
			model.FormatRecipes = append(model.FormatRecipes, prefix+"_format")
			model.HasFormat = true
		}
		if flags["format-check"] {
			model.FormatCheckRecipes = append(model.FormatCheckRecipes, prefix+"_format_check")
			model.HasFormatCheck = true
		}
		if flags["lint"] {
			model.LintRecipes = append(model.LintRecipes, prefix+"_lint")
			model.HasLint = true
		}
		if flags["typecheck"] {
			model.TypecheckRecipes = append(model.TypecheckRecipes, prefix+"_typecheck")
			model.HasTypecheck = true
		}
		if flags["test"] {
			model.TestRecipes = append(model.TestRecipes, prefix+"_test")
			model.HasTest = true
		}
		if flags["build"] {
			model.BuildRecipes = append(model.BuildRecipes, prefix+"_build")
			model.HasBuild = true
		}
		if flags["check"] {
			model.CheckRecipes = append(model.CheckRecipes, prefix+"_check")
		}
		if flags["iac-check"] {
			model.CheckRecipes = append(model.CheckRecipes, prefix+"_iac_check")
		}
		if flags["ci"] {
			model.CIRecipes = append(model.CIRecipes, prefix+"_ci")
		}
	}
	return model
}

func componentCapabilities(component Component) map[string]bool {
	profiles := stringSet(component.Profiles)
	flags := map[string]bool{}
	if profiles["go"] || profiles["node-typescript"] || profiles["python"] || profiles["rust"] {
		flags["init"] = true
		for _, capability := range []string{"format", "format-check", "lint", "typecheck", "test", "build", "ci"} {
			flags[capability] = true
		}
		if profiles["go"] || profiles["python"] {
			flags["check"] = true
		}
	}
	if profiles["zig"] {
		flags["init"] = true
		for _, capability := range []string{"format", "format-check", "typecheck", "test", "build", "ci"} {
			flags[capability] = true
		}
	}
	if profiles["zig-toolchain"] {
		flags["init"] = true
		for _, capability := range []string{"typecheck", "test", "build", "ci"} {
			flags[capability] = true
		}
	}
	if profiles["documentation"] {
		flags["format-check"] = true
		flags["test"] = true
	}
	if profiles["infrastructure-terraform"] || profiles["infrastructure-opentofu"] {
		flags["init"] = true
		flags["format"] = true
		flags["format-check"] = true
		flags["typecheck"] = true
	}
	if profiles["infrastructure-aws-cdk"] {
		flags["iac-check"] = true
	}
	return flags
}

func aggregateMetadata(request Request, mode string) ([]string, []string, []string) {
	profileSet, artifactSet, capabilitySet := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, component := range request.Components {
		for _, profile := range component.Profiles {
			profileSet[profile] = true
		}
		for _, artifact := range component.ArtifactTypes {
			artifactSet[artifact] = true
		}
		for _, capability := range metadataCapabilities(component, mode) {
			capabilitySet[capability] = true
		}
	}
	return sortedKeys(profileSet), sortedKeys(artifactSet), sortedKeys(capabilitySet)
}

func metadataCapabilities(component Component, mode string) []string {
	if mode == "adoption" {
		return cloneStrings(component.Capabilities)
	}
	flags := componentCapabilities(component)
	capabilities := map[string]bool{"dependency-automation": true, "cache": true}
	for _, capability := range []string{"format", "lint", "typecheck", "test", "build"} {
		if flags[capability] || capability == "format" && flags["format-check"] {
			capabilities[capability] = true
		}
	}
	for _, capability := range component.Capabilities {
		capabilities[capability] = true
	}
	return sortedKeys(capabilities)
}

func buildAutomationModel(release ReleaseManifest, profiles []string) (automationModel, error) {
	profileData, _ := json.Marshal(profiles)
	bundle, err := LoadBundle()
	if err != nil {
		return automationModel{}, err
	}
	model := automationModel{
		AutomationSourceCommit: release.Source.Commit, CheckerVersion: release.ReleaseVersion,
		GitHubCLIVersion: bundle.Tools["github-cli"], ProfilesJSON: string(profileData),
	}
	for _, asset := range release.Assets {
		switch asset.OS + "/" + asset.Architecture {
		case "darwin/amd64":
			model.DarwinAMD64SHA256 = asset.SHA256
			model.DarwinAMD64ExecutableSHA256 = asset.ExecutableSHA256
		case "darwin/arm64":
			model.DarwinARM64SHA256 = asset.SHA256
			model.DarwinARM64ExecutableSHA256 = asset.ExecutableSHA256
		case "linux/amd64":
			model.LinuxAMD64SHA256 = asset.SHA256
			model.LinuxAMD64ExecutableSHA256 = asset.ExecutableSHA256
		case "linux/arm64":
			model.LinuxARM64SHA256 = asset.SHA256
			model.LinuxARM64ExecutableSHA256 = asset.ExecutableSHA256
		}
	}
	return model, nil
}

func buildDependabotModel(request Request) dependabotModel {
	seen := map[string]bool{}
	var result dependabotModel
	for _, component := range request.Components {
		profiles := stringSet(component.Profiles)
		directory := "/"
		if component.Path != "." {
			directory += component.Path
		}
		for _, ecosystem := range []string{"cargo", "gomod", "npm", "terraform", "uv"} {
			selected := ecosystem == "cargo" && profiles["rust"] || ecosystem == "gomod" && profiles["go"] || ecosystem == "npm" && profiles["node-typescript"] || ecosystem == "uv" && profiles["python"] || ecosystem == "terraform" && (profiles["infrastructure-terraform"] || profiles["infrastructure-opentofu"])
			key := ecosystem + "\x00" + directory
			if selected && !seen[key] {
				result.Dependabot = append(result.Dependabot, dependabotEntry{Ecosystem: ecosystem, Directory: directory})
				seen[key] = true
			}
		}
	}
	sort.Slice(result.Dependabot, func(left, right int) bool {
		if result.Dependabot[left].Ecosystem == result.Dependabot[right].Ecosystem {
			return result.Dependabot[left].Directory < result.Dependabot[right].Directory
		}
		return result.Dependabot[left].Ecosystem < result.Dependabot[right].Ecosystem
	})
	return result
}

func selectedMiseTools(profiles []string) []string {
	selected := map[string]bool{"actionlint": true, "github-cli": true, "just": true}
	set := stringSet(profiles)
	if set["go"] {
		selected["go"] = true
		selected["golangci-lint"] = true
	}
	if set["node-typescript"] || set["infrastructure-aws-cdk"] {
		selected["node"] = true
	}
	if set["python"] {
		selected["uv"] = true
	}
	if set["zig"] || set["zig-toolchain"] {
		selected["zig"] = true
	}
	if set["infrastructure-terraform"] {
		selected["terraform"] = true
	}
	if set["infrastructure-opentofu"] {
		selected["opentofu"] = true
	}
	if set["infrastructure-pulumi"] {
		selected["pulumi"] = true
		selected["node"] = true
	}
	return sortedKeys(selected)
}

func renderMiseTOML(bundle Bundle, tools []string) ([]byte, error) {
	var output strings.Builder
	fmt.Fprintf(&output, "min_version = %q\n\n[tools]\n", bundle.MiseMinimumVersion)
	for _, tool := range tools {
		version := bundle.Tools[tool]
		if version == "" {
			return nil, fmt.Errorf("template bundle does not pin selected mise tool %q", tool)
		}
		fmt.Fprintf(&output, "%s = %q\n", tool, version)
	}
	output.WriteString("\n[settings]\nlockfile = true\n")
	return []byte(output.String()), nil
}

func filterMiseLock(tools []string) ([]byte, error) {
	data, err := bundlefs.Files.ReadFile("tooling/mise.lock")
	if err != nil {
		return nil, fmt.Errorf("read embedded mise lock catalog: %w", err)
	}
	selected := stringSet(tools)
	found := make(map[string]bool, len(tools))
	lines := strings.Split(string(data), "\n")
	var output strings.Builder
	include := true
	for _, line := range lines {
		if strings.HasPrefix(line, "[[tools.") && strings.HasSuffix(line, "]]") {
			name := strings.TrimSuffix(strings.TrimPrefix(line, "[[tools."), "]]")
			include = selected[name]
			if include {
				found[name] = true
			}
		}
		if include {
			output.WriteString(line)
			output.WriteByte('\n')
		}
	}
	for _, tool := range tools {
		if !found[tool] {
			return nil, fmt.Errorf("embedded mise lock catalog does not contain selected tool %q", tool)
		}
	}
	return []byte(strings.TrimRight(output.String(), "\n") + "\n"), nil
}

func modelWithEngine(model componentModel, profiles []string, bundle Bundle) componentModel {
	set := stringSet(profiles)
	if set["infrastructure-terraform"] {
		model.Engine = "terraform"
		model.EngineVersion = bundle.Tools["terraform"]
	}
	if set["infrastructure-opentofu"] {
		model.Engine = "tofu"
		model.EngineVersion = bundle.Tools["opentofu"]
	}
	return model
}

func validateRenderedFiles(files []File) error {
	if len(files) == 0 {
		return fmt.Errorf("generated file set is empty")
	}
	seen := map[string]bool{}
	for _, file := range files {
		cleaned, err := cleanRelativePath(file.Path)
		if err != nil || cleaned == "." || seen[cleaned] {
			return fmt.Errorf("generated an invalid or duplicate path %q", file.Path)
		}
		if file.Mode != 0o644 && file.Mode != 0o755 {
			return fmt.Errorf("generated unsupported mode for %q", file.Path)
		}
		if len(file.Content) == 0 {
			return fmt.Errorf("generated empty file %q", file.Path)
		}
		if file.Source == "" {
			return fmt.Errorf("generated file %q has no source identity", file.Path)
		}
		seen[cleaned] = true
	}
	return nil
}

func cloneRequest(request Request) Request {
	request.Targets = cloneTargets(request.Targets)
	request.Components = append([]Component(nil), request.Components...)
	for index := range request.Components {
		request.Components[index].Profiles = cloneStrings(request.Components[index].Profiles)
		request.Components[index].ArtifactTypes = cloneStrings(request.Components[index].ArtifactTypes)
		request.Components[index].Capabilities = cloneStrings(request.Components[index].Capabilities)
	}
	return request
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append(make([]string, 0, len(values)), values...)
}

func hasArtifact(component Component, artifact string) bool {
	return stringSet(component.ArtifactTypes)[artifact]
}

func hasExecutableArtifact(component Component) bool {
	artifacts := stringSet(component.ArtifactTypes)
	for _, artifact := range []string{"application", "service", "cli", "binary", "container", "tooling"} {
		if artifacts[artifact] {
			return true
		}
	}
	return false
}

func hasSourceArtifact(component Component) bool {
	return hasExecutableArtifact(component) || hasArtifact(component, "library") || hasArtifact(component, "package")
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key, enabled := range values {
		if enabled {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func indentJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode generated JSON: %w", err)
	}
	return append(data, '\n'), nil
}

func zigFingerprint(name string) string {
	sum := sha256.Sum256([]byte("5010-dev/golden-path/zig/" + name))
	id := binary.LittleEndian.Uint32(sum[:4])
	if id == 0 || id == ^uint32(0) {
		id = 1
	}
	fingerprint := uint64(crc32.ChecksumIEEE([]byte(strings.ReplaceAll(name, "-", "_"))))<<32 | uint64(id)
	return fmt.Sprintf("0x%016x", fingerprint)
}

func titleWords(value string) []string {
	words := strings.Split(value, "-")
	for index := range words {
		if words[index] != "" {
			words[index] = strings.ToUpper(words[index][:1]) + words[index][1:]
		}
	}
	return words
}

func majorMinor(version string) string {
	parts := strings.Split(version, ".")
	return strings.Join(parts[:2], ".")
}

func nextMajor(version string) string {
	major, _ := strconv.Atoi(strings.Split(version, ".")[0])
	return strconv.Itoa(major + 1)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func formatterPreferredQuotedString(value string) string {
	delimiter := byte('"')
	if strings.Count(value, "'") < strings.Count(value, `"`) {
		delimiter = '\''
	}
	var output strings.Builder
	output.WriteByte(delimiter)
	for _, character := range value {
		switch character {
		case '\\':
			output.WriteString(`\\`)
		case rune(delimiter):
			output.WriteByte('\\')
			output.WriteRune(character)
		default:
			output.WriteRune(character)
		}
	}
	output.WriteByte(delimiter)
	return output.String()
}

func justInterpolationSafe(value string) string {
	return strings.NewReplacer("{{", `{{"{{"}}`, "}}", `{{"}}"}}`).Replace(value)
}

func safeGoIdentifier(value string) string {
	identifier := strings.ReplaceAll(value, "-", "_")
	if goKeywords[identifier] {
		return identifier + "_pkg"
	}
	return identifier
}

func safePythonIdentifier(value string) string {
	identifier := strings.ReplaceAll(value, "-", "_")
	if pythonKeywords[identifier] {
		return identifier + "_pkg"
	}
	return identifier
}

func zigEnumLiteral(value string) string {
	identifier := strings.ReplaceAll(value, "-", "_")
	if zigKeywords[identifier] {
		return `.@"` + identifier + `"`
	}
	return "." + identifier
}

var goKeywords = stringSet(strings.Fields("break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var"))
var pythonKeywords = stringSet(strings.Fields("False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield match case type"))
var zigKeywords = stringSet(strings.Fields("addrspace align allowzero and anyframe anytype asm async await break callconv catch comptime const continue defer else enum errdefer error export extern fn for if inline linksection noalias noinline nosuspend opaque or orelse packed pub resume return struct suspend switch test threadlocal try union unreachable usingnamespace var volatile while"))
