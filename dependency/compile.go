package dependency

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/5010-dev/engineering-tooling/internal/justsyntax"

	"gopkg.in/yaml.v3"
)

type dependabotUpdate struct {
	PackageEcosystem      string `yaml:"package-ecosystem"`
	Directory             string `yaml:"directory"`
	TargetBranch          string `yaml:"target-branch"`
	OpenPullRequestsLimit *int   `yaml:"open-pull-requests-limit"`
}

type dependabotDocument struct {
	Version int                `yaml:"version"`
	Updates []dependabotUpdate `yaml:"updates"`
}

type rootSurface struct {
	RootID       string
	Ecosystem    string
	Directory    string
	TargetBranch string
	Budget       int
	Pending      bool
	Reason       string
}

var nonIdentifier = regexp.MustCompile(`[^a-z0-9]+`)

const dependabotNativeDefaultPRBudget = 5
const maxJustImportFiles = 128

// Evaluate validates repository facts and compiles bounded adapter output. It
// never executes the referenced canonical gate or mutates the repository.
func Evaluate(root string) Evaluation {
	result := Evaluation{
		SchemaVersion: "golden-path-dependency-evaluation/v1",
		Complete:      false,
		ExitCode:      0,
		Roots:         []EffectiveRoot{},
		Findings:      []Finding{},
		RenderedFiles: map[string][]byte{},
	}
	policy, _, err := loadPolicy(root)
	if err != nil {
		message := "Dependency policy is missing."
		if !errors.Is(err, errInputNotFound) {
			message = "Dependency policy is invalid: " + err.Error()
		}
		return configurationEvaluation(result, ".github/golden-path-dependency-policy.yaml", message)
	}
	if policy.Adapter != "dependabot" {
		return configurationEvaluation(result, ".github/golden-path-dependency-policy.yaml", "This compiler release supports the default Dependabot adapter; a Renovate selection must not be interpreted as Dependabot configuration.")
	}
	metadata, err := loadMetadataInput(root)
	if err != nil {
		return configurationEvaluation(result, ".github/golden-path.yaml", "Golden Path metadata is unavailable: "+err.Error())
	}
	if !contains(metadata.Capabilities, "dependency-automation") {
		result.Complete = true
		result.Findings = append(result.Findings, Finding{
			RuleID: "DT-DEP-005", Status: "skip", Severity: "info", Path: ".github/golden-path.yaml",
			Message: "Dependency automation capability is not declared.", Remediation: "No dependency policy is required for this repository.",
		})
		return result
	}
	if finding, err := securityClosureReferenceFinding(root, policy.Defaults.SecurityFallback, "defaults"); err != nil {
		return configurationEvaluationForRule(result, "DT-DEP-012", ".github/golden-path-dependency-policy.yaml", "Default security closure reference is invalid: "+err.Error())
	} else if finding != nil {
		result.Findings = append(result.Findings, *finding)
	}
	nativeRoots, explicitRoots, err := loadNativeRootsInput(root)
	if err != nil {
		return configurationEvaluation(result, ".github/golden-path-native-roots.yaml", "Native-root authority is invalid: "+err.Error())
	}
	units, err := loadReleaseUnits(root, policy.ReleaseUnits.Path)
	if err != nil {
		classified := false
		for _, binding := range policy.RootBindings {
			if binding.ClassificationState == "classified" {
				classified = true
				break
			}
		}
		if classified || !errors.Is(err, errInputNotFound) {
			return configurationEvaluation(result, policy.ReleaseUnits.Path, "Release-unit authority is unavailable for a classified root: "+err.Error())
		}
		units = map[string]releaseUnit{}
	}

	bindingByID := make(map[string]RootBinding, len(policy.RootBindings))
	for _, binding := range policy.RootBindings {
		if _, duplicate := bindingByID[binding.NativeRootRef]; duplicate {
			return configurationEvaluation(result, ".github/golden-path-dependency-policy.yaml", "Root bindings must have unique nativeRootRef values.")
		}
		bindingByID[binding.NativeRootRef] = binding
	}
	knownRoots := make(map[string]bool, len(nativeRoots))
	surfaces := make([]rootSurface, 0)
	for _, nativeRoot := range nativeRoots {
		knownRoots[nativeRoot.ID] = true
		binding, present := bindingByID[nativeRoot.ID]
		if !present {
			binding = RootBinding{NativeRootRef: nativeRoot.ID, ClassificationState: "pending-classification", Reason: "root has no dependency policy binding"}
		}
		ecosystems := ecosystemsForProfiles(nativeRoot.Profiles)
		if len(ecosystems) > 1 {
			return configurationEvaluation(result, ".github/golden-path-native-roots.yaml", "Native root "+nativeRoot.ID+" resolves to multiple adapter ecosystems ("+strings.Join(ecosystems, ", ")+"); declare separate existing native roots, which may share the same path.")
		}
		effective, finding := compileRoot(root, nativeRoot, binding, policy.Defaults, units)
		if finding != nil {
			return configurationEvaluation(result, finding.Path, finding.Message)
		}
		if binding.Security != nil {
			closureFinding, closureErr := securityClosureReferenceFinding(root, effective.Security, nativeRoot.ID)
			if closureErr != nil {
				return configurationEvaluationForRule(result, "DT-DEP-012", ".github/golden-path-dependency-policy.yaml", "Security closure reference for root "+nativeRoot.ID+" is invalid: "+closureErr.Error())
			}
			if closureFinding != nil {
				result.Findings = append(result.Findings, *closureFinding)
			}
		}
		result.Roots = append(result.Roots, effective)
		for _, ecosystem := range ecosystems {
			surfaces = append(surfaces, rootSurface{
				RootID: nativeRoot.ID, Ecosystem: ecosystem, Directory: dependabotDirectory(nativeRoot.Path),
				TargetBranch: effective.ReleaseFlow.IntegrationBranch, Budget: effective.RoutinePRBudget,
				Pending: effective.Classification != "classified", Reason: effective.Reason,
			})
		}
	}
	for _, binding := range policy.RootBindings {
		if !knownRoots[binding.NativeRootRef] {
			return configurationEvaluation(result, ".github/golden-path-dependency-policy.yaml", "Root "+binding.NativeRootRef+" does not exist in the native-root authority.")
		}
	}

	dependabotData, err := readRegular(root, ".github/dependabot.yml")
	if err != nil {
		return configurationEvaluation(result, ".github/dependabot.yml", "Dependabot configuration is unavailable: "+err.Error())
	}
	var dependabot dependabotDocument
	if err := yaml.Unmarshal(dependabotData, &dependabot); err != nil || dependabot.Version != 2 {
		return configurationEvaluation(result, ".github/dependabot.yml", "Dependabot configuration is invalid.")
	}
	surfaceByKey := make(map[string]rootSurface, len(surfaces))
	for _, surface := range surfaces {
		key := surface.Ecosystem + "\x00" + surface.Directory
		if existing, duplicate := surfaceByKey[key]; duplicate && existing.RootID != surface.RootID {
			return configurationEvaluation(result, ".github/golden-path-native-roots.yaml", "Native roots "+existing.RootID+" and "+surface.RootID+" claim the same "+surface.Ecosystem+" adapter surface at "+surface.Directory+".")
		}
		surfaceByKey[key] = surface
	}
	limits := make(map[string]int, len(dependabot.Updates))
	targets := make(map[string]string, len(dependabot.Updates))
	seenUpdates := make(map[string]bool, len(dependabot.Updates))
	for _, update := range dependabot.Updates {
		key := update.PackageEcosystem + "\x00" + normalizedDependabotDirectory(update.Directory)
		if update.PackageEcosystem == "" || update.Directory == "" {
			return configurationEvaluation(result, ".github/dependabot.yml", "Dependabot updates must identify an ecosystem and directory.")
		}
		if seenUpdates[key] {
			return configurationEvaluation(result, ".github/dependabot.yml", "Dependabot declares the same ecosystem and directory more than once.")
		}
		seenUpdates[key] = true
		surface, matched := surfaceByKey[key]
		if !matched {
			surface = rootSurface{
				RootID:    "candidate-" + identifierPart(update.PackageEcosystem) + "-" + identifierPart(normalizedDependabotDirectory(update.Directory)),
				Ecosystem: update.PackageEcosystem, Directory: normalizedDependabotDirectory(update.Directory),
				TargetBranch: policy.Defaults.ReleaseFlow.IntegrationBranch, Pending: true,
				Reason: "adapter surface is not bound to an explicit native root",
			}
			result.Roots = append(result.Roots, EffectiveRoot{
				RootID: surface.RootID, Path: strings.TrimPrefix(surface.Directory, "/"), Profiles: []string{},
				Classification: "pending-classification", AffectedReleaseUnits: []string{}, ValidationOnly: []ValidationInvariant{},
				Owner: policy.Defaults.Owner, ReleaseFlow: policy.Defaults.ReleaseFlow, CanonicalGate: policy.Defaults.CanonicalGate,
				RoutinePRBudget: 0, Security: policy.Defaults.SecurityFallback, Reason: surface.Reason,
			})
		}
		desired := surface.Budget
		if surface.Pending {
			desired = 0
		}
		limits[key] = desired
		targets[key] = surface.TargetBranch
		actual := dependabotNativeDefaultPRBudget
		if update.OpenPullRequestsLimit != nil {
			actual = *update.OpenPullRequestsLimit
		}
		if actual < 0 {
			return configurationEvaluation(result, ".github/dependabot.yml", "Dependabot open-pull-requests-limit must be zero or a positive integer.")
		}
		if actual != desired {
			rule := "DT-DEP-007"
			message := fmt.Sprintf("Routine surface %s at %s uses open-pull-requests-limit %d; compiled value is %d.", update.PackageEcosystem, update.Directory, actual, desired)
			if surface.Pending {
				rule = "DT-DEP-005"
				message = fmt.Sprintf("Pending routine surface %s at %s must remain stopped with open-pull-requests-limit 0.", update.PackageEcosystem, update.Directory)
			}
			result.Findings = append(result.Findings, Finding{
				RuleID: rule, Status: "fail", Severity: "error", Path: ".github/dependabot.yml", Secondary: surface.RootID,
				Message: message, Remediation: "Apply the deterministic dependency candidate and review the repository-owned adapter diff.",
			})
		}
		if update.TargetBranch != surface.TargetBranch {
			result.Findings = append(result.Findings, Finding{
				RuleID: "DT-DEP-005", Status: "fail", Severity: "error", Path: ".github/dependabot.yml", Secondary: surface.RootID,
				Message:     fmt.Sprintf("Routine surface %s at %s targets %q; compiled integration branch is %q.", update.PackageEcosystem, update.Directory, update.TargetBranch, surface.TargetBranch),
				Remediation: "Apply the deterministic dependency candidate and review the repository-owned adapter diff.",
			})
		}
	}
	renderedDependabot, err := renderDependabot(dependabotData, limits, targets)
	if err != nil {
		return internalEvaluation(result, "Dependabot candidate rendering failed: "+err.Error())
	}
	result.RenderedFiles[".github/dependabot.yml"] = renderedDependabot
	if finding := duplicateAdapterFinding(root, dependabot.Updates); finding != nil {
		result.Findings = append(result.Findings, *finding)
	}

	if routerPath := existingSecurityRouter(root); routerPath != "" {
		data, readErr := readRegular(root, routerPath)
		if readErr != nil {
			return internalEvaluation(result, "Security router could not be read safely.")
		}
		result.RenderedFiles[routerPath] = data
	} else if requiresSecurityRouter(result.Roots) {
		flow := policy.Defaults.ReleaseFlow
		result.RenderedFiles[".github/workflows/dependency-security-router.yml"] = renderSecurityRouter(flow.ReleaseBranch, flow.IntegrationBranch)
	}

	result.Complete = true
	sortEvaluation(&result)
	if hasFail(result.Findings) {
		result.ExitCode = 1
	}
	if len(result.Findings) == 0 {
		result.Findings = append(result.Findings, Finding{
			RuleID: "DT-DEP-005", Status: "pass", Severity: "info", Path: ".github/golden-path-dependency-policy.yaml",
			Message: "Dependency policy and adapter output are semantically aligned.", Remediation: "No action required.",
		})
	}
	if !explicitRoots {
		result.Findings = append(result.Findings, Finding{
			RuleID: "DT-DEP-005", Status: "warn", Severity: "warning", Path: ".github/golden-path-native-roots.yaml",
			Message:     "No explicit native-root authority exists; discovered routine surfaces remain pending-classification.",
			Remediation: "Declare actual native roots only when repository facts are known; do not create dummy manifests or infer release impact.",
		})
	}
	sortEvaluation(&result)
	return result
}

func compileRoot(root string, nativeRoot NativeRoot, binding RootBinding, defaults RootPolicy, units map[string]releaseUnit) (EffectiveRoot, *Finding) {
	affected := make([]string, 0, len(binding.AffectedArtifacts))
	for _, artifact := range binding.AffectedArtifacts {
		affected = append(affected, artifact.ReleaseUnitRef)
	}
	effective := EffectiveRoot{
		RootID: nativeRoot.ID, Path: nativeRoot.Path, Profiles: sortedStrings(nativeRoot.Profiles),
		Classification: binding.ClassificationState, AffectedReleaseUnits: sortedStrings(affected),
		ValidationOnly: append([]ValidationInvariant(nil), binding.ValidationOnly...), Owner: defaults.Owner,
		ReleaseFlow: defaults.ReleaseFlow, CanonicalGate: defaults.CanonicalGate,
		RoutinePRBudget: effectiveBudget(defaults.Routine), Security: defaults.SecurityFallback, Reason: binding.Reason,
	}
	if binding.Owner != nil {
		effective.Owner = *binding.Owner
	}
	if binding.ReleaseFlow != nil {
		effective.ReleaseFlow = *binding.ReleaseFlow
	}
	if binding.CanonicalGate != nil {
		effective.CanonicalGate = *binding.CanonicalGate
	}
	if binding.Routine != nil {
		effective.RoutinePRBudget = effectiveBudget(*binding.Routine)
	}
	if binding.Security != nil {
		effective.Security = *binding.Security
	}
	if binding.Routine != nil && binding.Routine.BudgetOverride != nil && binding.Routine.MaxOpenPullRequests == 0 {
		return EffectiveRoot{}, &Finding{Path: ".github/golden-path-dependency-policy.yaml", Message: "A budgetOverride requires an explicit positive maxOpenPullRequests for root " + nativeRoot.ID + "."}
	}
	if effective.Classification != "classified" {
		effective.RoutinePRBudget = 0
		return effective, nil
	}
	for _, id := range effective.AffectedReleaseUnits {
		unit, exists := units[id]
		if !exists {
			return EffectiveRoot{}, &Finding{Path: ".github/golden-path-dependency-policy.yaml", Message: "Affected release unit " + id + " does not exist."}
		}
		if releaseUnitKind(unit) != "artifact" {
			return EffectiveRoot{}, &Finding{Path: ".github/golden-path-dependency-policy.yaml", Message: "Affected release unit " + id + " is not an artifact unit according to its profiles."}
		}
	}
	for _, invariant := range effective.ValidationOnly {
		unit, exists := units[invariant.InvariantReleaseUnitRef]
		if !exists || releaseUnitKind(unit) != invariant.Kind {
			return EffectiveRoot{}, &Finding{Path: ".github/golden-path-dependency-policy.yaml", Message: "Validation-only reference " + invariant.InvariantReleaseUnitRef + " does not match an existing " + invariant.Kind + " release unit."}
		}
	}
	if err := validateGate(root, effective.CanonicalGate); err != nil {
		return EffectiveRoot{}, &Finding{Path: ".github/golden-path-dependency-policy.yaml", Message: "Canonical gate for root " + nativeRoot.ID + " is invalid: " + err.Error()}
	}
	effective.GroupingKey = groupingKey(effective)
	return effective, nil
}

func effectiveBudget(policy RoutinePolicy) int {
	if policy.MaxOpenPullRequests > 0 {
		return policy.MaxOpenPullRequests
	}
	return DefaultPRBudget
}

func releaseUnitKind(unit releaseUnit) string {
	for _, profile := range unit.Profiles {
		switch profile {
		case "event-schema", "contract":
			return "contract"
		case "database-migration", "migration":
			return "migration"
		}
	}
	return "artifact"
}

func validateGate(root string, gate GateReference) error {
	workingDirectory := filepath.ToSlash(filepath.Clean(gate.Command.WorkingDirectory))
	if filepath.IsAbs(workingDirectory) || workingDirectory == ".." || strings.HasPrefix(workingDirectory, "../") {
		return fmt.Errorf("working directory escapes the repository")
	}
	justPath := "justfile"
	if workingDirectory != "." {
		justPath = workingDirectory + "/justfile"
	}
	defined, err := justRecipeDefined(root, justPath, "ci")
	if err != nil {
		return err
	}
	if !defined {
		return fmt.Errorf("referenced justfile import graph does not define recipe ci")
	}
	for _, evidence := range gate.CIEvidence {
		workflow, err := readRegular(root, evidence.Workflow)
		if err != nil {
			return fmt.Errorf("workflow %s is unavailable", evidence.Workflow)
		}
		var document struct {
			Jobs map[string]any `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(workflow, &document); err != nil || document.Jobs[evidence.Job] == nil {
			return fmt.Errorf("workflow %s does not define job %s", evidence.Workflow, evidence.Job)
		}
	}
	return nil
}

func justRecipeDefined(root, entrypoint, recipe string) (bool, error) {
	visited := map[string]bool{}
	recipeFound := false
	entrypoint = filepath.ToSlash(filepath.Clean(entrypoint))
	scheduled := map[string]bool{entrypoint: true}
	queue := []string{entrypoint}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if visited[path] {
			continue
		}
		if len(visited) >= maxJustImportFiles {
			return false, fmt.Errorf("referenced justfile import graph exceeds %d files", maxJustImportFiles)
		}
		visited[path] = true

		data, err := readRegular(root, path)
		if err != nil && path == entrypoint {
			alternate := strings.TrimSuffix(path, "justfile") + "Justfile"
			data, err = readRegular(root, alternate)
			if err == nil {
				path = alternate
			}
		}
		if err != nil {
			return false, fmt.Errorf("referenced justfile %s is unavailable", path)
		}

		if justsyntax.Recipes(string(data))[recipe] {
			recipeFound = true
		}
		for _, imported := range justsyntax.Imports(string(data)) {
			importInput := filepath.FromSlash(imported.Path)
			if filepath.IsAbs(importInput) {
				return false, fmt.Errorf("referenced justfile import %s escapes the repository", imported.Path)
			}
			importPath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(path), importInput)))
			if _, importErr := readRegular(root, importPath); importErr != nil {
				if imported.Optional && errors.Is(importErr, errInputNotFound) {
					continue
				}
				return false, fmt.Errorf("referenced justfile import %s is unavailable", importPath)
			}
			if !scheduled[importPath] {
				if len(scheduled) >= maxJustImportFiles {
					return false, fmt.Errorf("referenced justfile import graph exceeds %d files", maxJustImportFiles)
				}
				scheduled[importPath] = true
				queue = append(queue, importPath)
			}
		}
	}
	return recipeFound, nil
}

func securityClosureReferenceFinding(root string, route SecurityRoute, secondary string) (*Finding, error) {
	if route.CanonicalGate == nil {
		return nil, nil
	}
	if len(route.CanonicalGate.CIEvidence) == 0 {
		return &Finding{
			RuleID: "DT-DEP-012", Status: "fail", Severity: "error",
			Path: ".github/golden-path-dependency-policy.yaml", Secondary: secondary,
			Message:     "The security canonical gate does not reference a repository-owned conditional closure job.",
			Remediation: "Add the existing security closure workflow/job to canonicalGate.ciEvidence; central conformance validates only the reference and does not execute repository CI.",
		}, nil
	}
	if err := validateGate(root, *route.CanonicalGate); err != nil {
		return nil, err
	}
	return nil, nil
}

func groupingKey(root EffectiveRoot) string {
	invariants := make([]string, 0, len(root.ValidationOnly))
	for _, invariant := range root.ValidationOnly {
		invariants = append(invariants, invariant.Kind+":"+invariant.InvariantReleaseUnitRef)
	}
	sort.Strings(invariants)
	gate, _ := json.Marshal(root.CanonicalGate)
	return strings.Join([]string{root.RootID, strings.Join(root.AffectedReleaseUnits, ","), strings.Join(invariants, ","), sha256Digest(gate)}, "|")
}

func ecosystemsForProfiles(profiles []string) []string {
	values := map[string]string{
		"node-typescript": "npm", "python": "uv", "go": "gomod", "rust": "cargo",
		"infrastructure-terraform": "terraform", "infrastructure-opentofu": "terraform",
	}
	set := map[string]bool{}
	for _, profile := range profiles {
		if value := values[profile]; value != "" {
			set[value] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func dependabotDirectory(path string) string {
	if path == "." || path == "" {
		return "/"
	}
	return "/" + strings.Trim(path, "/")
}

func normalizedDependabotDirectory(path string) string {
	if path == "" || path == "." || path == "/" {
		return "/"
	}
	return "/" + strings.Trim(path, "/")
}

func identifierPart(value string) string {
	value = strings.ToLower(strings.Trim(value, "/."))
	value = nonIdentifier.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "root"
	}
	if value[0] < 'a' || value[0] > 'z' {
		value = "root-" + value
	}
	return value
}

func renderDependabot(source []byte, limits map[string]int, targets map[string]string) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(source, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 {
		return nil, fmt.Errorf("expected one YAML document")
	}
	root := document.Content[0]
	updates := mappingValue(root, "updates")
	if updates == nil || updates.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("updates must be a sequence")
	}
	for _, update := range updates.Content {
		ecosystem := scalarMappingValue(update, "package-ecosystem")
		directory := normalizedDependabotDirectory(scalarMappingValue(update, "directory"))
		limit, ok := limits[ecosystem+"\x00"+directory]
		if !ok {
			continue
		}
		setScalarMappingValue(update, "open-pull-requests-limit", fmt.Sprintf("%d", limit), "!!int")
		setScalarMappingValue(update, "target-branch", targets[ecosystem+"\x00"+directory], "!!str")
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func scalarMappingValue(node *yaml.Node, key string) string {
	value := mappingValue(node, key)
	if value == nil {
		return ""
	}
	return value.Value
}

func setScalarMappingValue(node *yaml.Node, key, value, tag string) {
	if existing := mappingValue(node, key); existing != nil {
		existing.Value, existing.Tag, existing.Kind = value, tag, yaml.ScalarNode
		return
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
	)
}

func duplicateAdapterFinding(root string, updates []dependabotUpdate) *Finding {
	paths := []string{"renovate.json", "renovate.json5", ".github/renovate.json", ".github/renovate.json5"}
	var data []byte
	var path string
	for _, candidate := range paths {
		value, err := readRegular(root, candidate)
		if err == nil {
			data, path = value, candidate
			break
		}
	}
	if len(data) == 0 {
		return nil
	}
	var renovate struct {
		EnabledManagers []string `json:"enabledManagers"`
		Enabled         *bool    `json:"enabled"`
	}
	if json.Unmarshal(data, &renovate) != nil {
		return &Finding{RuleID: "DT-DEP-010", Status: "fail", Severity: "error", Path: path,
			Message: "A Renovate configuration exists but its enabled managers cannot be proven disjoint from Dependabot.", Remediation: "Remove the second adapter or use a parseable, explicit non-overlapping enabledManagers declaration."}
	}
	if renovate.Enabled != nil && !*renovate.Enabled {
		return nil
	}
	dependabotManagers := map[string]bool{}
	for _, update := range updates {
		dependabotManagers[update.PackageEcosystem] = true
	}
	if len(renovate.EnabledManagers) == 0 {
		return &Finding{RuleID: "DT-DEP-010", Status: "fail", Severity: "error", Path: path,
			Message: "Renovate does not declare an explicit enabledManagers set, so its default managers can overlap Dependabot.", Remediation: "Remove the second adapter or declare a parseable, explicit non-overlapping enabledManagers set."}
	}
	aliases := map[string][]string{
		"npm":              {"npm"},
		"cargo":            {"cargo"},
		"gomod":            {"gomod"},
		"pip_requirements": {"uv", "pip"},
		"pip_setup":        {"uv", "pip"},
		"pip-compile":      {"uv", "pip"},
		"poetry":           {"uv", "pip"},
		"pep621":           {"uv", "pip"},
		"uv":               {"uv", "pip"},
		"terraform":        {"terraform"},
		"github-actions":   {"github-actions"},
		"dockerfile":       {"docker"},
	}
	for _, manager := range renovate.EnabledManagers {
		for _, ecosystem := range aliases[manager] {
			if dependabotManagers[ecosystem] {
				return &Finding{RuleID: "DT-DEP-010", Status: "fail", Severity: "error", Path: path, Secondary: manager,
					Message: "Dependabot and Renovate declare overlapping manager " + manager + ".", Remediation: "Select one adapter for this dependency surface."}
			}
		}
	}
	return nil
}

func existingSecurityRouter(root string) string {
	directory := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := ".github/workflows/" + entry.Name()
		data, err := readRegular(root, path)
		if err == nil && bytes.Contains(data, []byte("pull_request_target")) && bytes.Contains(data, []byte("dependabot[bot]")) && bytes.Contains(data, []byte("pull_request.head.repo.full_name")) {
			return path
		}
	}
	return ""
}

func requiresSecurityRouter(roots []EffectiveRoot) bool {
	for _, root := range roots {
		if root.ReleaseFlow.IntegrationBranch != "" && root.ReleaseFlow.ReleaseBranch != "" && root.ReleaseFlow.IntegrationBranch != root.ReleaseFlow.ReleaseBranch {
			return true
		}
	}
	return false
}

func renderSecurityRouter(releaseBranch, integrationBranch string) []byte {
	return []byte(fmt.Sprintf(`name: Dependabot Security Routing

on:
  pull_request_target:
    branches:
      - %s
    types:
      - opened
      - reopened
      - synchronize

permissions:
  pull-requests: write

concurrency:
  group: dependabot-security-routing-${{ github.event.pull_request.number }}
  cancel-in-progress: false

jobs:
  route-to-integration:
    if: >-
      github.event.pull_request.user.login == 'dependabot[bot]' &&
      github.event.pull_request.head.repo.full_name == github.repository &&
      startsWith(github.event.pull_request.head.ref, 'dependabot/')
    runs-on: ubuntu-24.04
    steps:
      - name: Retarget Dependabot security update
        env:
          GH_TOKEN: ${{ github.token }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
          REPOSITORY: ${{ github.repository }}
        run: gh api --method PATCH "repos/${REPOSITORY}/pulls/${PR_NUMBER}" -f base=%s
`, releaseBranch, integrationBranch))
}

func configurationEvaluation(result Evaluation, path, message string) Evaluation {
	return configurationEvaluationForRule(result, "DT-DEP-005", path, message)
}

func configurationEvaluationForRule(result Evaluation, ruleID, path, message string) Evaluation {
	result.ExitCode = 2
	result.Findings = append(result.Findings, Finding{RuleID: ruleID, Status: "error", Severity: "error", Path: path, Message: message, Remediation: "Correct the repository declaration or reference before compiling dependency automation."})
	return result
}

func internalEvaluation(result Evaluation, message string) Evaluation {
	result.ExitCode = 3
	result.Findings = append(result.Findings, Finding{RuleID: "DT-DEP-005", Status: "error", Severity: "error", Path: ".", Message: message, Remediation: "Retry with the immutable tooling release and report an implementation defect if it repeats."})
	return result
}

func hasFail(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Status == "fail" {
			return true
		}
	}
	return false
}

func sortEvaluation(result *Evaluation) {
	sort.Slice(result.Roots, func(left, right int) bool { return result.Roots[left].RootID < result.Roots[right].RootID })
	sort.Slice(result.Findings, func(left, right int) bool {
		a, b := result.Findings[left], result.Findings[right]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Secondary < b.Secondary
	})
}
