package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/5010-dev/engineering-tooling/checker"
)

type manifest struct {
	SchemaVersion     string                     `json:"schemaVersion"`
	ReleaseVersion    string                     `json:"releaseVersion"`
	StandardVersion   string                     `json:"standardVersion"`
	ContractVersion   string                     `json:"contractVersion"`
	SchemaVersions    []string                   `json:"schemaVersions"`
	Source            source                     `json:"source"`
	CatalogDigest     string                     `json:"catalogDigest"`
	SnapshotDigest    string                     `json:"snapshotAggregateDigest"`
	Compatibility     string                     `json:"compatibilityManifest"`
	Snapshot          string                     `json:"standardSnapshotManifest"`
	RuntimeSelections []checker.RuntimeSelection `json:"runtimeSelections"`
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
	Name         string `json:"name"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
	SBOM         sbom   `json:"sbom"`
}

type sbom struct {
	Name        string `json:"name"`
	Format      string `json:"format"`
	SpecVersion string `json:"specVersion"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

type buildEvidence struct {
	Toolchain string   `json:"toolchain"`
	Commands  []string `json:"commands"`
}

type provenance struct {
	Provider string `json:"provider"`
	Workflow string `json:"workflow"`
	Subject  string `json:"subject"`
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
	tag := flags.String("tag", "", "exact release tag")
	commit := flags.String("source-commit", "", "full source commit SHA")
	output := flags.String("output", "", "manifest output path")
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
	document := manifest{
		SchemaVersion:   "golden-path-release-manifest/v1",
		ReleaseVersion:  checker.Version,
		StandardVersion: checker.StandardVersion,
		ContractVersion: checker.ContractVersion,
		SchemaVersions: []string{
			"golden-path-metadata/v1",
			"golden-path-exceptions/v1",
			"golden-path-checker-output/v1",
			"golden-path-rule-catalog/v1",
			"runtime-support/v1",
		},
		Source: source{
			Repository: "https://github.com/5010-dev/engineering-tooling",
			Commit:     *commit,
			Tag:        *tag,
		},
		CatalogDigest:     standard.CatalogDigest,
		SnapshotDigest:    standard.SnapshotAggregateDigest,
		Compatibility:     "compatibility-manifest.json",
		Snapshot:          "standard-snapshot-manifest.json",
		RuntimeSelections: compatibility.RuntimeSelections,
		Assets:            assets,
		Build: buildEvidence{
			Toolchain: "go" + goVersion,
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
			Name:         name,
			OS:           target.OS,
			Architecture: target.Architecture,
			SHA256:       archiveDigest,
			Size:         info.Size(),
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
