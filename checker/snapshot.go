package checker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/5010-dev/engineering-tooling/standards"
)

const snapshotRoot = "snapshots/2026.08"

const snapshotAggregateDefinition = "SHA-256 of UTF-8 lines sorted by snapshot-relative path, each formatted as <file-sha256>  standards/snapshots/2026.08/<snapshot-relative-path>\\n, excluding manifest.json"

type snapshotManifest struct {
	SchemaVersion   string `json:"schemaVersion"`
	StandardVersion string `json:"standardVersion"`
	ContractVersion string `json:"contractVersion"`
	Source          struct {
		Repository string `json:"repository"`
		Commit     string `json:"commit"`
		Path       string `json:"path"`
		GitTree    string `json:"gitTree"`
	} `json:"source"`
	Aggregate struct {
		Algorithm  string `json:"algorithm"`
		Definition string `json:"definition"`
		Digest     string `json:"digest"`
	} `json:"aggregate"`
	Files map[string]string `json:"files"`
}

func readSnapshot(path string) ([]byte, error) {
	data, err := standards.Snapshots.ReadFile(snapshotRoot + "/" + path)
	if err != nil {
		return nil, fmt.Errorf("read bundled snapshot %q: %w", path, err)
	}
	return data, nil
}

func loadSnapshot() (Catalog, string, error) {
	var manifest snapshotManifest
	manifestBytes, err := readSnapshot("manifest.json")
	if err != nil {
		return Catalog{}, "", err
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Catalog{}, "", fmt.Errorf("decode bundled snapshot manifest: %w", err)
	}
	if manifest.SchemaVersion != "golden-path-standard-snapshot/v1" ||
		manifest.StandardVersion != StandardVersion ||
		manifest.ContractVersion != ContractVersion ||
		manifest.Source.Repository != "https://github.com/5010-dev/.github" ||
		manifest.Source.Commit != SnapshotSourceCommit ||
		manifest.Source.Path != "docs/standards/developer-tooling" ||
		manifest.Source.GitTree != SnapshotSourceTree {
		return Catalog{}, "", fmt.Errorf("bundled snapshot compatibility mismatch")
	}
	if manifest.Aggregate.Algorithm != "sha256" || manifest.Aggregate.Definition != snapshotAggregateDefinition {
		return Catalog{}, "", fmt.Errorf("bundled snapshot aggregate contract mismatch")
	}

	inventory := make(map[string]struct{}, len(manifest.Files))
	err = fs.WalkDir(standards.Snapshots, snapshotRoot, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || name == snapshotRoot+"/manifest.json" {
			return nil
		}
		relative := strings.TrimPrefix(name, snapshotRoot+"/")
		if _, declared := manifest.Files[relative]; !declared {
			return fmt.Errorf("bundled snapshot contains undeclared file %q", relative)
		}
		inventory[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return Catalog{}, "", fmt.Errorf("inventory bundled snapshot: %w", err)
	}
	if len(inventory) != len(manifest.Files) {
		return Catalog{}, "", fmt.Errorf("bundled snapshot manifest inventory is incomplete")
	}

	names := make([]string, 0, len(manifest.Files))
	for name := range manifest.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	aggregate := sha256.New()
	for _, name := range names {
		want := manifest.Files[name]
		data, readErr := readSnapshot(name)
		if readErr != nil {
			return Catalog{}, "", readErr
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != want {
			return Catalog{}, "", fmt.Errorf("bundled snapshot digest mismatch for %q", name)
		}
		_, _ = fmt.Fprintf(
			aggregate,
			"%s  standards/%s/%s\n",
			hex.EncodeToString(got[:]),
			snapshotRoot,
			name,
		)
	}
	aggregateDigest := hex.EncodeToString(aggregate.Sum(nil))
	if aggregateDigest != manifest.Aggregate.Digest ||
		"sha256:"+aggregateDigest != SnapshotAggregateDigest {
		return Catalog{}, "", fmt.Errorf("bundled snapshot aggregate digest mismatch")
	}

	catalogBytes, err := readSnapshot("rules/catalog.v1.json")
	if err != nil {
		return Catalog{}, "", err
	}
	if err := validateJSON("schemas/golden-path-rule-catalog-v1.schema.json", catalogBytes); err != nil {
		return Catalog{}, "", fmt.Errorf("validate bundled rule catalog: %w", err)
	}
	runtimeBytes, err := readSnapshot("rules/runtime-support.v1.json")
	if err != nil {
		return Catalog{}, "", err
	}
	if err := validateJSON("schemas/runtime-support-v1.schema.json", runtimeBytes); err != nil {
		return Catalog{}, "", fmt.Errorf("validate bundled runtime support catalog: %w", err)
	}

	var catalog Catalog
	if err := json.Unmarshal(catalogBytes, &catalog); err != nil {
		return Catalog{}, "", fmt.Errorf("decode bundled rule catalog: %w", err)
	}
	sum := sha256.Sum256(catalogBytes)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if digest != CatalogDigest {
		return Catalog{}, "", fmt.Errorf("bundled catalog digest does not match checker identity")
	}
	return catalog, digest, nil
}
