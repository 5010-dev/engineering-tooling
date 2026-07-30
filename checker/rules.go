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
	"DT-ZIG-001":     evaluateZigProfile,
	"DT-RELEASE-001": evaluateReleaseVersion,
}

func evaluateRule(root string, metadata Metadata, rule Rule, exceptionsPresent bool, evaluatedAt time.Time) Finding {
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
	if rule.ID == "DT-RUNTIME-001" {
		return evaluateRuntimeDisposition(root, metadata, rule, evaluatedAt)
	}
	evaluator, exists := evaluators[rule.ID]
	if !exists {
		return baseFinding(rule, "error", ".", "The checker release has no evaluator for an applicable automated rule.")
	}
	return evaluator(root, metadata, rule, exceptionsPresent)
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
	recipes, err := collectJustRecipes(root, name, data, map[string]bool{}, 0)
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

var justRecipePattern = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_-]*)(?:\s+[^:=\n]+)?\s*:`)
var justImportPattern = regexp.MustCompile(`(?m)^\s*import(\?)?\s+["']([^"']+)["']\s*$`)

func parseJustRecipes(value string) map[string]bool {
	result := map[string]bool{}
	for _, match := range justRecipePattern.FindAllStringSubmatch(value, -1) {
		result[match[1]] = true
	}
	return result
}

func collectJustRecipes(root, name string, data []byte, seen map[string]bool, depth int) (map[string]bool, error) {
	if depth > 32 {
		return nil, fmt.Errorf("just import depth exceeds limit")
	}
	name = path.Clean(filepath.ToSlash(name))
	if name == ".." || strings.HasPrefix(name, "../") {
		return nil, fmt.Errorf("just import escapes repository root")
	}
	if seen[name] {
		return map[string]bool{}, nil
	}
	seen[name] = true
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
		importedRecipes, err := collectJustRecipes(root, importName, imported, seen, depth+1)
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
		for name, value := range parseMiseTools(string(data)) {
			selectors = append(selectors, name+"="+value)
			if !isExactToolVersion(value) {
				return baseFinding(rule, deviationStatus(rule), "mise.toml", "Mise tool "+name+" does not use an exact version.")
			}
		}
	} else if !errors.Is(err, errNotFound) {
		return inputError(rule, "mise.toml")
	}
	if slices.Contains(metadata.Profiles, "python") {
		data, err := readRepositoryFile(root, ".python-version")
		if err == nil {
			value := strings.TrimSpace(string(data))
			selectors = append(selectors, "python="+value)
			if !exactPatchVersion(value) {
				return baseFinding(rule, deviationStatus(rule), ".python-version", "Python is not pinned to an exact patch release.")
			}
		} else if !errors.Is(err, errNotFound) {
			return inputError(rule, ".python-version")
		}
	}
	if data, err := readRepositoryFile(root, "rust-toolchain.toml"); err == nil {
		value := tomlString(string(data), "channel")
		selectors = append(selectors, "rust="+value)
		if !exactPatchVersion(value) {
			return baseFinding(rule, deviationStatus(rule), "rust-toolchain.toml", "Rust is not pinned to an exact release.")
		}
	} else if !errors.Is(err, errNotFound) {
		return inputError(rule, "rust-toolchain.toml")
	}
	if len(selectors) == 0 {
		return baseFinding(rule, "skip", ".", "No runtime or repository tool selector was detected.")
	}
	return baseFinding(rule, "pass", ".", "Detected runtime and repository tool selectors are exact.")
}

func evaluateMiseLock(root string, _ Metadata, rule Rule, _ bool) Finding {
	_, err := readRepositoryFile(root, "mise.toml")
	if errors.Is(err, errNotFound) {
		return baseFinding(rule, "skip", "mise.toml", "Mise does not manage repository tools.")
	}
	if err != nil {
		return inputError(rule, "mise.toml")
	}
	data, err := readRepositoryFile(root, "mise.toml")
	if err != nil {
		return inputError(rule, "mise.toml")
	}
	minVersion := tomlString(string(data), "min_version")
	if !exactPatchVersion(minVersion) {
		return baseFinding(rule, deviationStatus(rule), "mise.toml", "Mise configuration does not declare an exact minimum mise version.")
	}
	if _, err := readRepositoryFile(root, "mise.lock"); errors.Is(err, errNotFound) {
		return baseFinding(rule, deviationStatus(rule), "mise.lock", "Mise manages tools but mise.lock is missing.")
	} else if err != nil {
		return inputError(rule, "mise.lock")
	}
	return baseFinding(rule, "pass", "mise.lock", "Mise configuration and lock are committed with an exact minimum version.")
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
			if _, err := readRepositoryFile(root, name); errors.Is(err, errNotFound) {
				return baseFinding(rule, deviationStatus(rule), name, "The "+requirement.profile+" profile is missing a required native manifest or resolution record.")
			} else if err != nil {
				return inputError(rule, name)
			}
		}
	}
	if !checked {
		return baseFinding(rule, "skip", ".", "The declared profiles do not resolve third-party dependencies through a supported native record.")
	}
	return baseFinding(rule, "pass", ".", "Required profile-native manifests and resolution records are present.")
}

func evaluateDirectDependencies(root string, _ Metadata, rule Rule, _ bool) Finding {
	var detected bool
	if data, err := readRepositoryFile(root, "package.json"); err == nil {
		var manifest map[string]any
		if json.Unmarshal(data, &manifest) != nil {
			return baseFinding(rule, "error", "package.json", "package.json could not be decoded for direct dependency validation.")
		}
		for _, section := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
			dependencies, _ := manifest[section].(map[string]any)
			for name, raw := range dependencies {
				value, _ := raw.(string)
				if isDirectReference(value) {
					detected = true
					if !directReferenceIsImmutable(value) {
						finding := baseFinding(rule, deviationStatus(rule), "package.json", "Direct dependency "+name+" is not pinned to an immutable reference with integrity.")
						finding.Secondary = name
						return finding
					}
				}
			}
		}
	} else if !errors.Is(err, errNotFound) {
		return inputError(rule, "package.json")
	}
	for _, name := range []string{"pyproject.toml", "Cargo.toml"} {
		data, err := readRepositoryFile(root, name)
		if errors.Is(err, errNotFound) {
			continue
		}
		if err != nil {
			return inputError(rule, name)
		}
		for _, line := range strings.Split(string(data), "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "git =") || strings.Contains(lower, "url =") {
				detected = true
				if !fullCommitPattern.MatchString(line) && !strings.Contains(lower, "sha256") && !strings.Contains(lower, "sha512") {
					return baseFinding(rule, deviationStatus(rule), name, "A direct dependency does not declare an immutable commit or integrity digest.")
				}
			}
		}
	}
	if !detected {
		return baseFinding(rule, "skip", ".", "No direct VCS, URL, archive, binary, or generated-source dependency was detected.")
	}
	return baseFinding(rule, "skip", ".", "Detected direct references are immutable, but ecosystem integrity-record correlation is not yet complete in checker 0.x.")
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
	}
	if remoteCount == 0 {
		return baseFinding(rule, "skip", ".github/workflows", "No remote executable workflow reference was detected.")
	}
	return baseFinding(rule, "pass", ".github/workflows", "Remote workflow and action references are immutable.")
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

func evaluateRuntimeDisposition(root string, metadata Metadata, rule Rule, evaluatedAt time.Time) Finding {
	symbolicDisposition := false
	for _, profile := range metadata.Profiles {
		switch profile {
		case "node-typescript":
			version, finding := miseVersion(root, "node", rule)
			if finding != nil {
				return *finding
			}
			major := firstVersionPart(version)
			if major != 22 && major != 24 {
				return baseFinding(rule, deviationStatus(rule), "mise.toml", "Selected Node.js runtime is blocked by runtime-support/v1.")
			}
		case "python":
			data, err := readRepositoryFile(root, ".python-version")
			if err != nil {
				return missingOrInput(rule, ".python-version", err, "Python runtime selector is missing.")
			}
			parts := versionParts(strings.TrimSpace(string(data)))
			if len(parts) != 3 || parts[0] != 3 || parts[1] < 10 || parts[1] > 14 {
				return baseFinding(rule, deviationStatus(rule), ".python-version", "Selected Python runtime is not supported by runtime-support/v1.")
			}
			if parts[1] == 10 {
				if evaluatedAt.UTC().Format("2006-01-02") > "2026-10-31" {
					return baseFinding(rule, deviationStatus(rule), ".python-version", "Selected Python compatibility-only runtime is past its snapshot support deadline.")
				}
			}
		case "go":
			version, finding := miseVersion(root, "go", rule)
			if finding != nil {
				return *finding
			}
			parts := versionParts(version)
			if len(parts) != 3 || parts[0] != 1 || parts[1] < 25 || parts[1] > 26 {
				return baseFinding(rule, deviationStatus(rule), "mise.toml", "Selected Go runtime is blocked by runtime-support/v1.")
			}
		case "rust":
			symbolicDisposition = true
		case "zig", "zig-toolchain":
			version, finding := miseVersion(root, "zig", rule)
			if finding != nil {
				return *finding
			}
			if version != "0.16.0" {
				return baseFinding(rule, deviationStatus(rule), "mise.toml", "Selected Zig runtime is not the organization-approved exact tagged baseline.")
			}
		}
	}
	if symbolicDisposition {
		return baseFinding(rule, "skip", ".", "The bundled runtime catalog uses a coordinated symbolic Rust selector; exact disposition mapping is not yet present in the 0.x compatibility manifest.")
	}
	return baseFinding(rule, "pass", ".", "Selected language runtime lines have an allowed organization disposition.")
}

func evaluateNodeProfile(root string, _ Metadata, rule Rule, _ bool) Finding {
	if _, finding := miseVersion(root, "node", rule); finding != nil {
		return *finding
	}
	data, err := readRepositoryFile(root, "package.json")
	if err != nil {
		return missingOrInput(rule, "package.json", err, "Node profile requires package.json.")
	}
	var manifest struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return baseFinding(rule, "error", "package.json", "package.json could not be decoded.")
	}
	if !pnpmExactPattern.MatchString(manifest.PackageManager) {
		return baseFinding(rule, deviationStatus(rule), "package.json", "packageManager must pin an exact pnpm version.")
	}
	if _, err := readRepositoryFile(root, "pnpm-lock.yaml"); err != nil {
		return missingOrInput(rule, "pnpm-lock.yaml", err, "Node profile requires pnpm-lock.yaml.")
	}
	return baseFinding(rule, "skip", "package.json", "Node.js and pnpm authority is exact and pnpm-lock.yaml is present; frozen-install CI semantics require the repository quality gate.")
}

func evaluatePythonProfile(root string, _ Metadata, rule Rule, _ bool) Finding {
	for _, name := range []string{"pyproject.toml", "uv.lock", ".python-version"} {
		if _, err := readRepositoryFile(root, name); err != nil {
			return missingOrInput(rule, name, err, "Python profile requires "+name+".")
		}
	}
	data, _ := readRepositoryFile(root, ".python-version")
	if !exactPatchVersion(strings.TrimSpace(string(data))) {
		return baseFinding(rule, deviationStatus(rule), ".python-version", "Python must be pinned to an exact patch version.")
	}
	if miseData, err := readRepositoryFile(root, "mise.toml"); err == nil {
		if _, exists := parseMiseTools(string(miseData))["python"]; exists {
			return baseFinding(rule, deviationStatus(rule), "mise.toml", "Uv, not mise, must own the Python runtime.")
		}
	}
	return baseFinding(rule, "skip", "pyproject.toml", "Uv owns exact Python runtime and dependency records; locked-sync CI semantics require the repository quality gate.")
}

func evaluateGoAuthority(root string, _ Metadata, rule Rule, _ bool) Finding {
	miseGo, finding := miseVersion(root, "go", rule)
	if finding != nil {
		return *finding
	}
	data, err := readRepositoryFile(root, "go.mod")
	if errors.Is(err, errNotFound) {
		data, err = readRepositoryFile(root, "go.work")
	}
	if err != nil {
		return missingOrInput(rule, "go.mod", err, "Go profile requires go.mod or go.work.")
	}
	goLine := regexp.MustCompile(`(?m)^go\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)\s*$`).FindStringSubmatch(string(data))
	toolchainLine := regexp.MustCompile(`(?m)^toolchain\s+go([0-9]+\.[0-9]+\.[0-9]+)\s*$`).FindStringSubmatch(string(data))
	parts := versionParts(miseGo)
	if len(parts) != 3 || len(goLine) < 2 {
		return baseFinding(rule, deviationStatus(rule), "go.mod", "Go runtime declarations are incomplete.")
	}
	expectedMinor := strconv.Itoa(parts[0]) + "." + strconv.Itoa(parts[1])
	if goLine[1] != expectedMinor && goLine[1] != miseGo {
		return baseFinding(rule, deviationStatus(rule), "go.mod", "The Go directive contradicts the mise-selected runtime.")
	}
	if len(toolchainLine) == 2 && toolchainLine[1] != miseGo {
		return baseFinding(rule, deviationStatus(rule), "go.mod", "The Go toolchain directive contradicts the mise-selected runtime.")
	}
	return baseFinding(rule, "pass", "go.mod", "Mise and native Go runtime declarations align.")
}

func evaluateGoDependencies(root string, _ Metadata, rule Rule, _ bool) Finding {
	data, err := readRepositoryFile(root, "go.mod")
	if err != nil {
		return missingOrInput(rule, "go.mod", err, "Go profile requires go.mod.")
	}
	hasThirdParty := regexp.MustCompile(`(?m)^\s*(?:require\s+)?(?:github\.com|gitlab\.com|golang\.org|gopkg\.in)/`).Match(data)
	if hasThirdParty {
		if _, err := readRepositoryFile(root, "go.sum"); err != nil {
			return missingOrInput(rule, "go.sum", err, "Go modules resolve third-party dependencies but go.sum is missing.")
		}
	}
	return baseFinding(rule, "skip", "go.mod", "Go dependency authority uses native records; tidy drift and go tool execution require the repository quality gate.")
}

func evaluateRustProfile(root string, _ Metadata, rule Rule, _ bool) Finding {
	data, err := readRepositoryFile(root, "rust-toolchain.toml")
	if err != nil {
		return missingOrInput(rule, "rust-toolchain.toml", err, "Rust profile requires rust-toolchain.toml.")
	}
	channel := tomlString(string(data), "channel")
	if !exactPatchVersion(channel) {
		return baseFinding(rule, deviationStatus(rule), "rust-toolchain.toml", "Rust toolchain channel must be an exact release.")
	}
	if mise, err := readRepositoryFile(root, "mise.toml"); err == nil {
		if _, exists := parseMiseTools(string(mise))["rust"]; exists {
			return baseFinding(rule, deviationStatus(rule), "mise.toml", "Rustup must be the sole Rust toolchain owner.")
		}
	}
	return baseFinding(rule, "pass", "rust-toolchain.toml", "Rustup owns an exact Rust toolchain release.")
}

func evaluateZigProfile(root string, _ Metadata, rule Rule, _ bool) Finding {
	version, finding := miseVersion(root, "zig", rule)
	if finding != nil {
		return *finding
	}
	if version != "0.16.0" {
		return baseFinding(rule, deviationStatus(rule), "mise.toml", "Zig must use an exact tagged stable release.")
	}
	return baseFinding(rule, "pass", "mise.toml", "Zig is conditionally declared and pinned to an exact tagged release.")
}

func evaluateReleaseVersion(_ string, metadata Metadata, rule Rule, _ bool) Finding {
	if !calverPattern.MatchString(metadata.StandardVersion) || !semverPattern.MatchString(metadata.AssetBundleVersion) {
		return baseFinding(rule, deviationStatus(rule), ".github/golden-path.yaml", "Standard or asset bundle version uses the wrong version scheme.")
	}
	return baseFinding(rule, "pass", ".github/golden-path.yaml", "Golden Path standard uses CalVer and the executable asset bundle uses SemVer.")
}

func inputError(rule Rule, name string) Finding {
	return baseFinding(rule, "error", name, "A bounded repository input could not be read safely.")
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

func parseMiseTools(value string) map[string]string {
	result := map[string]string{}
	inTools := false
	for _, rawLine := range strings.Split(value, "\n") {
		line := strings.TrimSpace(strings.SplitN(rawLine, "#", 2)[0])
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inTools = line == "[tools]"
			continue
		}
		if !inTools || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.Trim(strings.TrimSpace(parts[0]), `"'`)
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

func tomlString(value, key string) string {
	pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*["']([^"']+)["']\s*(?:#.*)?$`)
	match := pattern.FindStringSubmatch(value)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func miseVersion(root, tool string, rule Rule) (string, *Finding) {
	data, err := readRepositoryFile(root, "mise.toml")
	if err != nil {
		finding := missingOrInput(rule, "mise.toml", err, "Mise configuration is required for "+tool+".")
		return "", &finding
	}
	version := parseMiseTools(string(data))[tool]
	if !isExactToolVersion(version) {
		finding := baseFinding(rule, deviationStatus(rule), "mise.toml", "Mise must pin "+tool+" to an exact release.")
		return "", &finding
	}
	return strings.TrimPrefix(version, "v"), nil
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

func firstVersionPart(value string) int {
	parts := versionParts(value)
	if len(parts) == 0 {
		return -1
	}
	return parts[0]
}

func isDirectReference(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "git+") ||
		strings.HasPrefix(lower, "github:") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://")
}

func directReferenceIsImmutable(value string) bool {
	lower := strings.ToLower(value)
	return fullCommitPattern.MatchString(value) ||
		strings.Contains(lower, "sha256:") ||
		strings.Contains(lower, "sha512-")
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

var (
	floatingPattern       = regexp.MustCompile(`(?i)(?:^|[-./])(?:latest|stable|master|main|nightly)(?:$|[-./])`)
	fullCommitPattern     = regexp.MustCompile(`(?i)[a-f0-9]{40}`)
	fullCommitOnlyPattern = regexp.MustCompile(`(?i)^[a-f0-9]{40}$`)
	digestPattern         = regexp.MustCompile(`(?i)@sha256:[a-f0-9]{64}$`)
	usesPattern           = regexp.MustCompile(`(?m)^\s*uses:\s*([^\s#]+)`)
	pnpmExactPattern      = regexp.MustCompile(`^pnpm@[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
	calverPattern         = regexp.MustCompile(`^[0-9]{4}\.(0[1-9]|1[0-2])(?:\.[1-9][0-9]*)?$`)
	semverPattern         = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)
