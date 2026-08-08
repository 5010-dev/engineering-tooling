package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/5010-dev/engineering-tooling/checker"
	"github.com/5010-dev/engineering-tooling/dependency"
	"github.com/5010-dev/engineering-tooling/generator"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 1 && (arguments[0] == "--version" || arguments[0] == "version") {
		if !writeMessage(stdout, fmt.Sprintf("golden-path %s", checker.Version)) {
			return 3
		}
		return 0
	}
	if len(arguments) == 0 {
		if !writeUsage(stderr) {
			return 3
		}
		return 2
	}
	switch arguments[0] {
	case "check":
		return runCheck(arguments[1:], stdout, stderr)
	case "generate":
		return runMaterialization("generate", arguments[1:], stdout, stderr)
	case "upgrade":
		return runMaterialization("upgrade", arguments[1:], stdout, stderr)
	case "dependency":
		return runDependency(arguments[1:], stdout, stderr)
	default:
		if !writeUsage(stderr) {
			return 3
		}
		return 2
	}
}

func runCheck(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	evaluatedAtInput := flags.String("evaluated-at", "", "explicit RFC3339 UTC evaluation time")
	enforcement := flags.String("enforcement", "report-only", "enforcement state declared by the release compatibility manifest")
	jsonOutput := flags.String("json-output", "-", "JSON output path outside the repository, or - for standard output")
	githubSummaryOutput := flags.String("github-summary-output", "", "GitHub step summary path outside the repository")
	githubAnnotations := flags.Bool("github-annotations", false, "emit bounded GitHub workflow annotations")
	showAll := flags.Bool("show-all", false, "show every passing and skipped finding in human-readable output")
	verbose := flags.Bool("verbose", false, "alias for --show-all")
	expectedProfiles := flags.String("expected-profiles", "", "exact JSON profile array declared by the thin caller")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		if !writeMessage(stderr, "unexpected positional arguments") {
			return 3
		}
		return 2
	}

	evaluatedAt, err := time.Parse(time.RFC3339, *evaluatedAtInput)
	if err != nil {
		if !writeMessage(stderr, "evaluated-at must be an explicit RFC3339 UTC timestamp") {
			return 3
		}
		return 2
	}
	_, offset := evaluatedAt.Zone()
	if offset != 0 {
		if !writeMessage(stderr, "evaluated-at must use UTC") {
			return 3
		}
		return 2
	}

	resolvedJSONOutput := *jsonOutput
	if *jsonOutput != "-" {
		resolvedJSONOutput, err = safeOutputPath(*root, *jsonOutput)
		if err != nil {
			if !writeMessage(stderr, "unable to validate JSON output path") {
				return 3
			}
			return 2
		}
	}
	resolvedSummaryOutput := ""
	if *githubSummaryOutput != "" {
		resolvedSummaryOutput, err = safeOutputPath(*root, *githubSummaryOutput)
		if err != nil {
			if !writeMessage(stderr, "unable to validate GitHub summary output path") {
				return 3
			}
			return 2
		}
		if resolvedSummaryOutput == resolvedJSONOutput {
			if !writeMessage(stderr, "JSON and GitHub summary outputs must use different paths") {
				return 3
			}
			return 2
		}
	}

	result := checker.Check(checker.Options{
		Root:        *root,
		EvaluatedAt: evaluatedAt,
		Enforcement: *enforcement,
	})
	if *expectedProfiles != "" {
		var expected []string
		if err := json.Unmarshal([]byte(*expectedProfiles), &expected); err != nil {
			if !writeMessage(stderr, "expected-profiles must be a JSON string array") {
				return 3
			}
			return 2
		}
		slices.Sort(expected)
		if slices.Contains(expected, "") || !slices.Equal(expected, slices.Compact(expected)) {
			if !writeMessage(stderr, "expected-profiles must contain unique non-empty identifiers") {
				return 3
			}
			return 2
		}
		if result.Complete && !slices.Equal(expected, result.Profiles) {
			result.Findings = append(result.Findings, checker.Finding{
				RuleID: "DT-META-002", Status: "fail", Severity: "error", Assessment: "automated",
				Path: ".github/golden-path.yaml", Secondary: "caller-profile-contract",
				Message:     fmt.Sprintf("Thin caller profiles %v do not match repository metadata profiles %v.", expected, result.Profiles),
				Remediation: "Regenerate the thin caller and repository metadata from the same Golden Path request.",
			})
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
			result.Summary.Fail++
			result.ExitCode = 1
		}
	}
	jsonData, err := checker.RenderJSON(result)
	if err != nil {
		if !writeMessage(stderr, "checker result failed its bundled output contract") {
			return 3
		}
		return 3
	}
	textData := checker.RenderText(result)
	if *showAll || *verbose {
		textData = checker.RenderTextAll(result)
	}
	if resolvedSummaryOutput != "" {
		if err := writeOutput(resolvedSummaryOutput, checker.RenderGitHubSummary(result)); err != nil {
			if !writeMessage(stderr, "unable to write GitHub summary output") {
				return 3
			}
			return 3
		}
	}
	if *githubAnnotations {
		if _, err := stderr.Write(checker.RenderGitHubAnnotations(result)); err != nil {
			return 3
		}
	}

	if *jsonOutput == "-" {
		if _, err := stderr.Write(textData); err != nil {
			return 3
		}
		if _, err := stdout.Write(jsonData); err != nil {
			return 3
		}
		return result.ExitCode
	}
	if _, err := stdout.Write(textData); err != nil {
		return 3
	}
	if err := writeOutput(resolvedJSONOutput, jsonData); err != nil {
		if !writeMessage(stderr, "unable to write JSON output") {
			return 3
		}
		return 3
	}
	return result.ExitCode
}

func runMaterialization(operation string, arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(stderr)
	requestPath := flags.String("request", "", "generator request YAML or JSON")
	releasePath := flags.String("release-manifest", "", "verified Golden Path release manifest JSON")
	repository := flags.String("root", "", "existing repository root for upgrade")
	output := flags.String("output", "", "empty staging directory outside the upgraded repository")
	write := flags.Bool("write", false, "materialize the candidate into the staging output")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *requestPath == "" || *releasePath == "" {
		if !writeMessage(stderr, operation+" requires --request and --release-manifest with no positional arguments") {
			return 3
		}
		return 2
	}
	if *write && *output == "" || !*write && *output != "" {
		if !writeMessage(stderr, "--write and --output must be provided together") {
			return 3
		}
		return 2
	}
	if operation == "upgrade" && *repository == "" || operation == "generate" && *repository != "" {
		if !writeMessage(stderr, "upgrade requires --root; generate does not accept --root") {
			return 3
		}
		return 2
	}
	requestFile, err := os.Open(*requestPath)
	if err != nil {
		return materializationError(stderr, "open generator request", err)
	}
	request, err := generator.DecodeRequest(requestFile)
	closeRequestErr := requestFile.Close()
	if err == nil && closeRequestErr != nil {
		err = closeRequestErr
	}
	if err != nil {
		return materializationError(stderr, "validate generator request", err)
	}
	releaseFile, err := os.Open(*releasePath)
	if err != nil {
		return materializationError(stderr, "open release manifest", err)
	}
	release, err := generator.DecodeReleaseManifest(releaseFile)
	closeReleaseErr := releaseFile.Close()
	if err == nil && closeReleaseErr != nil {
		err = closeReleaseErr
	}
	if err != nil {
		return materializationError(stderr, "validate release manifest", err)
	}
	files, requestDigest, err := generator.Render(request, release)
	if err != nil {
		return materializationError(stderr, "render Golden Path assets", err)
	}
	bundle, err := generator.LoadBundle()
	if err != nil {
		return materializationError(stderr, "load Golden Path bundle", err)
	}
	var plan generator.Plan
	if operation == "generate" {
		plan = generator.GeneratePlan(files, requestDigest, bundle, release)
	} else {
		plan, err = generator.UpgradePlan(*repository, files, requestDigest, bundle, release)
		if err != nil {
			return materializationError(stderr, "plan Golden Path upgrade", err)
		}
		if *write {
			if err := generator.ValidateSeparateOutput(*repository, *output); err != nil {
				return materializationError(stderr, "validate upgrade staging output", err)
			}
		}
	}
	generator.SortPlan(&plan)
	if plan.ConflictCount != 0 {
		planData, marshalErr := json.MarshalIndent(plan, "", "  ")
		if marshalErr != nil {
			return materializationError(stderr, "encode materialization plan", marshalErr)
		}
		planData = append(planData, '\n')
		if _, writeErr := stdout.Write(planData); writeErr != nil {
			return 3
		}
		return 1
	}
	if *write {
		if err := generator.WriteStaging(*output, files, plan); err != nil {
			return materializationError(stderr, "write Golden Path staging output", err)
		}
	}
	planData, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return materializationError(stderr, "encode materialization plan", err)
	}
	planData = append(planData, '\n')
	if _, err := stdout.Write(planData); err != nil {
		return 3
	}
	return 0
}

type repeatedFlag []string

func (values *repeatedFlag) String() string         { return strings.Join(*values, ",") }
func (values *repeatedFlag) Set(value string) error { *values = append(*values, value); return nil }

func runDependency(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		if !writeMessage(stderr, "dependency requires check, compile, preview, report, or seal-observation") {
			return 3
		}
		return 2
	}
	switch arguments[0] {
	case "check":
		return runDependencyCheck(arguments[1:], stdout, stderr)
	case "compile", "preview":
		return runDependencyPreview(arguments[1:], stdout, stderr)
	case "report":
		return runDependencyReport(arguments[1:], stdout, stderr)
	case "seal-observation":
		return runSealObservation(arguments[1:], stdout, stderr)
	default:
		if !writeMessage(stderr, "unsupported dependency operation") {
			return 3
		}
		return 2
	}
}

func runDependencyCheck(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dependency check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return 2
	}
	result := dependency.Evaluate(*root)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return dependencyCommandError(stderr, "encode dependency evaluation", err, 3)
	}
	data = append(data, '\n')
	if _, err := stdout.Write(data); err != nil {
		return 3
	}
	return result.ExitCode
}

func runDependencyPreview(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dependency preview", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	observationPath := flags.String("observation", "", "optional sealed live observation JSON")
	write := flags.Bool("write", false, "write candidate files to a separate empty staging directory")
	output := flags.String("output", "", "separate empty staging directory")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return 2
	}
	if *write != (*output != "") {
		return dependencyCommandError(stderr, "validate staging output", fmt.Errorf("--write and --output must be supplied together"), 2)
	}
	var observation *dependency.Observation
	if *observationPath != "" {
		file, err := os.Open(*observationPath)
		if err != nil {
			return dependencyCommandError(stderr, "open observation", err, 2)
		}
		decoded, decodeErr := dependency.DecodeObservation(file)
		closeErr := file.Close()
		if decodeErr == nil {
			decodeErr = closeErr
		}
		if decodeErr != nil {
			return dependencyCommandError(stderr, "decode observation", decodeErr, 2)
		}
		observation = &decoded
	}
	candidate, evaluation, err := dependency.Preview(*root, observation)
	if err != nil {
		return dependencyCommandError(stderr, "compile dependency candidate", err, evaluation.ExitCode)
	}
	if *write {
		if err := dependency.WriteStaging(*root, *output, candidate, evaluation); err != nil {
			return dependencyCommandError(stderr, "write dependency staging output", err, 2)
		}
	}
	data, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return dependencyCommandError(stderr, "encode dependency candidate", err, 3)
	}
	data = append(data, '\n')
	if _, err := stdout.Write(data); err != nil {
		return 3
	}
	return evaluation.ExitCode
}

func runSealObservation(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dependency seal-observation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "unsealed observation JSON")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *input == "" {
		return 2
	}
	// #nosec G304 -- input is an explicit CLI path and the decoder enforces a bounded document.
	file, err := os.Open(*input)
	if err != nil {
		return dependencyCommandError(stderr, "read observation", err, 2)
	}
	observation, decodeErr := dependency.DecodeUnsealedObservation(file)
	closeErr := file.Close()
	if decodeErr == nil {
		decodeErr = closeErr
	}
	if decodeErr != nil {
		return dependencyCommandError(stderr, "decode observation", decodeErr, 2)
	}
	sealed, err := dependency.SealObservation(observation)
	if err != nil {
		return dependencyCommandError(stderr, "seal observation", err, 2)
	}
	if _, err := stdout.Write(sealed); err != nil {
		return 3
	}
	return 0
}

func runDependencyReport(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dependency report", flag.ContinueOnError)
	flags.SetOutput(stderr)
	observationPath := flags.String("observation", "", "sealed observation JSON")
	generatedAtInput := flags.String("generated-at", "", "explicit RFC3339 UTC generation time")
	var candidateFlags, deferFlags, notApplicableFlags repeatedFlag
	flags.Var(&candidateFlags, "candidate", "repository=dependency-candidate.json (repeatable)")
	flags.Var(&deferFlags, "defers", "repository=dependency-defers.yaml (repeatable)")
	flags.Var(&notApplicableFlags, "not-applicable", "repository name (repeatable)")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *observationPath == "" {
		return 2
	}
	generatedAt, err := time.Parse(time.RFC3339, *generatedAtInput)
	_, generatedAtOffset := generatedAt.Zone()
	if err != nil || generatedAtOffset != 0 {
		return dependencyCommandError(stderr, "parse generated-at", fmt.Errorf("generated-at must be explicit UTC"), 2)
	}
	generatedAt = generatedAt.UTC()
	observationFile, err := os.Open(*observationPath)
	if err != nil {
		return dependencyCommandError(stderr, "open observation", err, 2)
	}
	observation, decodeErr := dependency.DecodeObservation(observationFile)
	closeErr := observationFile.Close()
	if decodeErr == nil {
		decodeErr = closeErr
	}
	if decodeErr != nil {
		return dependencyCommandError(stderr, "decode observation", decodeErr, 2)
	}
	candidates := map[string]dependency.Candidate{}
	for _, value := range candidateFlags {
		repository, path, parseErr := splitAssignment(value)
		if parseErr != nil {
			return dependencyCommandError(stderr, "parse candidate", parseErr, 2)
		}
		// #nosec G304 -- path is the explicit candidate CLI input and the decoder applies a bounded schema check.
		file, openErr := os.Open(path)
		if openErr != nil {
			return dependencyCommandError(stderr, "open candidate", openErr, 2)
		}
		candidate, decodeErr := dependency.DecodeCandidate(file)
		closeErr := file.Close()
		if decodeErr == nil {
			decodeErr = closeErr
		}
		if decodeErr != nil {
			return dependencyCommandError(stderr, "decode candidate", decodeErr, 2)
		}
		candidates[repository] = candidate
	}
	defers := map[string]dependency.DefersFile{}
	for _, value := range deferFlags {
		repository, path, parseErr := splitAssignment(value)
		if parseErr != nil {
			return dependencyCommandError(stderr, "parse defers", parseErr, 2)
		}
		// #nosec G304 -- path is the explicit defer CLI input and the decoder applies a bounded schema check.
		file, openErr := os.Open(path)
		if openErr != nil {
			return dependencyCommandError(stderr, "open defers", openErr, 2)
		}
		decoded, decodeErr := dependency.DecodeDefers(file)
		closeErr := file.Close()
		if decodeErr == nil {
			decodeErr = closeErr
		}
		if decodeErr != nil {
			return dependencyCommandError(stderr, "decode defers", decodeErr, 2)
		}
		defers[repository] = decoded
	}
	notApplicable := map[string]bool{}
	for _, repository := range notApplicableFlags {
		notApplicable[repository] = true
	}
	report, err := dependency.GenerateReport(observation, candidates, defers, notApplicable, generatedAt)
	if err != nil {
		return dependencyCommandError(stderr, "generate dependency report", err, 2)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return dependencyCommandError(stderr, "encode dependency report", err, 3)
	}
	data = append(data, '\n')
	if _, err := stdout.Write(data); err != nil {
		return 3
	}
	return 0
}

func splitAssignment(value string) (string, string, error) {
	repository, path, found := strings.Cut(value, "=")
	if !found || repository == "" || path == "" {
		return "", "", fmt.Errorf("expected repository=path")
	}
	return repository, path, nil
}

func dependencyCommandError(stderr io.Writer, context string, err error, exitCode int) int {
	if exitCode == 0 {
		exitCode = 2
	}
	if !writeMessage(stderr, context+": "+err.Error()) {
		return 3
	}
	return exitCode
}

func materializationError(stderr io.Writer, context string, err error) int {
	if !writeMessage(stderr, context+": "+err.Error()) {
		return 3
	}
	return 2
}

func writeUsage(writer io.Writer) bool {
	return writeMessage(writer, "usage:\n  golden-path check --root <repository> --evaluated-at <RFC3339 UTC> [--expected-profiles <JSON>] [--json-output <path|->] [--github-summary-output <path>] [--github-annotations] [--show-all|--verbose]\n  golden-path generate --request <path> --release-manifest <path> [--write --output <empty-directory>]\n  golden-path upgrade --root <repository> --request <path> --release-manifest <path> [--write --output <separate-empty-directory>]\n  golden-path dependency check --root <repository>\n  golden-path dependency preview --root <repository> [--observation <sealed.json>] [--write --output <separate-empty-directory>]\n  golden-path dependency report --observation <sealed.json> --generated-at <RFC3339 UTC> [--candidate <repository=path>] [--defers <repository=path>]\n  golden-path dependency seal-observation --input <unsealed.json>")
}

func writeMessage(writer io.Writer, message string) bool {
	_, err := io.WriteString(writer, message+"\n")
	return err == nil
}

func safeOutputPath(root, output string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", err
	}
	absoluteOutput, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absoluteOutput))
	if err != nil {
		return "", err
	}
	resolvedOutput := filepath.Join(resolvedParent, filepath.Base(absoluteOutput))
	relative, err := filepath.Rel(resolvedRoot, resolvedOutput)
	if err != nil {
		return "", err
	}
	if relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("output resolves inside repository")
	}
	return resolvedOutput, nil
}

func writeOutput(name string, data []byte) error {
	parent := filepath.Dir(name)
	file, err := os.CreateTemp(parent, ".golden-path-result-*.tmp")
	if err != nil {
		return err
	}
	tempName := file.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(tempName)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, name); err != nil {
		return err
	}
	remove = false
	return nil
}
