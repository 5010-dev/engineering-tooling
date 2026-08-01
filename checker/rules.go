package checker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
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
		return baseFinding(rule, "skip", ".", "Rule is retired in this standard snapshot.")
	}
	if rule.ID != "DT-META-001" && metadata.Applicability.Status == "not-applicable" {
		return baseFinding(rule, "skip", ".", "Repository declares Golden Path as not applicable.")
	}
	if !ruleApplies(metadata, rule) {
		return baseFinding(rule, "skip", ".", "Rule does not apply to the declared profiles, artifact types, or capabilities.")
	}
	if rule.Assessment != "automated" {
		return baseFinding(rule, "skip", ".", "Rule requires hybrid or manual evidence and was not asserted by the structural checker.")
	}
	if componentScopedRules[rule.ID] && len(metadata.Components) > 0 {
		return evaluateComponentRule(root, metadata, rule, exceptions, exceptionsPresent, evaluatedAt, compatibility)
	}
	return evaluateAutomatedRule(root, metadata, rule, exceptionsPresent, evaluatedAt, compatibility)
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
		return baseFinding(rule, "skip", ".", "Rule does not apply to any declared component.")
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

func validateProfileRoot(root string, metadata Metadata) (string, string) {
	markers := []nativeProfileMarker{
		{paths: []string{"package.json"}, profiles: []string{"node-typescript"}},
		{paths: []string{"pyproject.toml"}, profiles: []string{"python"}},
		{paths: []string{"go.mod", "go.work"}, profiles: []string{"go"}},
		{paths: []string{"Cargo.toml"}, profiles: []string{"rust"}},
		{paths: []string{"build.zig", "build.zig.zon"}, profiles: []string{"zig", "zig-toolchain"}},
	}
	for _, marker := range markers {
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

var justRecipePattern = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_-]*)(?:\s+[^:\n]*)?\s*:(?:[ \t]|$)`)
var justImportPattern = regexp.MustCompile(`(?m)^\s*import(\?)?\s+["']([^"']+)["']\s*(?:#.*)?$`)

func parseJustRecipes(value string) map[string]bool {
	result := map[string]bool{}
	for _, match := range justRecipePattern.FindAllStringSubmatch(value, -1) {
		result[match[1]] = true
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
	for _, match := range justImportPattern.FindAllStringSubmatch(string(data), -1) {
		importName := path.Clean(path.Join(path.Dir(name), match[2]))
		imported, err := readRepositoryFile(root, importName)
		if errors.Is(err, errNotFound) && match[1] == "?" {
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
		{"go", []string{"go.mod"}},
		{"rust", []string{"Cargo.toml", "Cargo.lock"}},
		{"zig", []string{"build.zig", "build.zig.zon"}},
	}
	checked := false
	for _, requirement := range requirements {
		if !slices.Contains(metadata.Profiles, requirement.profile) {
			continue
		}
		checked = true
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
	for _, name := range []string{"pyproject.toml", "Cargo.toml"} {
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
	if !detected {
		return baseFinding(rule, "skip", ".", "No direct VCS, URL, archive, binary, or generated-source dependency was detected.")
	}
	return baseFinding(rule, "skip", ".", "Detected direct references are immutable, but ecosystem integrity-record correlation is outside this structural check.")
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
	for _, name := range files {
		data, readErr := readRepositoryFile(root, name)
		if readErr != nil {
			return inputError(rule, name)
		}
		for _, match := range usesPattern.FindAllStringSubmatch(string(data), -1) {
			reference := strings.Trim(match[1], `"'`)
			if strings.HasPrefix(reference, "./") {
				continue
			}
			remoteCount++
			if !immutableActionReference(reference) {
				finding := baseFinding(rule, deviationStatus(rule), name, "Remote executable reference is not pinned by full commit SHA or image digest.")
				finding.Secondary = reference
				return finding
			}
		}
		for _, match := range imagePattern.FindAllStringSubmatch(string(data), -1) {
			reference := strings.Trim(match[1], `"'`)
			remoteCount++
			if !immutableImageReference(reference) {
				finding := baseFinding(rule, deviationStatus(rule), name, "Container image reference is not pinned by SHA-256 digest.")
				finding.Secondary = reference
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
	goMod := componentFile(metadata, "go.mod")
	data, err := readRepositoryFile(root, goMod)
	if errors.Is(err, errNotFound) {
		goMod = componentFile(metadata, "go.work")
		data, err = readRepositoryFile(root, goMod)
	}
	if err != nil {
		return missingOrInput(rule, componentFile(metadata, "go.mod"), err, "Go profile requires go.mod or go.work.")
	}
	goLine := regexp.MustCompile(`(?m)^go\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)\s*$`).FindStringSubmatch(string(data))
	toolchainLine := regexp.MustCompile(`(?m)^toolchain\s+go([0-9]+\.[0-9]+\.[0-9]+)\s*$`).FindStringSubmatch(string(data))
	parts := versionParts(miseGo)
	if len(parts) != 3 || len(goLine) < 2 {
		return baseFinding(rule, deviationStatus(rule), goMod, "Go runtime declarations are incomplete.")
	}
	if compareNumericVersions(goLine[1], miseGo) > 0 {
		return baseFinding(rule, deviationStatus(rule), goMod, "The Go directive contradicts the mise-selected runtime.")
	}
	if len(toolchainLine) == 2 && toolchainLine[1] != miseGo {
		return baseFinding(rule, deviationStatus(rule), goMod, "The Go toolchain directive contradicts the mise-selected runtime.")
	}
	return baseFinding(rule, "pass", goMod, "Mise and native Go runtime declarations align.")
}

func evaluateGoDependencies(root string, metadata Metadata, rule Rule, _ bool) Finding {
	goMod := componentFile(metadata, "go.mod")
	data, err := readRepositoryFile(root, goMod)
	if err != nil {
		return missingOrInput(rule, goMod, err, "Go profile requires go.mod.")
	}
	hasThirdParty := regexp.MustCompile(`(?m)^\s*(?:require\s+)?(?:github\.com|gitlab\.com|golang\.org|gopkg\.in)/`).Match(data)
	if hasThirdParty {
		goSum := componentFile(metadata, "go.sum")
		if _, err := readRepositoryFile(root, goSum); err != nil {
			return missingOrInput(rule, goSum, err, "Go modules resolve third-party dependencies but go.sum is missing.")
		}
	}
	return baseFinding(rule, "skip", goMod, "Go dependency authority uses native records; tidy drift and go tool execution require the repository quality gate.")
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
	lower := strings.ToLower(value)
	return fullCommitPattern.MatchString(value) ||
		strings.Contains(lower, "sha256:") ||
		strings.Contains(lower, "sha256=") ||
		strings.Contains(lower, "sha512-") ||
		strings.Contains(lower, "sha512:") ||
		strings.Contains(lower, "sha512=")
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
	fullCommitPattern          = regexp.MustCompile(`(?i)[a-f0-9]{40}`)
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
	usesPattern      = regexp.MustCompile(`(?m)^\s*(?:-\s+)?uses:\s*([^\s#]+)`)
	imagePattern     = regexp.MustCompile(`(?m)^\s*image:\s*([^\s#]+)`)
	pnpmExactPattern = regexp.MustCompile(`^pnpm@[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
	calverPattern    = regexp.MustCompile(`^[0-9]{4}\.(0[1-9]|1[0-2])(?:\.[1-9][0-9]*)?$`)
	semverPattern    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)
