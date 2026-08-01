package generator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func GeneratePlan(files []File, requestDigest string, bundle Bundle, release ReleaseManifest) Plan {
	plan := Plan{
		SchemaVersion: PlanSchema, Operation: "generate", StandardVersion: bundle.StandardVersion,
		AssetBundleVersion: bundle.AssetBundleVersion, ReleaseVersion: release.ReleaseVersion,
		RequestSHA256: requestDigest, Changes: make([]FileChange, 0, len(files)),
	}
	for _, file := range files {
		plan.Changes = append(plan.Changes, FileChange{
			Path: file.Path, Status: "create", Source: file.Source,
			DesiredMode: fileModeString(file.Mode), DesiredSHA256: digest(file.Content),
		})
	}
	SortPlan(&plan)
	return plan
}

func UpgradePlan(repository string, files []File, requestDigest string, bundle Bundle, release ReleaseManifest) (Plan, error) {
	if err := validateRenderedFiles(files); err != nil {
		return Plan{}, fmt.Errorf("validate upgrade candidate files: %w", err)
	}
	root, err := os.OpenRoot(repository)
	if err != nil {
		return Plan{}, fmt.Errorf("open upgrade source repository: %w", err)
	}
	defer func() { _ = root.Close() }()
	previous, err := readPreviousAssets(root)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		SchemaVersion: PlanSchema, Operation: "upgrade", StandardVersion: bundle.StandardVersion,
		AssetBundleVersion: bundle.AssetBundleVersion, ReleaseVersion: release.ReleaseVersion,
		RequestSHA256: requestDigest, Changes: make([]FileChange, 0, len(files)+len(previous)),
	}
	desiredPaths := make(map[string]bool, len(files))
	for _, file := range files {
		desiredPaths[file.Path] = true
		previousAsset, tracked := previous[file.Path]
		change := FileChange{
			Path: file.Path, Source: file.Source,
			DesiredMode: fileModeString(file.Mode), DesiredSHA256: digest(file.Content),
		}
		if tracked {
			change.PreviousMode = previousAsset.Mode
			change.PreviousSHA256 = previousAsset.SHA256
		}
		info, statErr := root.Lstat(filepath.FromSlash(file.Path))
		if errors.Is(statErr, fs.ErrNotExist) {
			if tracked {
				change.Status = "conflict"
				plan.ConflictCount++
			} else {
				change.Status = "create"
			}
			plan.Changes = append(plan.Changes, change)
			continue
		}
		if statErr != nil {
			return Plan{}, fmt.Errorf("inspect existing generated path %q: %w", file.Path, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			change.Status = "conflict"
			plan.ConflictCount++
			plan.Changes = append(plan.Changes, change)
			continue
		}
		change.CurrentMode = fileModeString(info.Mode())
		content, readErr := root.ReadFile(filepath.FromSlash(file.Path))
		if readErr != nil {
			return Plan{}, fmt.Errorf("read existing generated path %q: %w", file.Path, readErr)
		}
		change.CurrentSHA256 = digest(content)
		switch {
		case change.CurrentSHA256 == change.DesiredSHA256 && change.CurrentMode == change.DesiredMode:
			change.Status = "unchanged"
		case tracked && change.CurrentSHA256 == change.PreviousSHA256 && change.CurrentMode == change.PreviousMode:
			change.Status = "update"
		default:
			change.Status = "conflict"
			plan.ConflictCount++
		}
		plan.Changes = append(plan.Changes, change)
	}
	for previousPath, previousAsset := range previous {
		if desiredPaths[previousPath] || previousPath == ".github/golden-path-assets.json" {
			continue
		}
		change := FileChange{
			Path: previousPath, Status: "remove", Source: "previous-generated-asset",
			PreviousMode: previousAsset.Mode, PreviousSHA256: previousAsset.SHA256,
		}
		info, statErr := root.Lstat(filepath.FromSlash(previousPath))
		if errors.Is(statErr, fs.ErrNotExist) {
			change.Status = "unchanged"
			plan.Changes = append(plan.Changes, change)
			continue
		}
		if statErr != nil {
			return Plan{}, fmt.Errorf("inspect retired generated path %q: %w", previousPath, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			change.Status = "conflict"
			plan.ConflictCount++
			plan.Changes = append(plan.Changes, change)
			continue
		}
		change.CurrentMode = fileModeString(info.Mode())
		content, readErr := root.ReadFile(filepath.FromSlash(previousPath))
		if readErr != nil {
			return Plan{}, fmt.Errorf("read retired generated path %q: %w", previousPath, readErr)
		}
		change.CurrentSHA256 = digest(content)
		if change.CurrentSHA256 != change.PreviousSHA256 || change.CurrentMode != change.PreviousMode {
			change.Status = "conflict"
			plan.ConflictCount++
		}
		plan.Changes = append(plan.Changes, change)
	}
	SortPlan(&plan)
	return plan, nil
}

func WriteStaging(output string, files []File, plan Plan) error {
	if err := validateRenderedFiles(files); err != nil {
		return fmt.Errorf("validate staged files: %w", err)
	}
	if plan.ConflictCount != 0 {
		return fmt.Errorf("refuse to write a candidate with %d unresolved conflicts", plan.ConflictCount)
	}
	absoluteOutput, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolve staging output: %w", err)
	}
	if err := requireEmptyOutput(absoluteOutput); err != nil {
		return err
	}
	root, err := os.OpenRoot(absoluteOutput)
	if err != nil {
		return fmt.Errorf("open staging output: %w", err)
	}
	defer func() { _ = root.Close() }()
	for _, file := range files {
		name := filepath.FromSlash(file.Path)
		if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return fmt.Errorf("create staging directory for %q: %w", file.Path, err)
		}
		if err := root.WriteFile(name, file.Content, file.Mode); err != nil {
			return fmt.Errorf("write staged file %q: %w", file.Path, err)
		}
		if err := root.Chmod(name, file.Mode); err != nil {
			return fmt.Errorf("set staged file mode %q: %w", file.Path, err)
		}
	}
	planData, err := indentJSON(plan)
	if err != nil {
		return err
	}
	if err := root.WriteFile("golden-path-plan.json", planData, 0o644); err != nil {
		return fmt.Errorf("write staged plan: %w", err)
	}
	return nil
}

func ValidateSeparateOutput(repository, output string) error {
	resolvedRepository, err := canonicalExistingPath(repository)
	if err != nil {
		return fmt.Errorf("resolve upgrade repository: %w", err)
	}
	absoluteOutput, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolve upgrade output: %w", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absoluteOutput))
	if err != nil {
		return fmt.Errorf("resolve upgrade output parent: %w", err)
	}
	resolvedOutput := filepath.Join(resolvedParent, filepath.Base(absoluteOutput))
	if sameOrWithin(resolvedRepository, resolvedOutput) || sameOrWithin(resolvedOutput, resolvedRepository) {
		return fmt.Errorf("upgrade output must be separate from the source repository")
	}
	return nil
}

func readPreviousAssets(root *os.Root) (map[string]GeneratedAsset, error) {
	data, err := root.ReadFile(filepath.FromSlash(".github/golden-path-assets.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]GeneratedAsset{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read previous generated-assets manifest: %w", err)
	}
	var manifest AssetManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.SchemaVersion != AssetsSchema {
		return nil, fmt.Errorf("previous generated-assets manifest is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("previous generated-assets manifest must contain exactly one JSON document")
	}
	result := make(map[string]GeneratedAsset, len(manifest.Files))
	for _, file := range manifest.Files {
		cleaned, pathErr := cleanRelativePath(file.Path)
		_, duplicate := result[file.Path]
		if pathErr != nil || cleaned == "." || file.Path == ".github/golden-path-assets.json" || !fullDigest(file.SHA256) || duplicate || (file.Mode != "0644" && file.Mode != "0755") || file.Source == "" {
			return nil, fmt.Errorf("previous generated-assets manifest contains an invalid file record")
		}
		result[file.Path] = file
	}
	result[".github/golden-path-assets.json"] = GeneratedAsset{
		Path: ".github/golden-path-assets.json", Mode: "0644", SHA256: digest(data), Source: "generator/assets-manifest",
	}
	return result, nil
}

func fileModeString(mode fs.FileMode) string {
	return fmt.Sprintf("%04o", mode.Perm())
}

func requireEmptyOutput(output string) error {
	info, err := os.Lstat(output)
	if errors.Is(err, fs.ErrNotExist) {
		// #nosec G301 -- staged repositories require traversable conventional directories.
		if err := os.MkdirAll(output, 0o755); err != nil {
			return fmt.Errorf("create staging output: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect staging output: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("staging output must be a real directory")
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return fmt.Errorf("read staging output: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("staging output must be empty")
	}
	return nil
}

func canonicalExistingPath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func sameOrWithin(parent, candidate string) bool {
	relative, err := filepath.Rel(parent, candidate)
	return err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func fullDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func SortPlan(plan *Plan) {
	sort.Slice(plan.Changes, func(left, right int) bool { return plan.Changes[left].Path < plan.Changes[right].Path })
}
