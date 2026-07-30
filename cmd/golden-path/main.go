package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/5010-dev/engineering-tooling/checker"
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
	if len(arguments) == 0 || arguments[0] != "check" {
		if !writeMessage(stderr, "usage: golden-path check --root <repository> --evaluated-at <RFC3339 UTC> [--json-output <path|->]") {
			return 3
		}
		return 2
	}

	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	evaluatedAtInput := flags.String("evaluated-at", "", "explicit RFC3339 UTC evaluation time")
	enforcement := flags.String("enforcement", "report-only", "report-only enforcement for 0.x")
	jsonOutput := flags.String("json-output", "-", "JSON output path outside the repository, or - for standard output")
	if err := flags.Parse(arguments[1:]); err != nil {
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

	if *jsonOutput != "-" {
		inside, checkErr := pathInsideRoot(*root, *jsonOutput)
		if checkErr != nil {
			if !writeMessage(stderr, "unable to validate JSON output path") {
				return 3
			}
			return 2
		}
		if inside {
			if !writeMessage(stderr, "json-output must be outside the checked repository") {
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
	jsonData, err := checker.RenderJSON(result)
	if err != nil {
		if !writeMessage(stderr, "checker result failed its bundled output contract") {
			return 3
		}
		return 3
	}
	textData := checker.RenderText(result)

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
	if err := writeOutput(*jsonOutput, jsonData); err != nil {
		if !writeMessage(stderr, "unable to write JSON output") {
			return 3
		}
		return 3
	}
	return result.ExitCode
}

func writeMessage(writer io.Writer, message string) bool {
	_, err := io.WriteString(writer, message+"\n")
	return err == nil
}

func pathInsideRoot(root, output string) (bool, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	absoluteOutput, err := filepath.Abs(output)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(absoluteRoot, absoluteOutput)
	if err != nil {
		return false, err
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
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
