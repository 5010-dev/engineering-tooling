package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/5010-dev/engineering-tooling/checker"
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
	enforcement := flags.String("enforcement", "report-only", "report-only enforcement for 0.x")
	jsonOutput := flags.String("json-output", "-", "JSON output path outside the repository, or - for standard output")
	githubSummaryOutput := flags.String("github-summary-output", "", "GitHub step summary path outside the repository")
	githubAnnotations := flags.Bool("github-annotations", false, "emit bounded GitHub workflow annotations")
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
		if !slices.Equal(expected, result.Profiles) {
			result.Findings = append(result.Findings, checker.Finding{
				RuleID: "DT-META-002", Status: "fail", Severity: "error", Assessment: "automated",
				Path: ".github/golden-path.yaml", Secondary: "caller-profile-contract",
				Message:     fmt.Sprintf("Thin caller profiles %v do not match repository metadata profiles %v.", expected, result.Profiles),
				Remediation: "Regenerate the thin caller and repository metadata from the same Golden Path request.",
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
	if plan.ConflictCount != 0 {
		return 1
	}
	return 0
}

func materializationError(stderr io.Writer, context string, err error) int {
	if !writeMessage(stderr, context+": "+err.Error()) {
		return 3
	}
	return 2
}

func writeUsage(writer io.Writer) bool {
	return writeMessage(writer, "usage:\n  golden-path check --root <repository> --evaluated-at <RFC3339 UTC> [--expected-profiles <JSON>] [--json-output <path|->] [--github-summary-output <path>] [--github-annotations]\n  golden-path generate --request <path> --release-manifest <path> [--write --output <empty-directory>]\n  golden-path upgrade --root <repository> --request <path> --release-manifest <path> [--write --output <separate-empty-directory>]")
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
