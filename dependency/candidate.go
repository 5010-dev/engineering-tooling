package dependency

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Preview projects compiled files and optional queue intent without mutation.
func Preview(root string, observation *Observation) (Candidate, Evaluation, error) {
	evaluation := Evaluate(root)
	if evaluation.ExitCode == 2 || evaluation.ExitCode == 3 {
		return Candidate{}, evaluation, fmt.Errorf("dependency evaluation is incomplete")
	}
	_, policySource, err := loadPolicy(root)
	if err != nil {
		return Candidate{}, evaluation, err
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Candidate{}, evaluation, err
	}
	candidate := Candidate{
		SchemaVersion: CandidateSchema,
		Repository:    filepath.Base(absolute),
		PolicySHA256:  sha256Digest(policySource),
		Changes:       []CandidateChange{},
		Roots:         []CandidateRoot{},
		QueueActions:  []QueueAction{},
	}
	for _, effective := range evaluation.Roots {
		candidate.Roots = append(candidate.Roots, CandidateRoot{
			NativeRootRef: effective.RootID, Status: effective.Classification, Reason: effective.Reason,
		})
	}
	for path, desired := range evaluation.RenderedFiles {
		change := CandidateChange{Path: path, Ownership: "repository-owned", DesiredSHA256: sha256Digest(desired)}
		current, readErr := readRegular(root, path)
		switch {
		case errors.Is(readErr, errInputNotFound):
			change.Status = "create"
		case readErr != nil:
			change.Status = "conflict"
			candidate.ConflictCount++
		case bytes.Equal(current, desired):
			change.Status = "preserve"
			change.CurrentSHA256 = change.DesiredSHA256
		default:
			change.Status = "update"
			change.CurrentSHA256 = sha256Digest(current)
		}
		candidate.Changes = append(candidate.Changes, change)
	}
	for _, path := range []string{
		".github/golden-path-dependency-policy.yaml",
		".github/golden-path-native-roots.yaml",
		".github/release-units.json",
		".github/golden-path-dependency-defers.yaml",
	} {
		if _, already := evaluation.RenderedFiles[path]; already {
			continue
		}
		current, readErr := readRegular(root, path)
		if readErr == nil {
			candidate.Changes = append(candidate.Changes, CandidateChange{
				Path: path, Ownership: "repository-owned", Status: "preserve", CurrentSHA256: sha256Digest(current),
			})
		}
	}
	if observation != nil {
		for _, pullRequest := range observation.PullRequests {
			if !repositoryMatches(pullRequest.Repository, candidate.Repository) {
				continue
			}
			action := "regroup-routine"
			reason := "routine queue action requires root, artifact, validation, and risk compatibility"
			switch pullRequest.Classification {
			case "security-update", "security-remediation":
				action, reason = "preserve-security", "security work does not wait for routine regrouping"
			case "unknown":
				action, reason = "preserve-manual", "classification is not authoritative enough for automatic closure"
			default:
				if pullRequest.NativeRootRef == "" {
					action, reason = "pending-classification", "routine PR has no explicit native-root binding"
				}
			}
			candidate.QueueActions = append(candidate.QueueActions, QueueAction{PullRequest: pullRequest.URL, Action: action, Reason: reason})
		}
	}
	sort.Slice(candidate.Changes, func(left, right int) bool { return candidate.Changes[left].Path < candidate.Changes[right].Path })
	sort.Slice(candidate.Roots, func(left, right int) bool {
		return candidate.Roots[left].NativeRootRef < candidate.Roots[right].NativeRootRef
	})
	sort.Slice(candidate.QueueActions, func(left, right int) bool {
		return candidate.QueueActions[left].PullRequest < candidate.QueueActions[right].PullRequest
	})
	encoded, err := json.Marshal(candidate)
	if err != nil {
		return Candidate{}, evaluation, err
	}
	if err := validateSchema("golden-path-dependency-candidate-v1.schema.json", encoded); err != nil {
		return Candidate{}, evaluation, fmt.Errorf("candidate violates bundled schema: %w", err)
	}
	return candidate, evaluation, nil
}

// WriteStaging writes only compiled candidate files to a separate empty output.
func WriteStaging(repository, output string, candidate Candidate, evaluation Evaluation) error {
	repositoryAbsolute, err := filepath.Abs(repository)
	if err != nil {
		return err
	}
	outputAbsolute, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(repositoryAbsolute, outputAbsolute)
	if err != nil {
		return err
	}
	if relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("staging output must be outside the repository")
	}
	info, err := os.Stat(outputAbsolute)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(outputAbsolute, 0o750); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("staging output must be a directory")
	}
	entries, err := os.ReadDir(outputAbsolute)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("staging output must be empty")
	}
	for path, content := range evaluation.RenderedFiles {
		target := filepath.Join(outputAbsolute, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(outputAbsolute, "dependency-candidate.json"), data, 0o600)
}

func repositoryMatches(fullName, name string) bool {
	return fullName == name || strings.HasSuffix(fullName, "/"+name)
}

// DecodeCandidate validates a bounded serialized candidate before report use.
func DecodeCandidate(reader io.Reader) (Candidate, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxRepositoryInput+1))
	if err != nil {
		return Candidate{}, err
	}
	if len(data) > maxRepositoryInput {
		return Candidate{}, fmt.Errorf("candidate exceeds the input limit")
	}
	if err := validateSchema("golden-path-dependency-candidate-v1.schema.json", data); err != nil {
		return Candidate{}, err
	}
	var candidate Candidate
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&candidate); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}
