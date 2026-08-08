package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/5010-dev/engineering-tooling/checker"
)

type manifest struct {
	SchemaVersion     string                     `json:"schemaVersion"`
	ReleaseVersion    string                     `json:"releaseVersion"`
	Lifecycle         string                     `json:"lifecycle"`
	Enforcement       []string                   `json:"enforcement"`
	StandardVersion   string                     `json:"standardVersion"`
	ContractVersion   string                     `json:"contractVersion"`
	SchemaVersions    []string                   `json:"schemaVersions"`
	Source            source                     `json:"source"`
	CatalogDigest     string                     `json:"catalogDigest"`
	SnapshotDigest    string                     `json:"snapshotAggregateDigest"`
	Compatibility     compatibilityEvidence      `json:"compatibility"`
	Snapshot          snapshotEvidence           `json:"standardSnapshot"`
	Schemas           []schemaEvidence           `json:"schemas"`
	ReleaseNotes      releaseNotesEvidence       `json:"releaseNotes"`
	ToolingCutoff     cutoffEvidence             `json:"toolingCutoff"`
	RuntimeSelections []checker.RuntimeSelection `json:"runtimeSelections"`
	Components        releaseComponents          `json:"components"`
	Assets            []asset                    `json:"assets"`
	Build             buildEvidence              `json:"build"`
	Provenance        provenance                 `json:"provenance"`
}

type source struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Tag        string `json:"tag"`
}

type asset struct {
	Name             string `json:"name"`
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	SHA256           string `json:"sha256"`
	ExecutableSHA256 string `json:"executableSHA256"`
	Size             int64  `json:"size"`
	SBOM             sbom   `json:"sbom"`
}

type sbom struct {
	Name        string `json:"name"`
	Format      string `json:"format"`
	SpecVersion string `json:"specVersion"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

type buildEvidence struct {
	Toolchain          string   `json:"toolchain"`
	CleanCheckout      bool     `json:"cleanCheckout"`
	ValidationWorkflow string   `json:"validationWorkflow"`
	Commands           []string `json:"commands"`
}

type provenance struct {
	Provider string `json:"provider"`
	Workflow string `json:"workflow"`
	Subject  string `json:"subject"`
}

type releaseComponents struct {
	Checker     executableComponent `json:"checker"`
	Generator   executableComponent `json:"generator"`
	AssetBundle templateComponent   `json:"assetBundle"`
	Automation  automationComponent `json:"automation"`
}

type executableComponent struct {
	Version     string   `json:"version"`
	Executable  string   `json:"executable"`
	Digest      string   `json:"digest"`
	Lifecycle   string   `json:"lifecycle"`
	Enforcement []string `json:"enforcement"`
}

type templateComponent struct {
	Version               string `json:"version"`
	Digest                string `json:"digest"`
	RequestSchema         string `json:"requestSchema"`
	GeneratedAssetsSchema string `json:"generatedAssetsSchema"`
	PlanSchema            string `json:"planSchema"`
}

type automationComponent struct {
	Version          string `json:"version"`
	Digest           string `json:"digest"`
	ReusableWorkflow string `json:"reusableWorkflow"`
	SetupAction      string `json:"setupAction"`
}

type cutoffEvidence struct {
	Manifest  string `json:"manifest"`
	Schema    string `json:"schema"`
	CheckedAt string `json:"checkedAt"`
	SHA256    string `json:"sha256"`
}

type fileEvidence struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type compatibilityEvidence struct {
	File               fileEvidence                 `json:"file"`
	SupportedStandards []checker.CompatibleStandard `json:"supportedStandards"`
}

type snapshotEvidence struct {
	File      fileEvidence   `json:"file"`
	Source    snapshotSource `json:"source"`
	Aggregate string         `json:"aggregateDigest"`
}

type snapshotSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Path       string `json:"path"`
	GitTree    string `json:"gitTree"`
}

type schemaEvidence struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type releaseNotesEvidence struct {
	File     fileEvidence `json:"file"`
	Provides []string     `json:"provides"`
}

type snapshotDocument struct {
	SchemaVersion   string         `json:"schemaVersion"`
	StandardVersion string         `json:"standardVersion"`
	ContractVersion string         `json:"contractVersion"`
	Source          snapshotSource `json:"source"`
	Aggregate       struct {
		Algorithm string `json:"algorithm"`
		Digest    string `json:"digest"`
	} `json:"aggregate"`
}

type schemaAsset struct {
	ID         string
	SourceName string
	DistName   string
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(arguments []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("release-manifest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dist := flags.String("dist", "dist", "release asset directory")
	sourceRoot := flags.String("source", ".", "release source directory")
	tag := flags.String("tag", "", "exact release tag")
	commit := flags.String("source-commit", "", "full source commit SHA")
	output := flags.String("output", "", "manifest output path")
	requireCleanCheckout := flags.Bool("require-clean-checkout", false, "require source HEAD and worktree to match source-commit")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *tag != "v"+checker.Version {
		return fmt.Errorf("release tag %q does not match checker version v%s", *tag, checker.Version)
	}
	if !fullCommit(*commit) {
		return fmt.Errorf("source commit must be a full lowercase SHA")
	}
	if *output == "" {
		return fmt.Errorf("output path is required")
	}
	cleanCheckout, err := cleanSourceCheckout(*sourceRoot, *commit)
	if err != nil {
		return fmt.Errorf("inspect release source state: %w", err)
	}
	if *requireCleanCheckout && !cleanCheckout {
		return fmt.Errorf("release source must be a clean checkout of %s", *commit)
	}

	compatibility, err := checker.BundledCompatibility()
	if err != nil {
		return fmt.Errorf("verify bundled compatibility manifest: %w", err)
	}
	if len(compatibility.Standards) != 1 {
		return fmt.Errorf("bundled compatibility manifest must select one standard")
	}
	assets, err := collectAssets(*dist, checker.Version, compatibility.SupportedTargets)
	if err != nil {
		return err
	}
	standard := compatibility.Standards[0]
	goVersion, err := preferredRuntimeVersion(compatibility.RuntimeSelections, "go")
	if err != nil {
		return err
	}
	if runtime.Version() != "go"+goVersion {
		return fmt.Errorf("release manifest compiler %q does not match preferred Go %q", runtime.Version(), goVersion)
	}
	root, err := os.OpenRoot(*sourceRoot)
	if err != nil {
		return fmt.Errorf("open release source: %w", err)
	}
	defer func() { _ = root.Close() }()
	templateDigest, err := digestSourceTree(root, []string{"templates"}, map[string]bool{"templates/embed.go": true})
	if err != nil {
		return fmt.Errorf("digest template bundle: %w", err)
	}
	automationDigest, err := digestSourceTree(root, []string{
		".github/workflows/golden-path-quality.yml",
		"actions/setup-golden-path/action.yml",
	}, nil)
	if err != nil {
		return fmt.Errorf("digest shared automation: %w", err)
	}
	checkerDigest, err := digestSourceTree(root, []string{"checker", "cmd/golden-path"}, nil)
	if err != nil {
		return fmt.Errorf("digest checker component: %w", err)
	}
	generatorDigest, err := digestSourceTree(root, []string{"generator"}, nil)
	if err != nil {
		return fmt.Errorf("digest generator component: %w", err)
	}
	distRoot, err := os.OpenRoot(*dist)
	if err != nil {
		return fmt.Errorf("open release asset directory: %w", err)
	}
	defer func() { _ = distRoot.Close() }()
	cutoffFile, cutoffData, err := matchingEvidence(root, distRoot, "release/tooling-cutoff-2026-08-04.json", "tooling-cutoff.json")
	if err != nil {
		return fmt.Errorf("read tooling cutoff: %w", err)
	}
	_, sourceIntegrityData, err := matchingEvidence(root, distRoot, "release/source-integrity-2026-08-09.json", "source-integrity.json")
	if err != nil {
		return fmt.Errorf("read source integrity: %w", err)
	}
	compatibilityFile, _, err := matchingEvidence(root, distRoot, "compatibility/manifest.json", "compatibility-manifest.json")
	if err != nil {
		return fmt.Errorf("bind compatibility manifest: %w", err)
	}
	snapshotFile, snapshotData, err := matchingEvidence(root, distRoot, "standards/snapshots/2026.08.4/manifest.json", "standard-snapshot-manifest.json")
	if err != nil {
		return fmt.Errorf("bind standard snapshot manifest: %w", err)
	}
	releaseNotesFile, _, err := matchingEvidence(root, distRoot, "release/RELEASE_NOTES.md", "RELEASE_NOTES.md")
	if err != nil {
		return fmt.Errorf("bind release notes: %w", err)
	}
	schemas, err := collectSchemaEvidence(root, distRoot)
	if err != nil {
		return err
	}
	var cutoff struct {
		SchemaVersion   string `json:"schemaVersion"`
		CheckedAt       string `json:"checkedAt"`
		StandardVersion string `json:"standardVersion"`
		ReleaseVersion  string `json:"releaseVersion"`
	}
	if err := json.Unmarshal(cutoffData, &cutoff); err != nil ||
		cutoff.SchemaVersion != "golden-path-tooling-cutoff/v1" ||
		cutoff.CheckedAt == "" {
		return fmt.Errorf("retained tooling cutoff is invalid")
	}
	var sourceIntegrity struct {
		SchemaVersion   string `json:"schemaVersion"`
		CheckedAt       string `json:"checkedAt"`
		StandardVersion string `json:"standardVersion"`
		ReleaseVersion  string `json:"releaseVersion"`
	}
	if err := json.Unmarshal(sourceIntegrityData, &sourceIntegrity); err != nil ||
		sourceIntegrity.SchemaVersion != "golden-path-source-integrity/v1" ||
		sourceIntegrity.StandardVersion != checker.StandardVersion ||
		sourceIntegrity.ReleaseVersion != checker.Version ||
		sourceIntegrity.CheckedAt == "" {
		return fmt.Errorf("source integrity identity does not match the release")
	}
	var snapshot snapshotDocument
	if err := json.Unmarshal(snapshotData, &snapshot); err != nil || !validSnapshotIdentity(snapshot, standard) {
		return fmt.Errorf("standard snapshot identity does not match the release")
	}
	document := manifest{
		SchemaVersion:   "golden-path-release-manifest/v2",
		ReleaseVersion:  checker.Version,
		Lifecycle:       compatibility.Lifecycle,
		Enforcement:     compatibility.Enforcement,
		StandardVersion: checker.StandardVersion,
		ContractVersion: checker.ContractVersion,
		SchemaVersions: []string{
			"golden-path-metadata/v1",
			"golden-path-native-roots/v1",
			"golden-path-dependency-policy/v1",
			"golden-path-dependency-defers/v1",
			"golden-path-dependency-observation/v1",
			"golden-path-dependency-candidate/v1",
			"golden-path-dependency-report/v1",
			"golden-path-exceptions/v1",
			"golden-path-checker-output/v1",
			"golden-path-rule-catalog/v1",
			"runtime-support/v1",
			"golden-path-generator-request/v1",
			"golden-path-generated-assets/v1",
			"golden-path-materialization-plan/v1",
		},
		Source: source{
			Repository: "https://github.com/5010-dev/engineering-tooling",
			Commit:     *commit,
			Tag:        *tag,
		},
		CatalogDigest:  standard.CatalogDigest,
		SnapshotDigest: standard.SnapshotAggregateDigest,
		Compatibility: compatibilityEvidence{
			File: compatibilityFile, SupportedStandards: compatibility.Standards,
		},
		Snapshot: snapshotEvidence{
			File: snapshotFile, Source: snapshot.Source, Aggregate: standard.SnapshotAggregateDigest,
		},
		Schemas: schemas,
		ReleaseNotes: releaseNotesEvidence{
			File: releaseNotesFile, Provides: []string{"changelog", "migration", "rollback"},
		},
		ToolingCutoff: cutoffEvidence{
			Manifest: "tooling-cutoff.json", Schema: "golden-path-tooling-cutoff-v1.schema.json",
			CheckedAt: cutoff.CheckedAt, SHA256: cutoffFile.SHA256,
		},
		RuntimeSelections: compatibility.RuntimeSelections,
		Components: releaseComponents{
			Checker: executableComponent{
				Version: checker.Version, Executable: "golden-path",
				Digest: checkerDigest, Lifecycle: compatibility.Lifecycle, Enforcement: compatibility.Enforcement,
			},
			Generator: executableComponent{
				Version: checker.Version, Executable: "golden-path",
				Digest: generatorDigest, Lifecycle: compatibility.Lifecycle, Enforcement: compatibility.Enforcement,
			},
			AssetBundle: templateComponent{
				Version: checker.Version, Digest: templateDigest, RequestSchema: "golden-path-generator-request/v1",
				GeneratedAssetsSchema: "golden-path-generated-assets/v1", PlanSchema: "golden-path-materialization-plan/v1",
			},
			Automation: automationComponent{
				Version: checker.Version, Digest: automationDigest, ReusableWorkflow: ".github/workflows/golden-path-quality.yml",
				SetupAction: "actions/setup-golden-path/action.yml",
			},
		},
		Assets: assets,
		Build: buildEvidence{
			Toolchain: "go" + goVersion, CleanCheckout: cleanCheckout,
			ValidationWorkflow: ".github/workflows/release.yml",
			Commands: []string{
				"just ci",
				"go run -mod=readonly ./cmd/release-package --source . --dist dist",
				"deterministic Go tar and gzip packager",
			},
		},
		Provenance: provenance{
			Provider: "GitHub artifact attestations",
			Workflow: ".github/workflows/release.yml",
			Subject:  "all published dist assets",
		},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*output, data, 0o600); err != nil {
		return fmt.Errorf("write release manifest: %w", err)
	}
	return nil
}

func validSnapshotIdentity(snapshot snapshotDocument, standard checker.CompatibleStandard) bool {
	return snapshot.SchemaVersion == "golden-path-standard-snapshot/v1" &&
		snapshot.StandardVersion == checker.StandardVersion &&
		snapshot.ContractVersion == checker.ContractVersion &&
		snapshot.Source.Repository == "https://github.com/5010-dev/.github" &&
		snapshot.Source.Commit == standard.SourceCommit &&
		snapshot.Source.Path == "docs/standards/developer-tooling" &&
		snapshot.Source.GitTree == checker.SnapshotSourceTree &&
		snapshot.Aggregate.Algorithm == "sha256" &&
		"sha256:"+snapshot.Aggregate.Digest == standard.SnapshotAggregateDigest
}

func matchingEvidence(sourceRoot, distRoot *os.Root, sourceName, distName string) (fileEvidence, []byte, error) {
	sourceData, err := sourceRoot.ReadFile(sourceName)
	if err != nil {
		return fileEvidence{}, nil, fmt.Errorf("read source evidence %q: %w", sourceName, err)
	}
	distInfo, err := distRoot.Stat(distName)
	if err != nil || !distInfo.Mode().IsRegular() {
		return fileEvidence{}, nil, fmt.Errorf("release evidence %q is missing or invalid", distName)
	}
	distData, err := distRoot.ReadFile(distName)
	if err != nil {
		return fileEvidence{}, nil, fmt.Errorf("read release evidence %q: %w", distName, err)
	}
	if !bytes.Equal(sourceData, distData) {
		return fileEvidence{}, nil, fmt.Errorf("release evidence %q does not match %q", distName, sourceName)
	}
	digest := sha256.Sum256(distData)
	return fileEvidence{Name: distName, SHA256: hex.EncodeToString(digest[:])}, distData, nil
}

func collectSchemaEvidence(sourceRoot, distRoot *os.Root) ([]schemaEvidence, error) {
	assets := []schemaAsset{
		{ID: "golden-path-checker-compatibility/v1", SourceName: "compatibility/golden-path-checker-compatibility-v1.schema.json", DistName: "golden-path-checker-compatibility-v1.schema.json"},
		{ID: "golden-path-dependency-candidate/v1", SourceName: "standards/snapshots/2026.08.4/schemas/golden-path-dependency-candidate-v1.schema.json", DistName: "golden-path-dependency-candidate-v1.schema.json"},
		{ID: "golden-path-dependency-defers/v1", SourceName: "standards/snapshots/2026.08.4/schemas/golden-path-dependency-defers-v1.schema.json", DistName: "golden-path-dependency-defers-v1.schema.json"},
		{ID: "golden-path-dependency-observation/v1", SourceName: "standards/snapshots/2026.08.4/schemas/golden-path-dependency-observation-v1.schema.json", DistName: "golden-path-dependency-observation-v1.schema.json"},
		{ID: "golden-path-dependency-policy/v1", SourceName: "standards/snapshots/2026.08.4/schemas/golden-path-dependency-policy-v1.schema.json", DistName: "golden-path-dependency-policy-v1.schema.json"},
		{ID: "golden-path-dependency-report/v1", SourceName: "standards/snapshots/2026.08.4/schemas/golden-path-dependency-report-v1.schema.json", DistName: "golden-path-dependency-report-v1.schema.json"},
		{ID: "golden-path-generated-assets/v1", SourceName: "generator/schemas/golden-path-generated-assets-v1.schema.json", DistName: "golden-path-generated-assets-v1.schema.json"},
		{ID: "golden-path-generator-request/v1", SourceName: "generator/schemas/golden-path-generator-request-v1.schema.json", DistName: "golden-path-generator-request-v1.schema.json"},
		{ID: "golden-path-materialization-plan/v1", SourceName: "generator/schemas/golden-path-materialization-plan-v1.schema.json", DistName: "golden-path-materialization-plan-v1.schema.json"},
		{ID: "golden-path-release-manifest/v1", SourceName: "release/golden-path-release-manifest-v1.schema.json", DistName: "golden-path-release-manifest-v1.schema.json"},
		{ID: "golden-path-release-manifest/v2", SourceName: "release/golden-path-release-manifest-v2.schema.json", DistName: "golden-path-release-manifest-v2.schema.json"},
		{ID: "golden-path-source-integrity/v1", SourceName: "release/golden-path-source-integrity-v1.schema.json", DistName: "golden-path-source-integrity-v1.schema.json"},
		{ID: "golden-path-tooling-cutoff/v1", SourceName: "release/golden-path-tooling-cutoff-v1.schema.json", DistName: "golden-path-tooling-cutoff-v1.schema.json"},
	}
	result := make([]schemaEvidence, 0, len(assets))
	for _, asset := range assets {
		evidence, _, err := matchingEvidence(sourceRoot, distRoot, asset.SourceName, asset.DistName)
		if err != nil {
			return nil, fmt.Errorf("bind schema %q: %w", asset.ID, err)
		}
		result = append(result, schemaEvidence{ID: asset.ID, Name: evidence.Name, SHA256: evidence.SHA256})
	}
	return result, nil
}

func digestSourceTree(root *os.Root, roots []string, excluded map[string]bool) (string, error) {
	var names []string
	for _, base := range roots {
		err := fs.WalkDir(root.FS(), base, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || excluded[name] {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("source asset %q is not a regular file", name)
			}
			names = append(names, name)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("source asset set is empty")
	}
	sort.Strings(names)
	hasher := sha256.New()
	for _, name := range names {
		info, err := root.Stat(name)
		if err != nil {
			return "", err
		}
		if _, err := fmt.Fprintf(hasher, "%s\x00%04o\x00%d\x00", name, normalizedSourceMode(info.Mode()), info.Size()); err != nil {
			return "", err
		}
		file, err := root.Open(name)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if _, err := hasher.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func normalizedSourceMode(mode fs.FileMode) fs.FileMode {
	if mode.Perm()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func cleanSourceCheckout(root, expectedCommit string) (bool, error) {
	head, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	status, err := gitOutput(root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(head)) == expectedCommit && len(bytes.TrimSpace(status)) == 0, nil
}

func gitOutput(root string, arguments ...string) ([]byte, error) {
	commandArguments := append([]string{"-C", root}, arguments...)
	// #nosec G204 -- git is fixed and arguments are passed without shell evaluation.
	command := exec.Command("git", commandArguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func preferredRuntimeVersion(selections []checker.RuntimeSelection, profile string) (string, error) {
	for _, selection := range selections {
		if selection.Profile != profile {
			continue
		}
		for _, version := range selection.Versions {
			if version.Disposition == "preferred" {
				return version.Version, nil
			}
		}
	}
	return "", fmt.Errorf("compatibility manifest has no preferred %s runtime", profile)
}

func collectAssets(directory, version string, targets []checker.SupportedTarget) ([]asset, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open release asset directory: %w", err)
	}
	defer func() {
		_ = root.Close()
	}()

	var assets []asset
	for _, target := range targets {
		name := fmt.Sprintf("golden-path_%s_%s_%s.tar.gz", version, target.OS, target.Architecture)
		info, err := root.Stat(name)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("required release asset %q is missing or invalid", name)
		}
		file, err := root.Open(name)
		if err != nil {
			return nil, fmt.Errorf("open release asset %q: %w", name, err)
		}
		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("hash release asset %q: %w", name, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close release asset %q: %w", name, err)
		}
		archiveDigest := hex.EncodeToString(hasher.Sum(nil))
		archive, err := root.Open(name)
		if err != nil {
			return nil, fmt.Errorf("reopen release asset %q: %w", name, err)
		}
		executableDigest, digestErr := digestExecutable(archive)
		closeErr := archive.Close()
		if digestErr != nil {
			return nil, fmt.Errorf("inspect release executable in %q: %w", name, digestErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close release asset %q: %w", name, closeErr)
		}
		sbomName := fmt.Sprintf("golden-path_%s_%s_%s.cdx.json", version, target.OS, target.Architecture)
		sbomInfo, err := root.Stat(sbomName)
		if err != nil || !sbomInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("required release SBOM %q is missing or invalid", sbomName)
		}
		sbomData, err := root.ReadFile(sbomName)
		if err != nil {
			return nil, fmt.Errorf("read release SBOM %q: %w", sbomName, err)
		}
		if err := validateSBOM(sbomData, name, archiveDigest); err != nil {
			return nil, fmt.Errorf("validate release SBOM %q: %w", sbomName, err)
		}
		sbomSum := sha256.Sum256(sbomData)
		assets = append(assets, asset{
			Name:             name,
			OS:               target.OS,
			Architecture:     target.Architecture,
			SHA256:           archiveDigest,
			ExecutableSHA256: executableDigest,
			Size:             info.Size(),
			SBOM: sbom{
				Name:        sbomName,
				Format:      "CycloneDX",
				SpecVersion: "1.6",
				SHA256:      hex.EncodeToString(sbomSum[:]),
				Size:        sbomInfo.Size(),
			},
		})
	}
	sort.Slice(assets, func(left, right int) bool { return assets[left].Name < assets[right].Name })
	return assets, nil
}

func digestExecutable(reader io.Reader) (string, error) {
	compressed, err := gzip.NewReader(reader)
	if err != nil {
		return "", err
	}
	defer func() { _ = compressed.Close() }()
	archive := tar.NewReader(compressed)
	for {
		header, nextErr := archive.Next()
		if nextErr == io.EOF {
			return "", fmt.Errorf("golden-path/golden-path is missing")
		}
		if nextErr != nil {
			return "", nextErr
		}
		if header.Name != "golden-path/golden-path" {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 {
			return "", fmt.Errorf("golden-path/golden-path is not a regular non-empty file")
		}
		hasher := sha256.New()
		if _, err := io.CopyN(hasher, archive, header.Size); err != nil {
			return "", err
		}
		return hex.EncodeToString(hasher.Sum(nil)), nil
	}
}

func validateSBOM(data []byte, archiveName, archiveDigest string) error {
	var document struct {
		BOMFormat   string `json:"bomFormat"`
		SpecVersion string `json:"specVersion"`
		Metadata    struct {
			Component struct {
				Name   string `json:"name"`
				Hashes []struct {
					Algorithm string `json:"alg"`
					Content   string `json:"content"`
				} `json:"hashes"`
			} `json:"component"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	if document.BOMFormat != "CycloneDX" ||
		document.SpecVersion != "1.6" ||
		document.Metadata.Component.Name != archiveName {
		return fmt.Errorf("SBOM identity does not match release asset")
	}
	for _, hash := range document.Metadata.Component.Hashes {
		if hash.Algorithm == "SHA-256" && hash.Content == archiveDigest {
			return nil
		}
	}
	return fmt.Errorf("SBOM does not bind the release asset digest")
}

func fullCommit(value string) bool {
	if len(value) != 40 || strings.ToLower(value) != value {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
