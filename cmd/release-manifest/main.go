package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/5010-dev/engineering-tooling/checker"
)

type manifest struct {
	SchemaVersion   string        `json:"schemaVersion"`
	ReleaseVersion  string        `json:"releaseVersion"`
	StandardVersion string        `json:"standardVersion"`
	ContractVersion string        `json:"contractVersion"`
	SchemaVersions  []string      `json:"schemaVersions"`
	Source          source        `json:"source"`
	CatalogDigest   string        `json:"catalogDigest"`
	Compatibility   string        `json:"compatibilityManifest"`
	Snapshot        string        `json:"standardSnapshotManifest"`
	Assets          []asset       `json:"assets"`
	Build           buildEvidence `json:"build"`
	Provenance      provenance    `json:"provenance"`
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

	assets, err := collectAssets(*dist, checker.Version)
	if err != nil {
		return err
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
		CatalogDigest: "sha256:c2ec366495c5f2aa124a886152aadd1f4f0d1b7dcb34beb674c9dfa0db4b86ac",
		Compatibility: "compatibility-manifest.json",
		Snapshot:      "standard-snapshot-manifest.json",
		Assets:        assets,
		Build: buildEvidence{
			Toolchain: "go1.26.5",
			Commands: []string{
				"just ci <explicit-utc-evaluation-time>",
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

func collectAssets(directory, version string) ([]asset, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open release asset directory: %w", err)
	}
	defer func() {
		_ = root.Close()
	}()

	var assets []asset
	for _, target := range []struct {
		os   string
		arch string
	}{
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
	} {
		name := fmt.Sprintf("golden-path_%s_%s_%s.tar.gz", version, target.os, target.arch)
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
		assets = append(assets, asset{
			Name:         name,
			OS:           target.os,
			Architecture: target.arch,
			SHA256:       hex.EncodeToString(hasher.Sum(nil)),
			Size:         info.Size(),
		})
	}
	sort.Slice(assets, func(left, right int) bool { return assets[left].Name < assets[right].Name })
	return assets, nil
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
