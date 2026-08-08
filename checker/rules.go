package checker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/5010-dev/engineering-tooling/dependency"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

type ruleEvaluator func(root string, metadata Metadata, rule Rule, exceptionsPresent bool) Finding

var evaluators = map[string]ruleEvaluator{
	"DT-META-001":    evaluateMetadata,
	"DT-CMD-001":     evaluateCommands,
	"DT-TOOL-002":    evaluateExactSelectors,
	"DT-TOOL-003":    evaluateMiseLock,
	"DT-DEP-001":     evaluateDependencyRecords,
	"DT-DEP-004":     evaluateDirectDependencies,
	"DT-ASSET-002":   evaluateImmutableActions,
	"DT-ASSET-003":   evaluateAssetVersion,
	"DT-CONF-001":    evaluateCheckerBoundary,
	"DT-EXC-001":     evaluateExceptionFile,
	"DT-ASSET-007":   evaluateWorkflowTemplates,
	"DT-NODE-001":    evaluateNodeProfile,
	"DT-PY-001":      evaluatePythonProfile,
	"DT-GO-001":      evaluateGoAuthority,
	"DT-GO-002":      evaluateGoDependencies,
	"DT-RUST-001":    evaluateRustProfile,
	"DT-RELEASE-001": evaluateReleaseVersion,
	"DT-DEP-005":     evaluateDependencyPolicy,
	"DT-DEP-006":     evaluateDependencyPolicy,
	"DT-DEP-007":     evaluateDependencyPolicy,
	"DT-DEP-010":     evaluateDependencyPolicy,
	"DT-DEP-011":     evaluateDependencyPolicy,
}

var componentScopedRules = map[string]bool{
	"DT-TOOL-002": true, "DT-DEP-001": true, "DT-DEP-004": true, "DT-RUNTIME-001": true,
	"DT-NODE-001": true, "DT-PY-001": true, "DT-GO-001": true, "DT-GO-002": true,
	"DT-RUST-001": true, "DT-ZIG-001": true,
}

func evaluateRule(
	root string,
	metadata Metadata,
	rule Rule,
	exceptions []Exception,
	exceptionsPresent bool,
	evaluatedAt time.Time,
	compatibility CompatibilityManifest,
) Finding {
	if rule.RetiredIn != nil {
		return skippedFinding(rule, skipCategoryOther, ".", "Rule is retired in this standard snapshot.")
	}
	if rule.ID != "DT-META-001" && metadata.Applicability.Status == "not-applicable" {
		return skippedFinding(rule, skipCategoryNotApplicable, ".", "Repository declares Golden Path as not applicable.")
	}
	if !ruleApplies(metadata, rule) {
		return skippedFinding(rule, skipCategoryNotApplicable, ".", "Rule does not apply to the declared profiles, artifact types, or capabilities.")
	}
	if rule.Assessment != "automated" {
		return baseFinding(rule, "skip", ".", "Rule requires hybrid or manual evidence and was not asserted by the structural checker.")
	}
	if componentScopedRules[rule.ID] && len(metadata.NativeRoots) > 0 {
		return evaluateNativeRootRule(root, metadata, rule, exceptions, exceptionsPresent, evaluatedAt, compatibility)
	}
	if componentScopedRules[rule.ID] && len(metadata.Components) > 0 {
		return evaluateComponentRule(root, metadata, rule, exceptions, exceptionsPresent, evaluatedAt, compatibility)
	}
	return evaluateAutomatedRule(root, metadata, rule, exceptionsPresent, evaluatedAt, compatibility)
}

func evaluateNativeRootRule(
	root string,
	metadata Metadata,
	rule Rule,
	exceptions []Exception,
	exceptionsPresent bool,
	evaluatedAt time.Time,
	compatibility CompatibilityManifest,
) Finding {
	var selected *Finding
	selectedWaivable := false
	for _, nativeRoot := range metadata.NativeRoots {
		for _, profile := range nativeRoot.Profiles {
			rootMetadata := Metadata{
				SchemaVersion: metadata.SchemaVersion, ContractVersion: metadata.ContractVersion,
				StandardVersion: metadata.StandardVersion, AssetBundleVersion: metadata.AssetBundleVersion,
				Profiles: []string{profile}, Targets: metadata.Targets,
				ComponentPath: nativeRoot.Path, NativeRootID: nativeRoot.ID,
			}
			if !ruleApplies(rootMetadata, rule) {
				continue
			}
			finding := evaluateAutomatedRule(root, rootMetadata, rule, exceptionsPresent, evaluatedAt, compatibility)
			if finding.Extensions == nil {
				finding.Extensions = map[string]any{}
			}
			finding.Extensions["nativeRootId"] = nativeRoot.ID
			finding.Extensions["nativeRootPath"] = nativeRoot.Path
			finding.Extensions["nativeRootProfile"] = profile
			waivable := false
			if finding.Status == "fail" {
				if exception, expired := matchingException(exceptions, rootMetadata, finding, rule, evaluatedAt); exception != nil && !expired {
					waivable = true
				}
			}
			priority := componentFindingPriority(finding.Status)
			selectedPriority := 0
			if selected != nil {
				selectedPriority = componentFindingPriority(selected.Status)
			}
			if selected == nil || priority > selectedPriority || priority == selectedPriority && selectedWaivable && !waivable {
				copyOfFinding := finding
				selected = &copyOfFinding
				selectedWaivable = waivable
			}
		}
	}
	if selected == nil {
		return skippedFinding(rule, skipCategoryNotApplicable, ".", "Rule does not apply to any declared native root.")
	}
	return *selected
}

func evaluateAutomatedRule(
	root string,
	metadata Metadata,
	rule Rule,
	exceptionsPresent bool,
	evaluatedAt time.Time,
	compatibility CompatibilityManifest,
) Finding {
	if rule.ID == "DT-RUNTIME-001" {
		return evaluateRuntimeDisposition(root, metadata, rule, evaluatedAt, compatibility)
	}
	if rule.ID == "DT-ZIG-001" {
		return evaluateZigProfile(root, metadata, rule, compatibility)
	}
	evaluator, exists := evaluators[rule.ID]
	if !exists {
		return baseFinding(rule, "error", ".", "The checker release has no evaluator for an applicable automated rule.")
	}
	return evaluator(root, metadata, rule, exceptionsPresent)
}

func evaluateDependencyPolicy(root string, _ Metadata, rule Rule, _ bool) Finding {
	evaluation := dependency.Evaluate(root)
	for _, candidate := range evaluation.Findings {
		if candidate.RuleID != rule.ID {
			continue
		}
		finding := baseFinding(rule, candidate.Status, candidate.Path, candidate.Message)
		finding.Secondary = candidate.Secondary
		finding.Remediation = candidate.Remediation
		if candidate.Status == "error" {
			finding.Extensions = map[string]any{}
			if evaluation.ExitCode == 2 {
				finding.Extensions["errorKind"] = "configuration"
			}
		}
		return finding
	}
	if evaluation.ExitCode == 2 || evaluation.ExitCode == 3 {
		finding := baseFinding(rule, "error", ".github/golden-path-dependency-policy.yaml", "Dependency policy evaluation did not complete.")
		if evaluation.ExitCode == 2 {
			finding.Extensions = map[string]any{"errorKind": "configuration"}
		}
		return finding
	}
	return baseFinding(rule, "pass", ".github/golden-path-dependency-policy.yaml", "Dependency policy satisfies this structural rule.")
}

func evaluateComponentRule(
	root string,
	metadata Metadata,
	rule Rule,
	exceptions []Exception,
	exceptionsPresent bool,
	evaluatedAt time.Time,
	compatibility CompatibilityManifest,
) Finding {
	var selected *Finding
	selectedWaivable := false
	for _, component := range metadata.Components {
		componentMetadata := Metadata{
			SchemaVersion: metadata.SchemaVersion, ContractVersion: metadata.ContractVersion,
			StandardVersion: metadata.StandardVersion, AssetBundleVersion: metadata.AssetBundleVersion,
			Profiles: component.Profiles, ArtifactTypes: component.ArtifactTypes,
			Capabilities: component.Capabilities, Targets: metadata.Targets, ComponentPath: component.Path,
		}
		if !ruleApplies(componentMetadata, rule) {
			continue
		}
		finding := evaluateAutomatedRule(root, componentMetadata, rule, exceptionsPresent, evaluatedAt, compatibility)
		if finding.Extensions == nil {
			finding.Extensions = map[string]any{}
		}
		finding.Extensions["componentPath"] = component.Path
		waivable := false
		if finding.Status == "fail" {
			if exception, expired := matchingException(exceptions, componentMetadata, finding, rule, evaluatedAt); exception != nil && !expired {
				waivable = true
			}
		}
		priority := componentFindingPriority(finding.Status)
		selectedPriority := 0
		if selected != nil {
			selectedPriority = componentFindingPriority(selected.Status)
		}
		if selected == nil || priority > selectedPriority || priority == selectedPriority && selectedWaivable && !waivable {
			copyOfFinding := finding
			selected = &copyOfFinding
			selectedWaivable = waivable
		}
	}
	if selected == nil {
		return skippedFinding(rule, skipCategoryNotApplicable, ".", "Rule does not apply to any declared component.")
	}
	return *selected
}

func componentFindingPriority(status string) int {
	switch status {
	case "error":
		return 5
	case "fail":
		return 4
	case "warn":
		return 3
	case "pass":
		return 2
	default:
		return 1
	}
}

type nativeProfileMarker struct {
	paths    []string
	profiles []string
}

func validateProfileDeclarations(root string, metadata Metadata) (string, string) {
	if len(metadata.NativeRoots) > 0 {
		return validateNativeRootProfileDeclarations(root, metadata.NativeRoots, metadata.Components)
	}
	if len(metadata.Components) == 0 {
		return validateProfileRoot(root, metadata)
	}
	rootMetadata := Metadata{ComponentPath: "."}
	for _, component := range metadata.Components {
		if component.Path == "." {
			rootMetadata.Profiles = component.Profiles
		}
	}
	if findingPath, message := validateProfileRoot(root, rootMetadata); message != "" {
		return findingPath, message
	}
	for _, component := range metadata.Components {
		componentMetadata := Metadata{Profiles: component.Profiles, ComponentPath: component.Path}
		if findingPath, message := validateProfileRoot(root, componentMetadata); message != "" {
			return findingPath, message
		}
	}
	return "", ""
}

func validateNativeRootProfileDeclarations(root string, nativeRoots []MetadataNativeRoot, components []MetadataComponent) (string, string) {
	if findingPath, message := validateNativeDependencyRootMarkers(root, nativeRoots); message != "" {
		return findingPath, message
	}
	return validateArtifactComponentMarkers(root, nativeRoots, components)
}

func validateNativeDependencyRootMarkers(root string, nativeRoots []MetadataNativeRoot) (string, string) {
	profilesByPath := map[string]map[string]bool{".": {}}
	for _, nativeRoot := range nativeRoots {
		profiles, exists := profilesByPath[nativeRoot.Path]
		if !exists {
			profiles = map[string]bool{}
			profilesByPath[nativeRoot.Path] = profiles
		}
		for _, profile := range nativeRoot.Profiles {
			profiles[profile] = true
		}
	}
	paths := make([]string, 0, len(profilesByPath))
	for rootPath := range profilesByPath {
		paths = append(paths, rootPath)
	}
	slices.Sort(paths)
	for _, rootPath := range paths {
		rootMetadata := Metadata{
			Profiles:      sortedEnabledKeys(profilesByPath[rootPath]),
			ComponentPath: rootPath,
		}
		if findingPath, message := validateProfileRoot(root, rootMetadata); message != "" {
			return findingPath, message
		}
	}
	return "", ""
}

func validateArtifactComponentMarkers(root string, nativeRoots []MetadataNativeRoot, components []MetadataComponent) (string, string) {
	for _, component := range components {
		componentMetadata := Metadata{Profiles: component.Profiles, ComponentPath: component.Path}
		if findingPath, message := validateProfileRoot(root, componentMetadata); message != "" {
			return findingPath, message
		}
		for _, marker := range nativeProfileMarkers() {
			name, _, err := firstExistingComponent(root, componentMetadata, marker.paths...)
			if errors.Is(err, errNotFound) {
				continue
			}
			if err != nil {
				return componentFile(componentMetadata, marker.paths[0]), "A native profile marker could not be read safely."
			}
			requiredProfiles := make([]string, 0, len(marker.profiles))
			for _, profile := range marker.profiles {
				if slices.Contains(component.Profiles, profile) {
					requiredProfiles = append(requiredProfiles, profile)
				}
			}
			covered := false
			for _, nativeRoot := range nativeRoots {
				if pathContains(nativeRoot.Path, component.Path) && intersects(nativeRoot.Profiles, requiredProfiles) {
					covered = true
					break
				}
			}
			if !covered {
				return name, "Native marker " + name + " is not covered by a declared native dependency root for its profile."
			}
		}
	}
	return "", ""
}

func validateProfileRoot(root string, metadata Metadata) (string, string) {
	for _, marker := range nativeProfileMarkers() {
		name, _, err := firstExistingComponent(root, metadata, marker.paths...)
		if errors.Is(err, errNotFound) {
			continue
		}
		if err != nil {
			return componentFile(metadata, marker.paths[0]), "A native profile marker could not be read safely."
		}
		if !intersects(marker.profiles, metadata.Profiles) {
			return name, "Native marker " + name + " requires one of the declared profiles: " + strings.Join(marker.profiles, ", ") + "."
		}
	}
	return "", ""
}

func nativeProfileMarkers() []nativeProfileMarker {
	return []nativeProfileMarker{
		{paths: []string{"package.json"}, profiles: []string{"node-typescript"}},
		{paths: []string{"pyproject.toml"}, profiles: []string{"python"}},
		{paths: []string{"go.mod", "go.work"}, profiles: []string{"go"}},
		{paths: []string{"Cargo.toml"}, profiles: []string{"rust"}},
		{paths: []string{"build.zig", "build.zig.zon"}, profiles: []string{"zig", "zig-toolchain"}},
	}
}

func pathContains(rootPath, candidatePath string) bool {
	return rootPath == "." || candidatePath == rootPath || strings.HasPrefix(candidatePath, rootPath+"/")
}

func componentFile(metadata Metadata, name string) string {
	if metadata.ComponentPath == "" || metadata.ComponentPath == "." {
		return name
	}
	return path.Join(metadata.ComponentPath, name)
}

func firstExistingComponent(root string, metadata Metadata, names ...string) (string, []byte, error) {
	for _, name := range names {
		componentName := componentFile(metadata, name)
		data, err := readRepositoryFile(root, componentName)
		if errors.Is(err, errNotFound) {
			continue
		}
		return componentName, data, err
	}
	return "", nil, errNotFound
}

type goModuleRecord struct {
	path string
	data []byte
}

type goWorkspaceRecord struct {
	authorityPath string
	authorityData []byte
	modules       []goModuleRecord
	runtimeFiles  []goModuleRecord
}

func loadGoWorkspace(root string, metadata Metadata, rule Rule) (goWorkspaceRecord, *Finding) {
	workPath := componentFile(metadata, "go.work")
	workData, workErr := readRepositoryFile(root, workPath)
	if workErr == nil {
		workFile, parseErr := modfile.ParseWork(workPath, workData, nil)
		if parseErr != nil {
			finding := inputError(rule, workPath)
			return goWorkspaceRecord{}, &finding
		}
		if len(workFile.Use) == 0 {
			finding := baseFinding(rule, deviationStatus(rule), workPath, "Go workspace must declare at least one repository-local module.")
			return goWorkspaceRecord{}, &finding
		}
		modules := make([]goModuleRecord, 0, len(workFile.Use))
		seen := make(map[string]bool, len(workFile.Use))
		for _, use := range workFile.Use {
			cleanPath := path.Clean(use.Path)
			if path.IsAbs(use.Path) || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") || strings.Contains(use.Path, "\\") {
				finding := baseFinding(rule, deviationStatus(rule), workPath, "Go workspace use paths must remain within the declared native root.")
				finding.Secondary = use.Path
				return goWorkspaceRecord{}, &finding
			}
			modulePath := componentFile(metadata, path.Join(cleanPath, "go.mod"))
			if seen[modulePath] {
				continue
			}
			moduleData, moduleErr := readRepositoryFile(root, modulePath)
			if moduleErr != nil {
				finding := missingOrInput(rule, modulePath, moduleErr, "Go workspace references a module without go.mod.")
				return goWorkspaceRecord{}, &finding
			}
			seen[modulePath] = true
			modules = append(modules, goModuleRecord{path: modulePath, data: moduleData})
		}
		slices.SortFunc(modules, func(left, right goModuleRecord) int {
			return strings.Compare(left.path, right.path)
		})
		var runtimeFiles []goModuleRecord
		rootModulePath := componentFile(metadata, "go.mod")
		if !seen[rootModulePath] {
			rootModuleData, rootModuleErr := readRepositoryFile(root, rootModulePath)
			switch {
			case rootModuleErr == nil:
				runtimeFiles = append(runtimeFiles, goModuleRecord{path: rootModulePath, data: rootModuleData})
			case !errors.Is(rootModuleErr, errNotFound):
				finding := inputError(rule, rootModulePath)
				return goWorkspaceRecord{}, &finding
			}
		}
		return goWorkspaceRecord{
			authorityPath: workPath,
			authorityData: workData,
			modules:       modules,
			runtimeFiles:  runtimeFiles,
		}, nil
	}
	if !errors.Is(workErr, errNotFound) {
		finding := inputError(rule, workPath)
		return goWorkspaceRecord{}, &finding
	}

	modulePath := componentFile(metadata, "go.mod")
	moduleData, moduleErr := readRepositoryFile(root, modulePath)
	if moduleErr != nil {
		finding := missingOrInput(rule, modulePath, moduleErr, "Go profile requires go.mod or go.work.")
		return goWorkspaceRecord{}, &finding
	}
	return goWorkspaceRecord{
		authorityPath: modulePath,
		authorityData: moduleData,
		modules:       []goModuleRecord{{path: modulePath, data: moduleData}},
	}, nil
}

func ruleApplies(metadata Metadata, rule Rule) bool {
	profileMatch := slices.Contains(rule.Applicability.Profiles, "*") ||
		len(rule.Applicability.Profiles) == 0 ||
		intersects(rule.Applicability.Profiles, metadata.Profiles)
	artifactMatch := len(rule.Applicability.ArtifactTypes) == 0 ||
		intersects(rule.Applicability.ArtifactTypes, metadata.ArtifactTypes)
	capabilityMatch := len(rule.Applicability.Capabilities) == 0 ||
		intersects(rule.Applicability.Capabilities, metadata.Capabilities)
	return profileMatch && artifactMatch && capabilityMatch
}

func evaluateMetadata(_ string, metadata Metadata, rule Rule, _ bool) Finding {
	if metadata.Applicability.Status == "not-applicable" {
		return baseFinding(rule, "pass", ".github/golden-path.yaml", "Schema-valid metadata records a bounded not-applicable declaration.")
	}
	return baseFinding(rule, "pass", ".github/golden-path.yaml", "Schema-valid repository metadata declares exact standard, contract, and asset bundle versions.")
}

func evaluateCommands(root string, _ Metadata, rule Rule, _ bool) Finding {
	name, data, err := firstExisting(root, "justfile", "Justfile", ".justfile")
	if errors.Is(err, errNotFound) {
		return baseFinding(rule, deviationStatus(rule), "justfile", "A supported root Just file is missing.")
	}
	if err != nil {
		return inputError(rule, "justfile")
	}
	recipes, err := collectJustRecipes(root, name, data, &justTraversal{seen: map[string]bool{}}, 0)
	if err != nil {
		return inputError(rule, name)
	}
	var missing []string
	for _, required := range []string{"init", "check", "ci"} {
		if !recipes[required] {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		return baseFinding(rule, deviationStatus(rule), name, "The root Just façade is missing required recipes: "+strings.Join(missing, ", ")+".")
	}
	return baseFinding(rule, "pass", name, "The root Just façade exposes init, check, and ci.")
}

func parseJustRecipes(value string) map[string]bool {
	result := map[string]bool{}
	for _, line := range strings.Split(value, "\n") {
		if name, ok := justRecipeName(line); ok {
			result[name] = true
		}
	}
	return result
}

func justRecipeName(line string) (string, bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return "", false
	}
	if line[0] == '@' {
		line = line[1:]
	}
	end := 0
	for end < len(line) && justIdentifierCharacter(line[end], end == 0) {
		end++
	}
	if end == 0 {
		return "", false
	}
	name := line[:end]
	var quote byte
	escaped := false
	var round, square, curly int
	for index := end; index < len(line); index++ {
		character := line[index]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '#':
			return "", false
		case '(':
			round++
		case ')':
			if round > 0 {
				round--
			}
		case '[':
			square++
		case ']':
			if square > 0 {
				square--
			}
		case '{':
			curly++
		case '}':
			if curly > 0 {
				curly--
			}
		case ':':
			if round == 0 && square == 0 && curly == 0 {
				if index+1 < len(line) && line[index+1] == '=' {
					return "", false
				}
				return name, true
			}
		}
	}
	return "", false
}

func justIdentifierCharacter(character byte, first bool) bool {
	if character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' {
		return true
	}
	return !first && (character == '-' || character >= '0' && character <= '9')
}

type justImport struct {
	optional bool
	path     string
}

func parseJustImports(value string) []justImport {
	var result []justImport
	for _, line := range strings.Split(value, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		optional := false
		remainder := ""
		switch {
		case strings.HasPrefix(line, "import? ") || strings.HasPrefix(line, "import?\t"):
			optional = true
			remainder = strings.TrimSpace(line[len("import?"):])
		case strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "import\t"):
			remainder = strings.TrimSpace(line[len("import"):])
		default:
			continue
		}
		if len(remainder) < 2 || remainder[0] != '\'' && remainder[0] != '"' {
			continue
		}
		quote := remainder[0]
		end := 1
		escaped := false
		for ; end < len(remainder); end++ {
			character := remainder[end]
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if character == quote {
				break
			}
		}
		if end == len(remainder) {
			continue
		}
		trailing := strings.TrimSpace(remainder[end+1:])
		if trailing != "" && !strings.HasPrefix(trailing, "#") {
			continue
		}
		importPath := remainder[1:end]
		if quote == '"' {
			decoded, err := strconv.Unquote(remainder[:end+1])
			if err != nil {
				continue
			}
			importPath = decoded
		}
		result = append(result, justImport{optional: optional, path: importPath})
	}
	return result
}

const (
	maxJustImportDepth = 32
	maxJustImportFiles = 256
	maxJustImportBytes = 16 << 20
)

type justTraversal struct {
	seen  map[string]bool
	files int
	bytes int
}

func collectJustRecipes(root, name string, data []byte, state *justTraversal, depth int) (map[string]bool, error) {
	if depth > maxJustImportDepth {
		return nil, fmt.Errorf("just import depth exceeds limit")
	}
	name = path.Clean(filepath.ToSlash(name))
	if name == ".." || strings.HasPrefix(name, "../") {
		return nil, fmt.Errorf("just import escapes repository root")
	}
	if state.seen[name] {
		return map[string]bool{}, nil
	}
	state.seen[name] = true
	state.files++
	state.bytes += len(data)
	if state.files > maxJustImportFiles {
		return nil, fmt.Errorf("just import file count exceeds limit")
	}
	if state.bytes > maxJustImportBytes {
		return nil, fmt.Errorf("just import bytes exceed limit")
	}
	result := parseJustRecipes(string(data))
	for _, importedFile := range parseJustImports(string(data)) {
		importName := path.Clean(path.Join(path.Dir(name), importedFile.path))
		imported, err := readRepositoryFile(root, importName)
		if errors.Is(err, errNotFound) && importedFile.optional {
			continue
		}
		if err != nil {
			return nil, err
		}
		importedRecipes, err := collectJustRecipes(root, importName, imported, state, depth+1)
		if err != nil {
			return nil, err
		}
		for recipe := range importedRecipes {
			result[recipe] = true
		}
	}
	return result, nil
}

func evaluateExactSelectors(root string, metadata Metadata, rule Rule, _ bool) Finding {
	var selectors []string
	if data, err := readRepositoryFile(root, "mise.toml"); err == nil {
		config, parseErr := parseMiseConfig(data)
		if parseErr != nil {
			return inputError(rule, "mise.toml")
		}
		if len(config.Tools) > 0 {
			lockData, lockErr := readRepositoryFile(root, "mise.lock")
			if errors.Is(lockErr, errNotFound) {
				return baseFinding(rule, deviationStatus(rule), "mise.lock", "Mise tool selectors cannot resolve exact versions because mise.lock is missing.")
			}
			if lockErr != nil {
				return inputError(rule, "mise.lock")
			}
			lock, lockErr := parseMiseLock(lockData)
			if lockErr != nil {
				return inputError(rule, "mise.lock")
			}
			names := sortedMapKeys(config.Tools)
			for _, name := range names {
				value := config.Tools[name]
				selectors = append(selectors, name+"="+value)
				if _, resolveErr := resolveMiseToolVersion(config, lock.Tools, name); resolveErr != nil {
					return baseFinding(rule, deviationStatus(rule), "mise.toml", "Mise tool "+name+" does not resolve through mise.lock to one exact version.")
				}
			}
		}
	} else if !errors.Is(err, errNotFound) {
		return inputError(rule, "mise.toml")
	}
	if slices.Contains(metadata.Profiles, "python") {
		name := componentFile(metadata, ".python-version")
		data, err := readRepositoryFile(root, name)
		if err == nil {
			value := strings.TrimSpace(string(data))
			selectors = append(selectors, "python="+value)
			if !exactPatchVersion(value) {
				return baseFinding(rule, deviationStatus(rule), name, "Python is not pinned to an exact patch release.")
			}
		} else if !errors.Is(err, errNotFound) {
			return inputError(rule, name)
		}
	}
	if slices.Contains(metadata.Profiles, "rust") {
		rustToolchain := componentFile(metadata, "rust-toolchain.toml")
		if data, err := readRepositoryFile(root, rustToolchain); err == nil {
			value, parseErr := rustToolchainVersion(data)
			if parseErr != nil {
				return inputError(rule, rustToolchain)
			}
			selectors = append(selectors, "rust="+value)
			if !exactPatchVersion(value) {
				return baseFinding(rule, deviationStatus(rule), rustToolchain, "Rust is not pinned to an exact release.")
			}
		} else if !errors.Is(err, errNotFound) {
			return inputError(rule, rustToolchain)
		}
	}
	if len(selectors) == 0 {
		return baseFinding(rule, "skip", ".", "No runtime or repository tool selector was detected.")
	}
	return baseFinding(rule, "pass", ".", "Detected runtime and repository tool selectors resolve to exact versions.")
}

func evaluateMiseLock(root string, _ Metadata, rule Rule, _ bool) Finding {
	data, err := readRepositoryFile(root, "mise.toml")
	if errors.Is(err, errNotFound) {
		return baseFinding(rule, "skip", "mise.toml", "Mise does not manage repository tools.")
	}
	if err != nil {
		return inputError(rule, "mise.toml")
	}
	config, err := parseMiseConfig(data)
	if err != nil {
		return inputError(rule, "mise.toml")
	}
	if len(config.Tools) == 0 {
		return baseFinding(rule, "skip", "mise.toml", "Mise configuration does not manage repository tools.")
	}
	if !minimumMiseVersion(config.MinVersion) {
		return baseFinding(rule, deviationStatus(rule), "mise.toml", "Mise configuration does not declare a bounded minimum mise version.")
	}
	lockData, err := readRepositoryFile(root, "mise.lock")
	if errors.Is(err, errNotFound) {
		return baseFinding(rule, deviationStatus(rule), "mise.lock", "Mise manages tools but mise.lock is missing.")
	} else if err != nil {
		return inputError(rule, "mise.lock")
	}
	lock, err := parseMiseLock(lockData)
	if err != nil {
		return inputError(rule, "mise.lock")
	}
	for _, tool := range sortedMapKeys(config.Tools) {
		if _, resolveErr := resolveMiseToolVersion(config, lock.Tools, tool); resolveErr != nil {
			finding := baseFinding(
				rule,
				deviationStatus(rule),
				"mise.lock",
				"Mise lock does not resolve configured tool "+tool+" to one exact version allowed by its selector.",
			)
			finding.Secondary = tool
			return finding
		}
	}
	return baseFinding(rule, "pass", "mise.lock", "Mise configuration and lock resolve every configured tool to one exact version.")
}

func evaluateDependencyRecords(root string, metadata Metadata, rule Rule, _ bool) Finding {
	type requirement struct {
		profile string
		files   []string
	}
	requirements := []requirement{
		{"node-typescript", []string{"package.json", "pnpm-lock.yaml"}},
		{"python", []string{"pyproject.toml", "uv.lock"}},
		{"go", nil},
		{"rust", []string{"Cargo.toml", "Cargo.lock"}},
		{"zig", []string{"build.zig", "build.zig.zon"}},
		{"infrastructure-aws-cdk", []string{"cdk.json"}},
		{"infrastructure-terraform", nil},
		{"infrastructure-opentofu", nil},
		{"infrastructure-pulumi", []string{"Pulumi.yaml"}},
	}
	checked := false
	for _, requirement := range requirements {
		if !slices.Contains(metadata.Profiles, requirement.profile) {
			continue
		}
		checked = true
		if requirement.profile == "go" {
			if _, finding := loadGoWorkspace(root, metadata, rule); finding != nil {
				return *finding
			}
			continue
		}
		if requirement.profile == "infrastructure-terraform" || requirement.profile == "infrastructure-opentofu" {
			terraformFiles, filesErr := repositoryDirectoryFiles(root, metadata.ComponentPath, 256, ".tf", ".tf.json")
			if filesErr != nil {
				return inputError(rule, componentFile(metadata, "."))
			}
			if len(terraformFiles) == 0 {
				return baseFinding(rule, deviationStatus(rule), componentFile(metadata, "."), "Terraform or OpenTofu root requires at least one configuration file.")
			}
			lockPath := componentFile(metadata, ".terraform.lock.hcl")
			if _, err := readRepositoryFile(root, lockPath); err != nil {
				return missingOrInput(rule, lockPath, err, "Terraform or OpenTofu root requires .terraform.lock.hcl.")
			}
			continue
		}
		for _, name := range requirement.files {
			componentName := componentFile(metadata, name)
			if _, err := readRepositoryFile(root, componentName); errors.Is(err, errNotFound) {
				return baseFinding(rule, deviationStatus(rule), componentName, "The "+requirement.profile+" profile is missing a required native manifest or resolution record.")
			} else if err != nil {
				return inputError(rule, componentName)
			}
		}
	}
	if !checked {
		return baseFinding(rule, "skip", ".", "The declared profiles do not resolve third-party dependencies through a supported native record.")
	}
	return baseFinding(rule, "pass", ".", "Required profile-native manifests and resolution records are present.")
}

func evaluateDirectDependencies(root string, metadata Metadata, rule Rule, _ bool) Finding {
	var detected bool
	if slices.Contains(metadata.Profiles, "node-typescript") {
		packageJSON := componentFile(metadata, "package.json")
		if data, err := readRepositoryFile(root, packageJSON); err == nil {
			var manifest map[string]any
			if json.Unmarshal(data, &manifest) != nil {
				return inputError(rule, packageJSON)
			}
			for _, section := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
				dependencies, _ := manifest[section].(map[string]any)
				for name, raw := range dependencies {
					value, _ := raw.(string)
					if isDirectReference(value) {
						detected = true
						if !directReferenceIsImmutable(value) {
							finding := baseFinding(rule, deviationStatus(rule), packageJSON, "Direct dependency "+name+" is not pinned to an immutable reference with integrity.")
							finding.Secondary = name
							return finding
						}
					}
				}
			}
		} else if !errors.Is(err, errNotFound) {
			return inputError(rule, packageJSON)
		}
	}
	var tomlNames []string
	if slices.Contains(metadata.Profiles, "python") {
		tomlNames = append(tomlNames, "pyproject.toml")
	}
	if slices.Contains(metadata.Profiles, "rust") {
		tomlNames = append(tomlNames, "Cargo.toml")
	}
	for _, name := range tomlNames {
		componentName := componentFile(metadata, name)
		data, err := readRepositoryFile(root, componentName)
		if errors.Is(err, errNotFound) {
			continue
		}
		if err != nil {
			return inputError(rule, componentName)
		}
		var document map[string]any
		if parseErr := toml.Unmarshal(data, &document); parseErr != nil {
			return inputError(rule, componentName)
		}
		references := collectTOMLDirectReferences(document)
		for _, reference := range references {
			detected = true
			if !reference.immutable {
				finding := baseFinding(rule, deviationStatus(rule), componentName, "A direct dependency does not declare an immutable commit or integrity digest.")
				finding.Secondary = reference.path
				return finding
			}
		}
	}
	if slices.Contains(metadata.Profiles, "infrastructure-terraform") || slices.Contains(metadata.Profiles, "infrastructure-opentofu") {
		terraformFinding, terraformDetected := evaluateTerraformDirectDependencies(root, metadata, rule)
		if terraformFinding != nil {
			return *terraformFinding
		}
		detected = detected || terraformDetected
	}
	if !detected {
		return baseFinding(rule, "skip", ".", "No direct VCS, URL, archive, binary, or generated-source dependency was detected.")
	}
	return baseFinding(rule, "skip", ".", "Detected direct references are immutable, but ecosystem integrity-record correlation is outside this structural check.")
}

func evaluateTerraformDirectDependencies(root string, metadata Metadata, rule Rule) (*Finding, bool) {
	const maxTerraformFiles = 256
	type moduleDirectory struct {
		path     string
		required bool
	}
	rootPath := metadata.ComponentPath
	if rootPath == "" {
		rootPath = "."
	}
	queue := []moduleDirectory{{path: rootPath}}
	visited := map[string]bool{}
	fileCount := 0
	detected := false
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current.path] {
			continue
		}
		visited[current.path] = true
		files, err := repositoryDirectoryFiles(root, current.path, maxTerraformFiles, ".tf", ".tf.json")
		if err != nil || (len(files) == 0 && current.required) {
			finding := inputError(rule, current.path)
			return &finding, detected
		}
		fileCount += len(files)
		if fileCount > maxTerraformFiles {
			finding := inputError(rule, componentFile(metadata, "."))
			return &finding, detected
		}
		for _, name := range files {
			data, readErr := readRepositoryFile(root, name)
			if readErr != nil {
				finding := inputError(rule, name)
				return &finding, detected
			}
			file, diagnostics := parseTerraformConfiguration(data, name)
			if diagnostics.HasErrors() {
				finding := inputError(rule, name)
				return &finding, detected
			}
			content, _, contentDiagnostics := file.Body.PartialContent(&hcl.BodySchema{
				Blocks: []hcl.BlockHeaderSchema{{Type: "module", LabelNames: []string{"name"}}},
			})
			if contentDiagnostics.HasErrors() {
				finding := inputError(rule, name)
				return &finding, detected
			}
			for _, block := range content.Blocks {
				attributes, attributeDiagnostics := block.Body.JustAttributes()
				if attributeDiagnostics.HasErrors() {
					finding := inputError(rule, name)
					return &finding, detected
				}
				moduleName := strings.Join(block.Labels, ".")
				sourceAttribute, exists := attributes["source"]
				if !exists {
					finding := baseFinding(rule, deviationStatus(rule), name, "Terraform or OpenTofu module dependency is missing a static source.")
					finding.Secondary = moduleName
					return &finding, detected
				}
				source, sourceOK := hclLiteralString(sourceAttribute)
				if !sourceOK {
					finding := baseFinding(rule, deviationStatus(rule), name, "Terraform or OpenTofu module source must be a static immutable reference.")
					finding.Secondary = moduleName
					return &finding, detected
				}
				if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
					localPath := path.Clean(path.Join(path.Dir(name), source))
					if localPath == ".." || strings.HasPrefix(localPath, "../") || path.IsAbs(source) || strings.Contains(source, "\\") {
						finding := baseFinding(rule, deviationStatus(rule), name, "Terraform or OpenTofu local module source must remain within the repository.")
						finding.Secondary = moduleName
						return &finding, detected
					}
					if visited[localPath] {
						continue
					}
					queue = append(queue, moduleDirectory{path: localPath, required: true})
					continue
				}
				detected = true
				if terraformRemoteSource(source) {
					if !directReferenceIsImmutable(source) {
						finding := baseFinding(rule, deviationStatus(rule), name, "Terraform or OpenTofu VCS or URL module source must pin an immutable commit or integrity digest.")
						finding.Secondary = moduleName
						return &finding, detected
					}
					continue
				}
				versionAttribute, exists := attributes["version"]
				version, versionOK := hclLiteralString(versionAttribute)
				if !exists || !versionOK || !exactTerraformModuleVersion(version) {
					finding := baseFinding(rule, deviationStatus(rule), name, "Terraform or OpenTofu registry module must pin one exact version.")
					finding.Secondary = moduleName
					return &finding, detected
				}
			}
		}
	}
	return nil, detected
}

func parseTerraformConfiguration(data []byte, name string) (*hcl.File, hcl.Diagnostics) {
	if strings.HasSuffix(name, ".tf.json") {
		return hcljson.Parse(data, name)
	}
	return hclsyntax.ParseConfig(data, name, hcl.Pos{Line: 1, Column: 1})
}

func hclLiteralString(attribute *hcl.Attribute) (string, bool) {
	if attribute == nil {
		return "", false
	}
	value, diagnostics := attribute.Expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type().FriendlyName() != "string" {
		return "", false
	}
	return value.AsString(), true
}

func terraformRemoteSource(source string) bool {
	lower := strings.ToLower(source)
	return strings.Contains(lower, "::") ||
		strings.HasPrefix(lower, "git@") ||
		strings.Contains(lower, "://") ||
		strings.HasPrefix(lower, "github.com/") ||
		strings.HasPrefix(lower, "bitbucket.org/")
}

func exactTerraformModuleVersion(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "=") {
		value = strings.TrimSpace(strings.TrimPrefix(value, "="))
	}
	return exactPatchVersion(value)
}

func evaluateImmutableActions(root string, _ Metadata, rule Rule, _ bool) Finding {
	files, err := repositoryFiles(root, ".github/workflows", 256, ".yml", ".yaml")
	if err != nil {
		return inputError(rule, ".github/workflows")
	}
	if len(files) == 0 {
		return baseFinding(rule, "skip", ".github/workflows", "No GitHub Actions workflow references were detected.")
	}
	var remoteCount int
	var localActions []localActionReference
	for _, name := range files {
		data, readErr := readRepositoryFile(root, name)
		if readErr != nil {
			return inputError(rule, name)
		}
		references, parseErr := workflowExecutableReferences(data)
		if parseErr != nil {
			return inputError(rule, name)
		}
		for _, reference := range references {
			switch reference.kind {
			case executableLocalAction:
				localActions = append(localActions, localActionReference{value: reference.value, origin: name})
			case executableAction:
				remoteCount++
				if immutableActionReference(reference.value) {
					continue
				}
				finding := baseFinding(rule, deviationStatus(rule), name, "Remote executable reference is not pinned by full commit SHA or image digest.")
				finding.Secondary = boundedFindingSecondary(reference.value)
				return finding
			case executableImage:
				remoteCount++
				if immutableImageReference(reference.value) {
					continue
				}
				finding := baseFinding(rule, deviationStatus(rule), name, "Container image reference is not pinned by SHA-256 digest.")
				finding.Secondary = boundedFindingSecondary(reference.value)
				return finding
			}
		}
	}
	seenLocalActions := map[string]bool{}
	seenLocalActionReferences := map[string]bool{}
	for index := 0; index < len(localActions); index++ {
		reference := localActions[index]
		if seenLocalActionReferences[reference.value] {
			continue
		}
		seenLocalActionReferences[reference.value] = true
		metadataPath, data, resolveErr := readLocalActionMetadata(root, reference.value)
		if resolveErr != nil {
			finding := inputError(rule, reference.origin)
			finding.Secondary = boundedFindingSecondary(reference.value)
			return finding
		}
		if seenLocalActions[metadataPath] {
			continue
		}
		if len(seenLocalActions) >= 256 {
			return inputError(rule, ".github/actions")
		}
		seenLocalActions[metadataPath] = true
		references, parseErr := actionExecutableReferences(data)
		if parseErr != nil {
			return inputError(rule, metadataPath)
		}
		for _, reference := range references {
			switch reference.kind {
			case executableLocalAction:
				localActions = append(localActions, localActionReference{value: reference.value, origin: metadataPath})
			case executableAction:
				remoteCount++
				if immutableActionReference(reference.value) {
					continue
				}
				finding := baseFinding(rule, deviationStatus(rule), metadataPath, "Remote executable reference is not pinned by full commit SHA or image digest.")
				finding.Secondary = boundedFindingSecondary(reference.value)
				return finding
			case executableImage:
				remoteCount++
				if immutableImageReference(reference.value) {
					continue
				}
				finding := baseFinding(rule, deviationStatus(rule), metadataPath, "Container image reference is not pinned by SHA-256 digest.")
				finding.Secondary = boundedFindingSecondary(reference.value)
				return finding
			}
		}
	}
	if remoteCount == 0 {
		return baseFinding(rule, "skip", ".github/workflows", "No remote executable workflow reference was detected.")
	}
	return baseFinding(
		rule,
		"skip",
		".github/workflows",
		"Detected workflow action and container references are immutable; archive, binary, VCS, generated-source, and script correlation remains outside this structural check.",
	)
}

type executableReferenceKind uint8

const (
	executableAction executableReferenceKind = iota
	executableImage
	executableLocalAction
)

type executableReference struct {
	kind  executableReferenceKind
	value string
}

type localActionReference struct {
	value  string
	origin string
}

func boundedFindingSecondary(value string) string {
	const maximumRunes = 500
	runes := []rune(value)
	if len(runes) > maximumRunes {
		runes = runes[:maximumRunes]
	}
	return string(runes)
}

func workflowExecutableReferences(data []byte) ([]executableReference, error) {
	document, err := decodeWorkflowYAML(data)
	if err != nil {
		return nil, err
	}
	jobs, err := workflowYAMLMappingValue(document, "jobs")
	if err != nil {
		return nil, err
	}
	if jobs == nil {
		return nil, nil
	}
	jobs, err = dereferenceWorkflowYAMLNode(jobs)
	if err != nil {
		return nil, err
	}
	if jobs.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow jobs must be a mapping")
	}
	var references []executableReference
	jobEntries, err := workflowYAMLMappingEntries(jobs, "workflow job")
	if err != nil {
		return nil, err
	}
	for _, entry := range jobEntries {
		jobName := entry.name
		job, err := dereferenceWorkflowYAMLNode(entry.value)
		if err != nil {
			return nil, err
		}
		if job.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("workflow job %q must be a mapping", jobName)
		}
		uses, err := workflowYAMLMappingValue(job, "uses")
		if err != nil {
			return nil, err
		}
		if uses != nil {
			reference, err := workflowYAMLString(uses, "job uses")
			if err != nil {
				return nil, err
			}
			if !localExecutableReference(reference) {
				references = append(references, executableReference{kind: executableAction, value: reference})
			}
		}
		container, err := workflowYAMLMappingValue(job, "container")
		if err != nil {
			return nil, err
		}
		if container != nil {
			containerReferences, err := containerExecutableReferences(container, "job container")
			if err != nil {
				return nil, err
			}
			references = append(references, containerReferences...)
		}
		services, err := workflowYAMLMappingValue(job, "services")
		if err != nil {
			return nil, err
		}
		if services != nil {
			services, err = dereferenceWorkflowYAMLNode(services)
			if err != nil {
				return nil, err
			}
			if services.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("workflow job services must be a mapping")
			}
			serviceEntries, err := workflowYAMLMappingEntries(services, "workflow service")
			if err != nil {
				return nil, err
			}
			for _, entry := range serviceEntries {
				serviceName := entry.name
				service, err := dereferenceWorkflowYAMLNode(entry.value)
				if err != nil {
					return nil, err
				}
				if service.Kind != yaml.MappingNode {
					return nil, fmt.Errorf("workflow service %q must be a mapping", serviceName)
				}
				image, err := workflowYAMLMappingValue(service, "image")
				if err != nil {
					return nil, err
				}
				if image == nil {
					return nil, fmt.Errorf("workflow service %q is missing image", serviceName)
				}
				reference, err := workflowYAMLString(image, "service image")
				if err != nil {
					return nil, err
				}
				references = append(references, executableReference{kind: executableImage, value: reference})
			}
		}
		steps, err := workflowYAMLMappingValue(job, "steps")
		if err != nil {
			return nil, err
		}
		if steps != nil {
			stepReferences, err := stepExecutableReferences(steps)
			if err != nil {
				return nil, err
			}
			references = append(references, stepReferences...)
		}
	}
	return references, nil
}

func actionExecutableReferences(data []byte) ([]executableReference, error) {
	document, err := decodeWorkflowYAML(data)
	if err != nil {
		return nil, err
	}
	runs, err := workflowYAMLMappingValue(document, "runs")
	if err != nil {
		return nil, err
	}
	if runs == nil {
		return nil, fmt.Errorf("action metadata is missing runs")
	}
	runs, err = dereferenceWorkflowYAMLNode(runs)
	if err != nil {
		return nil, err
	}
	if runs.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("action runs must be a mapping")
	}
	usingValue, err := workflowYAMLMappingValue(runs, "using")
	if err != nil {
		return nil, err
	}
	if usingValue == nil {
		return nil, fmt.Errorf("action metadata is missing runs.using")
	}
	using, err := workflowYAMLString(usingValue, "action runs.using")
	if err != nil {
		return nil, err
	}
	switch using {
	case "composite":
		steps, err := workflowYAMLMappingValue(runs, "steps")
		if err != nil {
			return nil, err
		}
		if steps == nil {
			return nil, fmt.Errorf("composite action is missing steps")
		}
		return stepExecutableReferences(steps)
	case "docker":
		imageValue, err := workflowYAMLMappingValue(runs, "image")
		if err != nil {
			return nil, err
		}
		if imageValue == nil {
			return nil, fmt.Errorf("docker action is missing runs.image")
		}
		image, err := workflowYAMLString(imageValue, "Docker action image")
		if err != nil {
			return nil, err
		}
		if image == "Dockerfile" || strings.HasPrefix(image, "./") {
			return nil, nil
		}
		if strings.HasPrefix(image, "docker://") {
			return []executableReference{{kind: executableAction, value: image}}, nil
		}
		return []executableReference{{kind: executableImage, value: image}}, nil
	default:
		return nil, nil
	}
}

func stepExecutableReferences(value *yaml.Node) ([]executableReference, error) {
	steps, err := dereferenceWorkflowYAMLNode(value)
	if err != nil {
		return nil, err
	}
	if steps.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("workflow or action steps must be a sequence")
	}
	var references []executableReference
	for index, stepValue := range steps.Content {
		step, err := dereferenceWorkflowYAMLNode(stepValue)
		if err != nil {
			return nil, err
		}
		if step.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("step %d must be a mapping", index)
		}
		usesValue, err := workflowYAMLMappingValue(step, "uses")
		if err != nil {
			return nil, err
		}
		if usesValue == nil {
			continue
		}
		reference, err := workflowYAMLString(usesValue, "step uses")
		if err != nil {
			return nil, err
		}
		kind := executableAction
		if localExecutableReference(reference) {
			kind = executableLocalAction
		}
		references = append(references, executableReference{kind: kind, value: reference})
	}
	return references, nil
}

func containerExecutableReferences(value *yaml.Node, context string) ([]executableReference, error) {
	container, err := dereferenceWorkflowYAMLNode(value)
	if err != nil {
		return nil, err
	}
	if container.Kind == yaml.ScalarNode {
		image, err := workflowYAMLString(container, context+" image")
		if err != nil {
			return nil, err
		}
		return []executableReference{{kind: executableImage, value: image}}, nil
	}
	if container.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a string or mapping", context)
	}
	imageValue, err := workflowYAMLMappingValue(container, "image")
	if err != nil {
		return nil, err
	}
	if imageValue == nil {
		return nil, fmt.Errorf("%s is missing image", context)
	}
	image, err := workflowYAMLString(imageValue, context+" image")
	if err != nil {
		return nil, err
	}
	return []executableReference{{kind: executableImage, value: image}}, nil
}

func decodeWorkflowYAML(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse workflow YAML: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("workflow YAML must contain exactly one document")
		}
		return nil, fmt.Errorf("parse trailing workflow YAML: %w", err)
	}
	if len(document.Content) != 1 {
		return nil, fmt.Errorf("workflow YAML must contain exactly one root value")
	}
	root, err := dereferenceWorkflowYAMLNode(document.Content[0])
	if err != nil {
		return nil, err
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow YAML root must be a mapping")
	}
	if err := validateWorkflowYAMLNode(root, map[*yaml.Node]bool{}); err != nil {
		return nil, err
	}
	return root, nil
}

func dereferenceWorkflowYAMLNode(node *yaml.Node) (*yaml.Node, error) {
	for depth := 0; node != nil && node.Kind == yaml.AliasNode; depth++ {
		if depth >= 32 || node.Alias == nil {
			return nil, fmt.Errorf("workflow YAML alias depth exceeds limit")
		}
		node = node.Alias
	}
	if node == nil {
		return nil, fmt.Errorf("workflow YAML node is missing")
	}
	return node, nil
}

func workflowYAMLMappingValue(node *yaml.Node, key string) (*yaml.Node, error) {
	mapping, err := dereferenceWorkflowYAMLNode(node)
	if err != nil {
		return nil, err
	}
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow YAML value must be a mapping")
	}
	var result *yaml.Node
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		name, err := workflowYAMLString(mapping.Content[index], "workflow YAML mapping key")
		if err != nil {
			return nil, err
		}
		if name != key {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("workflow YAML mapping contains duplicate key %q", key)
		}
		result = mapping.Content[index+1]
	}
	return result, nil
}

type workflowYAMLMappingEntry struct {
	name  string
	value *yaml.Node
}

func workflowYAMLMappingEntries(node *yaml.Node, context string) ([]workflowYAMLMappingEntry, error) {
	mapping, err := dereferenceWorkflowYAMLNode(node)
	if err != nil {
		return nil, err
	}
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s value must be a mapping", context)
	}
	entries := make([]workflowYAMLMappingEntry, 0, len(mapping.Content)/2)
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		name, err := workflowYAMLString(mapping.Content[index], context+" name")
		if err != nil {
			return nil, err
		}
		entries = append(entries, workflowYAMLMappingEntry{name: name, value: mapping.Content[index+1]})
	}
	slices.SortFunc(entries, func(left, right workflowYAMLMappingEntry) int {
		return strings.Compare(left.name, right.name)
	})
	return entries, nil
}

func validateWorkflowYAMLNode(node *yaml.Node, validated map[*yaml.Node]bool) error {
	resolved, err := dereferenceWorkflowYAMLNode(node)
	if err != nil {
		return err
	}
	if validated[resolved] {
		return nil
	}
	validated[resolved] = true
	switch resolved.Kind {
	case yaml.MappingNode:
		seen := map[string]bool{}
		for index := 0; index+1 < len(resolved.Content); index += 2 {
			key, err := workflowYAMLString(resolved.Content[index], "workflow YAML mapping key")
			if err != nil {
				return err
			}
			if seen[key] {
				return fmt.Errorf("workflow YAML mapping contains duplicate key %q", key)
			}
			seen[key] = true
			if err := validateWorkflowYAMLNode(resolved.Content[index+1], validated); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range resolved.Content {
			if err := validateWorkflowYAMLNode(child, validated); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		return nil
	default:
		return fmt.Errorf("workflow YAML contains an unsupported node")
	}
	return nil
}

func workflowYAMLString(value *yaml.Node, context string) (string, error) {
	scalar, err := dereferenceWorkflowYAMLNode(value)
	if err != nil {
		return "", err
	}
	if scalar.Kind != yaml.ScalarNode || scalar.Tag != "!!str" {
		return "", fmt.Errorf("%s must be a non-empty string", context)
	}
	reference := strings.TrimSpace(scalar.Value)
	if reference == "" {
		return "", fmt.Errorf("%s must be a non-empty string", context)
	}
	return reference, nil
}

func localExecutableReference(reference string) bool {
	return strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "$/")
}

func readLocalActionMetadata(root, reference string) (string, []byte, error) {
	if !localExecutableReference(reference) {
		return "", nil, fmt.Errorf("action reference is not repository-local")
	}
	clean := path.Clean(reference[2:])
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || strings.Contains(clean, "\\") {
		return "", nil, fmt.Errorf("local action reference escapes repository root")
	}
	for _, name := range []string{"action.yml", "action.yaml"} {
		metadataPath := path.Join(clean, name)
		data, err := readRepositoryFile(root, metadataPath)
		if err == nil {
			return metadataPath, data, nil
		}
		if !errors.Is(err, errNotFound) {
			return metadataPath, nil, err
		}
	}
	return clean, nil, errNotFound
}

func evaluateAssetVersion(_ string, metadata Metadata, rule Rule, _ bool) Finding {
	if metadata.StandardVersion == "" || metadata.AssetBundleVersion == "" {
		return baseFinding(rule, deviationStatus(rule), ".github/golden-path.yaml", "Exact standard and asset bundle versions are not recorded.")
	}
	return baseFinding(rule, "skip", ".github/golden-path.yaml", "Exact standard and asset bundle versions are recorded; change detection and just ci evidence are outside this structural invocation.")
}

func evaluateCheckerBoundary(_ string, _ Metadata, rule Rule, _ bool) Finding {
	return baseFinding(rule, "pass", ".", "This checker release evaluates bounded repository-local data without network access, credentials, external mutation, or repository writes.")
}

func evaluateExceptionFile(_ string, _ Metadata, rule Rule, exceptionsPresent bool) Finding {
	if !exceptionsPresent {
		return baseFinding(rule, "skip", ".github/golden-path-exceptions.yaml", "No Golden Path exception file is present.")
	}
	return baseFinding(rule, "pass", ".github/golden-path-exceptions.yaml", "Golden Path exceptions are schema-valid and reference known waivable MUST rules.")
}

func evaluateWorkflowTemplates(root string, _ Metadata, rule Rule, _ bool) Finding {
	files, err := repositoryFiles(root, "workflow-templates", 64, ".yml", ".yaml")
	if err != nil {
		return inputError(rule, "workflow-templates")
	}
	if len(files) == 0 {
		return baseFinding(rule, "skip", "workflow-templates", "No organization workflow template is published by this repository.")
	}
	for _, name := range files {
		data, readErr := readRepositoryFile(root, name)
		if readErr != nil {
			return inputError(rule, name)
		}
		properties := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml") + ".properties.json"
		if _, readErr := readRepositoryFile(root, properties); readErr != nil {
			return baseFinding(rule, deviationStatus(rule), properties, "Workflow template metadata is missing or unreadable.")
		}
		text := string(data)
		if !strings.Contains(text, "permissions:") || !strings.Contains(text, "just ci") {
			return baseFinding(rule, deviationStatus(rule), name, "Workflow template must declare permissions and delegate repository quality behavior to just ci.")
		}
	}
	return baseFinding(rule, "skip", "workflow-templates", "Workflow templates have matching files and thin caller markers; full GitHub workflow semantics require the repository quality gate.")
}

func evaluateRuntimeDisposition(
	root string,
	metadata Metadata,
	rule Rule,
	evaluatedAt time.Time,
	compatibility CompatibilityManifest,
) Finding {
	selections := make(map[string]RuntimeSelection, len(compatibility.RuntimeSelections))
	for _, selection := range compatibility.RuntimeSelections {
		selections[selection.Profile] = selection
	}
	for _, profile := range metadata.Profiles {
		selection, managed := selections[profile]
		if !managed {
			continue
		}
		var version, findingPath string
		switch profile {
		case "node-typescript":
			selected, finding := miseVersion(root, "node", rule)
			if finding != nil {
				return *finding
			}
			version, findingPath = selected, "mise.toml"
		case "python":
			name := componentFile(metadata, ".python-version")
			data, err := readRepositoryFile(root, name)
			if err != nil {
				return missingOrInput(rule, name, err, "Python runtime selector is missing.")
			}
			version, findingPath = strings.TrimSpace(string(data)), name
		case "go":
			selected, finding := miseVersion(root, "go", rule)
			if finding != nil {
				return *finding
			}
			version, findingPath = selected, "mise.toml"
		case "rust":
			name := componentFile(metadata, "rust-toolchain.toml")
			data, err := readRepositoryFile(root, name)
			if err != nil {
				return missingOrInput(rule, name, err, "Rust runtime selector is missing.")
			}
			selected, parseErr := rustToolchainVersion(data)
			if parseErr != nil {
				return inputError(rule, name)
			}
			version, findingPath = selected, name
		case "zig", "zig-toolchain":
			selected, finding := miseVersion(root, "zig", rule)
			if finding != nil {
				return *finding
			}
			version, findingPath = selected, "mise.toml"
		}
		var matched *RuntimeSelectionVersion
		for index := range selection.Versions {
			if selection.Versions[index].Version == version {
				matched = &selection.Versions[index]
				break
			}
		}
		if matched == nil || matched.Disposition == "blocked" {
			return baseFinding(
				rule,
				deviationStatus(rule),
				findingPath,
				"Selected "+selection.Tool+" runtime is not an allowed exact release in the checker compatibility manifest.",
			)
		}
		if matched.SupportEndsAt != "" &&
			evaluatedAt.UTC().Format("2006-01-02") > matched.SupportEndsAt {
			return baseFinding(
				rule,
				deviationStatus(rule),
				findingPath,
				"Selected "+selection.Tool+" runtime is past its compatibility support deadline.",
			)
		}
	}
	return baseFinding(rule, "pass", ".", "Selected language runtimes match exact allowed releases in the checker compatibility manifest.")
}

func evaluateNodeProfile(root string, metadata Metadata, rule Rule, _ bool) Finding {
	if _, finding := miseVersion(root, "node", rule); finding != nil {
		return *finding
	}
	packageJSON := componentFile(metadata, "package.json")
	data, err := readRepositoryFile(root, packageJSON)
	if err != nil {
		return missingOrInput(rule, packageJSON, err, "Node profile requires package.json.")
	}
	var manifest struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return inputError(rule, packageJSON)
	}
	if !pnpmExactPattern.MatchString(manifest.PackageManager) {
		return baseFinding(rule, deviationStatus(rule), packageJSON, "packageManager must pin an exact pnpm version.")
	}
	pnpmLock := componentFile(metadata, "pnpm-lock.yaml")
	if _, err := readRepositoryFile(root, pnpmLock); err != nil {
		return missingOrInput(rule, pnpmLock, err, "Node profile requires pnpm-lock.yaml.")
	}
	return baseFinding(rule, "skip", packageJSON, "Node.js and pnpm authority is exact and pnpm-lock.yaml is present; frozen-install CI semantics require the repository quality gate.")
}

func evaluatePythonProfile(root string, metadata Metadata, rule Rule, _ bool) Finding {
	for _, name := range []string{"pyproject.toml", "uv.lock", ".python-version"} {
		componentName := componentFile(metadata, name)
		if _, err := readRepositoryFile(root, componentName); err != nil {
			return missingOrInput(rule, componentName, err, "Python profile requires "+name+".")
		}
	}
	pythonVersion := componentFile(metadata, ".python-version")
	data, _ := readRepositoryFile(root, pythonVersion)
	if !exactPatchVersion(strings.TrimSpace(string(data))) {
		return baseFinding(rule, deviationStatus(rule), pythonVersion, "Python must be pinned to an exact patch version.")
	}
	if miseData, err := readRepositoryFile(root, "mise.toml"); err == nil {
		config, parseErr := parseMiseConfig(miseData)
		if parseErr != nil {
			return inputError(rule, "mise.toml")
		}
		if _, exists := config.Tools["python"]; exists {
			return baseFinding(rule, deviationStatus(rule), "mise.toml", "Uv, not mise, must own the Python runtime.")
		}
	}
	return baseFinding(rule, "skip", componentFile(metadata, "pyproject.toml"), "Uv owns exact Python runtime and dependency records; locked-sync CI semantics require the repository quality gate.")
}

func evaluateGoAuthority(root string, metadata Metadata, rule Rule, _ bool) Finding {
	miseGo, finding := miseVersion(root, "go", rule)
	if finding != nil {
		return *finding
	}
	workspace, workspaceFinding := loadGoWorkspace(root, metadata, rule)
	if workspaceFinding != nil {
		return *workspaceFinding
	}
	parts := versionParts(miseGo)
	if len(parts) != 3 {
		return baseFinding(rule, deviationStatus(rule), workspace.authorityPath, "Go runtime declarations are incomplete.")
	}
	files := append([]goModuleRecord{{path: workspace.authorityPath, data: workspace.authorityData}}, workspace.modules...)
	files = append(files, workspace.runtimeFiles...)
	seen := map[string]bool{}
	for _, file := range files {
		if seen[file.path] {
			continue
		}
		seen[file.path] = true
		goVersion, toolchain, parseErr := goRuntimeDirectives(file.path, file.data)
		goVersionParts := versionParts(goVersion)
		if parseErr != nil || len(goVersionParts) < 2 || len(goVersionParts) > 3 {
			return baseFinding(rule, deviationStatus(rule), file.path, "Go runtime declarations are incomplete.")
		}
		if compareNumericVersions(goVersion, miseGo) > 0 {
			return baseFinding(rule, deviationStatus(rule), file.path, "The Go directive contradicts the mise-selected runtime.")
		}
		if toolchain != "" && toolchain != "default" && toolchain != "go"+miseGo {
			return baseFinding(rule, deviationStatus(rule), file.path, "The Go toolchain directive contradicts the mise-selected runtime.")
		}
	}
	return baseFinding(rule, "pass", workspace.authorityPath, "Mise and native Go module or workspace runtime declarations align.")
}

func goRuntimeDirectives(name string, data []byte) (string, string, error) {
	if strings.HasSuffix(name, "go.work") {
		workFile, err := modfile.ParseWork(name, data, nil)
		if err != nil {
			return "", "", err
		}
		var goVersion, toolchain string
		if workFile.Go != nil {
			goVersion = workFile.Go.Version
		}
		if workFile.Toolchain != nil {
			toolchain = workFile.Toolchain.Name
		}
		return goVersion, toolchain, nil
	}
	moduleFile, err := modfile.Parse(name, data, nil)
	if err != nil {
		return "", "", err
	}
	var goVersion, toolchain string
	if moduleFile.Go != nil {
		goVersion = moduleFile.Go.Version
	}
	if moduleFile.Toolchain != nil {
		toolchain = moduleFile.Toolchain.Name
	}
	return goVersion, toolchain, nil
}

func evaluateGoDependencies(root string, metadata Metadata, rule Rule, _ bool) Finding {
	workspace, workspaceFinding := loadGoWorkspace(root, metadata, rule)
	if workspaceFinding != nil {
		return *workspaceFinding
	}
	moduleFiles := make([]*modfile.File, 0, len(workspace.modules))
	workspaceModules := make(map[string]bool, len(workspace.modules))
	for _, module := range workspace.modules {
		moduleFile, parseErr := modfile.Parse(module.path, module.data, nil)
		if parseErr != nil || moduleFile.Module == nil || moduleFile.Module.Mod.Path == "" {
			return inputError(rule, module.path)
		}
		moduleFiles = append(moduleFiles, moduleFile)
		workspaceModules[moduleFile.Module.Mod.Path] = true
	}
	for index, module := range workspace.modules {
		moduleFile := moduleFiles[index]
		hasExternalRequirement := false
		for _, requirement := range moduleFile.Require {
			if workspaceModules[requirement.Mod.Path] || goRequirementUsesLocalReplacement(moduleFile, requirement) {
				continue
			}
			hasExternalRequirement = true
			break
		}
		if hasExternalRequirement {
			goSum := path.Join(path.Dir(module.path), "go.sum")
			if _, err := readRepositoryFile(root, goSum); err != nil {
				return missingOrInput(rule, goSum, err, "Go modules resolve third-party dependencies but go.sum is missing.")
			}
		}
	}
	return baseFinding(rule, "skip", workspace.authorityPath, "Go dependency authority uses native module or workspace records; sync, tidy, verify, and go tool execution require the repository quality gate.")
}

func goRequirementUsesLocalReplacement(moduleFile *modfile.File, requirement *modfile.Require) bool {
	for _, replacement := range moduleFile.Replace {
		if replacement.Old.Path != requirement.Mod.Path ||
			replacement.Old.Version != "" && replacement.Old.Version != requirement.Mod.Version ||
			replacement.New.Version != "" {
			continue
		}
		return path.IsAbs(replacement.New.Path) ||
			replacement.New.Path == "." ||
			strings.HasPrefix(replacement.New.Path, "./") ||
			strings.HasPrefix(replacement.New.Path, "../")
	}
	return false
}

func evaluateRustProfile(root string, metadata Metadata, rule Rule, _ bool) Finding {
	rustToolchain := componentFile(metadata, "rust-toolchain.toml")
	data, err := readRepositoryFile(root, rustToolchain)
	if err != nil {
		return missingOrInput(rule, rustToolchain, err, "Rust profile requires rust-toolchain.toml.")
	}
	channel, parseErr := rustToolchainVersion(data)
	if parseErr != nil {
		return inputError(rule, rustToolchain)
	}
	if !exactPatchVersion(channel) {
		return baseFinding(rule, deviationStatus(rule), rustToolchain, "Rust toolchain channel must be an exact release.")
	}
	if mise, err := readRepositoryFile(root, "mise.toml"); err == nil {
		config, parseErr := parseMiseConfig(mise)
		if parseErr != nil {
			return inputError(rule, "mise.toml")
		}
		if _, exists := config.Tools["rust"]; exists {
			return baseFinding(rule, deviationStatus(rule), "mise.toml", "Rustup must be the sole Rust toolchain owner.")
		}
	}
	return baseFinding(rule, "pass", rustToolchain, "Rustup owns an exact Rust toolchain release.")
}

func evaluateZigProfile(root string, metadata Metadata, rule Rule, compatibility CompatibilityManifest) Finding {
	version, finding := miseVersion(root, "zig", rule)
	if finding != nil {
		return *finding
	}
	profile := "zig"
	if slices.Contains(metadata.Profiles, "zig-toolchain") {
		profile = "zig-toolchain"
	}
	for _, selection := range compatibility.RuntimeSelections {
		if selection.Profile != profile {
			continue
		}
		for _, candidate := range selection.Versions {
			if candidate.Version == version &&
				(candidate.Disposition == "preferred" || candidate.Disposition == "supported") {
				return baseFinding(rule, "pass", "mise.toml", "Zig is conditionally declared and resolves to an approved exact tagged release.")
			}
		}
	}
	return baseFinding(rule, deviationStatus(rule), "mise.toml", "Zig must resolve to a preferred or supported exact tagged release in the checker compatibility manifest.")
}

func evaluateReleaseVersion(_ string, metadata Metadata, rule Rule, _ bool) Finding {
	if !calverPattern.MatchString(metadata.StandardVersion) || !semverPattern.MatchString(metadata.AssetBundleVersion) {
		return baseFinding(rule, deviationStatus(rule), ".github/golden-path.yaml", "Standard or asset bundle version uses the wrong version scheme.")
	}
	return baseFinding(rule, "pass", ".github/golden-path.yaml", "Golden Path standard uses CalVer and the executable asset bundle uses SemVer.")
}

func inputError(rule Rule, name string) Finding {
	finding := baseFinding(rule, "error", name, "A bounded repository input could not be read or decoded safely.")
	finding.Extensions = map[string]any{"errorKind": "configuration"}
	return finding
}

func missingOrInput(rule Rule, name string, err error, missingMessage string) Finding {
	if errors.Is(err, errNotFound) {
		return baseFinding(rule, deviationStatus(rule), name, missingMessage)
	}
	return inputError(rule, name)
}

func firstExisting(root string, names ...string) (string, []byte, error) {
	for _, name := range names {
		data, err := readRepositoryFile(root, name)
		if errors.Is(err, errNotFound) {
			continue
		}
		return name, data, err
	}
	return "", nil, errNotFound
}

type miseConfig struct {
	MinVersion string            `toml:"min_version"`
	Tools      map[string]string `toml:"tools"`
}

type miseLock struct {
	Tools map[string][]struct {
		Version string `toml:"version"`
	} `toml:"tools"`
}

func parseMiseConfig(data []byte) (miseConfig, error) {
	var config miseConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return miseConfig{}, err
	}
	if config.Tools == nil {
		config.Tools = map[string]string{}
	}
	return config, nil
}

func parseMiseLock(data []byte) (struct{ Tools map[string][]string }, error) {
	var document miseLock
	if err := toml.Unmarshal(data, &document); err != nil {
		return struct{ Tools map[string][]string }{}, err
	}
	result := struct{ Tools map[string][]string }{Tools: map[string][]string{}}
	for tool, entries := range document.Tools {
		for _, entry := range entries {
			if !isExactToolVersion(entry.Version) {
				return struct{ Tools map[string][]string }{}, fmt.Errorf("mise lock tool %q has invalid version", tool)
			}
			result.Tools[tool] = append(result.Tools[tool], strings.TrimPrefix(entry.Version, "v"))
		}
	}
	if len(result.Tools) == 0 {
		return struct{ Tools map[string][]string }{}, fmt.Errorf("mise lock contains no tool resolutions")
	}
	return result, nil
}

func resolveMiseToolVersion(config miseConfig, locked map[string][]string, tool string) (string, error) {
	selector := strings.TrimPrefix(strings.TrimSpace(config.Tools[tool]), "v")
	if !reviewableMiseSelector(selector) {
		return "", fmt.Errorf("mise tool %q has an unsupported selector", tool)
	}
	matches := map[string]struct{}{}
	for _, version := range locked[tool] {
		if miseSelectorMatches(selector, version) {
			matches[version] = struct{}{}
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("mise tool %q resolves to %d exact versions", tool, len(matches))
	}
	for version := range matches {
		return version, nil
	}
	return "", fmt.Errorf("mise tool %q has no exact resolution", tool)
}

func reviewableMiseSelector(value string) bool {
	return isExactToolVersion(value) ||
		numericMiseSelectorPattern.MatchString(value)
}

func minimumMiseVersion(value string) bool {
	return minimumMiseVersionPattern.MatchString(strings.TrimSpace(value))
}

func miseSelectorMatches(selector, version string) bool {
	selector = strings.TrimPrefix(strings.TrimSpace(selector), "v")
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if isExactToolVersion(selector) {
		return selector == version
	}
	selectorParts := strings.Split(selector, ".")
	versionParts := strings.SplitN(version, "-", 2)
	exactParts := strings.Split(versionParts[0], ".")
	if len(selectorParts) >= len(exactParts) {
		return false
	}
	for index := range selectorParts {
		if selectorParts[index] != exactParts[index] {
			return false
		}
	}
	return true
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func rustToolchainVersion(data []byte) (string, error) {
	var document struct {
		Toolchain struct {
			Channel string `toml:"channel"`
		} `toml:"toolchain"`
	}
	if err := toml.Unmarshal(data, &document); err != nil {
		return "", err
	}
	return document.Toolchain.Channel, nil
}

func miseVersion(root, tool string, rule Rule) (string, *Finding) {
	data, err := readRepositoryFile(root, "mise.toml")
	if err != nil {
		finding := missingOrInput(rule, "mise.toml", err, "Mise configuration is required for "+tool+".")
		return "", &finding
	}
	config, err := parseMiseConfig(data)
	if err != nil {
		finding := inputError(rule, "mise.toml")
		return "", &finding
	}
	lockData, err := readRepositoryFile(root, "mise.lock")
	if err != nil {
		finding := missingOrInput(rule, "mise.lock", err, "Mise lock is required to resolve "+tool+" to an exact release.")
		return "", &finding
	}
	lock, err := parseMiseLock(lockData)
	if err != nil {
		finding := inputError(rule, "mise.lock")
		return "", &finding
	}
	version, err := resolveMiseToolVersion(config, lock.Tools, tool)
	if err != nil {
		finding := baseFinding(rule, deviationStatus(rule), "mise.toml", "Mise must resolve "+tool+" through mise.lock to one exact release.")
		return "", &finding
	}
	return version, nil
}

func repositoryFiles(root, directory string, limit int, extensions ...string) ([]string, error) {
	base := filepath.Join(root, filepath.FromSlash(directory))
	info, err := os.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("unsafe repository directory")
	}
	var result []string
	err = filepath.WalkDir(base, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		for _, extension := range extensions {
			if strings.HasSuffix(entry.Name(), extension) {
				relative, relErr := filepath.Rel(root, current)
				if relErr != nil {
					return relErr
				}
				result = append(result, filepath.ToSlash(relative))
				if len(result) > limit {
					return fmt.Errorf("repository file limit exceeded")
				}
				break
			}
		}
		return nil
	})
	slices.Sort(result)
	return result, err
}

func repositoryDirectoryFiles(root, directory string, limit int, extensions ...string) ([]string, error) {
	if directory == "" {
		directory = "."
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	rootFS, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rootFS.Close() }()
	entries, err := fs.ReadDir(rootFS.FS(), directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		for _, extension := range extensions {
			if strings.HasSuffix(entry.Name(), extension) {
				result = append(result, path.Join(directory, entry.Name()))
				if len(result) > limit {
					return nil, fmt.Errorf("repository file limit exceeded")
				}
				break
			}
		}
	}
	slices.Sort(result)
	return result, nil
}

func isExactToolVersion(value string) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	return exactPatchVersion(value) && !floatingPattern.MatchString(value)
}

func exactPatchVersion(value string) bool {
	return regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`).MatchString(value)
}

func versionParts(value string) []int {
	value = strings.TrimPrefix(value, "v")
	value = strings.SplitN(value, "-", 2)[0]
	parts := strings.Split(value, ".")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		result = append(result, number)
	}
	return result
}

func compareNumericVersions(left, right string) int {
	leftParts := versionParts(left)
	rightParts := versionParts(right)
	limit := max(len(leftParts), len(rightParts))
	for index := range limit {
		var leftPart, rightPart int
		if index < len(leftParts) {
			leftPart = leftParts[index]
		}
		if index < len(rightParts) {
			rightPart = rightParts[index]
		}
		switch {
		case leftPart < rightPart:
			return -1
		case leftPart > rightPart:
			return 1
		}
	}
	return 0
}

func isDirectReference(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "git+") ||
		strings.HasPrefix(lower, "github:") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.Contains(lower, " @ git+") ||
		strings.Contains(lower, " @ http://") ||
		strings.Contains(lower, " @ https://")
}

func directReferenceIsImmutable(value string) bool {
	value = strings.TrimSpace(value)
	if marker := strings.LastIndex(value, " @ "); marker >= 0 {
		value = strings.TrimSpace(value[marker+3:])
	}
	value = stripPEP508EnvironmentMarker(value)
	if immutableDigestValue(value) || fullCommitOnlyPattern.MatchString(value) {
		return true
	}

	gitGetterReference := strings.HasPrefix(value, "git::")
	candidate := strings.TrimPrefix(value, "git::")
	if gitGetterReference || strings.HasPrefix(candidate, "git@") {
		candidate = normalizeSCPStyleGitURL(candidate)
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return false
	}
	if immutableFragment(parsed.Fragment) {
		return true
	}
	for name, values := range parsed.Query() {
		lowerName := strings.ToLower(name)
		for _, queryValue := range values {
			switch lowerName {
			case "ref", "rev", "revision", "commit":
				if fullCommitOnlyPattern.MatchString(queryValue) {
					return true
				}
			case "sha256", "sha512", "checksum", "integrity", "hash":
				if immutableNamedDigest(lowerName, queryValue) {
					return true
				}
			}
		}
	}

	pathValue := parsed.Path
	if parsed.Opaque != "" {
		pathValue = parsed.Opaque
	}
	if at := strings.LastIndex(pathValue, "@"); at >= 0 && fullCommitOnlyPattern.MatchString(pathValue[at+1:]) {
		return true
	}
	return false
}

func normalizeSCPStyleGitURL(value string) string {
	if strings.Contains(value, "://") {
		return value
	}
	address := value
	suffix := ""
	if index := strings.IndexAny(address, "?#"); index >= 0 {
		address, suffix = address[:index], address[index:]
	}
	separator := strings.Index(address, ":")
	if separator <= 0 || separator == len(address)-1 || strings.Contains(address[:separator], "/") {
		return value
	}
	return "ssh://" + address[:separator] + "/" + address[separator+1:] + suffix
}

func stripPEP508EnvironmentMarker(value string) string {
	for index := 1; index < len(value); index++ {
		if value[index] == ';' && (value[index-1] == ' ' || value[index-1] == '\t') {
			return strings.TrimSpace(value[:index])
		}
	}
	return value
}

func immutableFragment(value string) bool {
	if fullCommitOnlyPattern.MatchString(value) || immutableDigestValue(value) {
		return true
	}
	values, err := url.ParseQuery(value)
	if err != nil {
		return false
	}
	for name, candidates := range values {
		lowerName := strings.ToLower(name)
		for _, candidate := range candidates {
			switch lowerName {
			case "ref", "rev", "revision", "commit":
				if fullCommitOnlyPattern.MatchString(candidate) {
					return true
				}
			case "sha256", "sha512", "checksum", "integrity", "hash":
				if immutableNamedDigest(lowerName, candidate) {
					return true
				}
			}
		}
	}
	return false
}

func immutableNamedDigest(name, value string) bool {
	if immutableDigestValue(value) {
		return true
	}
	switch name {
	case "sha256":
		return sha256ValuePattern.MatchString(value)
	case "sha512":
		return sha512ValuePattern.MatchString(value)
	default:
		return false
	}
}

func immutableDigestValue(value string) bool {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "sha256:") || strings.HasPrefix(lower, "sha256="):
		return sha256ValuePattern.MatchString(value[len("sha256:"):])
	case strings.HasPrefix(lower, "sha512:") || strings.HasPrefix(lower, "sha512="):
		return sha512ValuePattern.MatchString(value[len("sha512:"):])
	case strings.HasPrefix(lower, "sha256-"):
		return decodedDigestLength(value[len("sha256-"):], 32)
	case strings.HasPrefix(lower, "sha512-"):
		return decodedDigestLength(value[len("sha512-"):], 64)
	default:
		return false
	}
}

func decodedDigestLength(value string, length int) bool {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	}
	return err == nil && len(decoded) == length
}

type tomlDirectReference struct {
	path      string
	immutable bool
}

func collectTOMLDirectReferences(document map[string]any) []tomlDirectReference {
	var references []tomlDirectReference
	scanTOMLDirectReferences(document, nil, false, &references)
	return references
}

func scanTOMLDirectReferences(value any, keys []string, dependencyContext bool, references *[]tomlDirectReference) {
	switch typed := value.(type) {
	case map[string]any:
		if dependencyContext {
			if reference, exists := directReferenceFromTOMLTable(typed); exists {
				*references = append(*references, tomlDirectReference{
					path:      strings.Join(keys, "."),
					immutable: reference,
				})
				return
			}
		}
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			lower := strings.ToLower(name)
			nextContext := dependencyContext ||
				strings.Contains(lower, "dependencies") ||
				lower == "sources" && strings.Contains(strings.ToLower(strings.Join(keys, ".")), "tool.uv")
			scanTOMLDirectReferences(typed[name], append(keys, name), nextContext, references)
		}
	case []any:
		for index, item := range typed {
			scanTOMLDirectReferences(item, append(keys, strconv.Itoa(index)), dependencyContext, references)
		}
	case string:
		if dependencyContext && isDirectReference(typed) {
			*references = append(*references, tomlDirectReference{
				path:      strings.Join(keys, "."),
				immutable: directReferenceIsImmutable(typed),
			})
		}
	}
}

func directReferenceFromTOMLTable(table map[string]any) (bool, bool) {
	var reference string
	for _, name := range []string{"git", "url"} {
		if value, ok := table[name].(string); ok {
			reference = value
			break
		}
	}
	if reference == "" {
		return false, false
	}
	if directReferenceIsImmutable(reference) {
		return true, true
	}
	for _, name := range []string{"rev", "revision", "commit"} {
		if value, ok := table[name].(string); ok && fullCommitOnlyPattern.MatchString(value) {
			return true, true
		}
	}
	for _, name := range []string{"sha256", "sha512", "checksum", "integrity", "hash"} {
		if value, ok := table[name].(string); ok {
			if directReferenceIsImmutable(value) ||
				name == "sha256" && sha256ValuePattern.MatchString(value) ||
				name == "sha512" && sha512ValuePattern.MatchString(value) {
				return true, true
			}
		}
	}
	return false, true
}

func immutableActionReference(reference string) bool {
	if strings.HasPrefix(reference, "docker://") {
		return strings.Contains(reference, "@sha256:") && digestPattern.MatchString(reference)
	}
	at := strings.LastIndex(reference, "@")
	if at < 0 {
		return false
	}
	return fullCommitOnlyPattern.MatchString(reference[at+1:])
}

func immutableImageReference(reference string) bool {
	return digestPattern.MatchString(reference)
}

var (
	floatingPattern            = regexp.MustCompile(`(?i)(?:^|[-./])(?:latest|stable|master|main|nightly)(?:$|[-./])`)
	fullCommitOnlyPattern      = regexp.MustCompile(`(?i)^[a-f0-9]{40}$`)
	digestPattern              = regexp.MustCompile(`(?i)@sha256:[a-f0-9]{64}$`)
	sha256ValuePattern         = regexp.MustCompile(`(?i)^[a-f0-9]{64}$`)
	sha512ValuePattern         = regexp.MustCompile(`(?i)^[a-f0-9]{128}$`)
	numericMiseSelectorPattern = regexp.MustCompile(
		`^[0-9]+(?:\.[0-9]+)?$`,
	)
	minimumMiseVersionPattern = regexp.MustCompile(
		`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`,
	)
	pnpmExactPattern = regexp.MustCompile(`^pnpm@[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
	calverPattern    = regexp.MustCompile(`^[0-9]{4}\.(0[1-9]|1[0-2])(?:\.[1-9][0-9]*)?$`)
	semverPattern    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)
