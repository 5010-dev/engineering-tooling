package checker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/5010-dev/engineering-tooling/standards"
)

const snapshotRoot = "snapshots/2026.07"

type snapshotManifest struct {
	SchemaVersion   string `json:"schemaVersion"`
	StandardVersion string `json:"standardVersion"`
	ContractVersion string `json:"contractVersion"`
	Source          struct {
		Commit string `json:"commit"`
	} `json:"source"`
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
	if manifest.StandardVersion != StandardVersion || manifest.ContractVersion != ContractVersion {
		return Catalog{}, "", fmt.Errorf("bundled snapshot compatibility mismatch")
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

	for path, want := range manifest.Files {
		data, readErr := readSnapshot(path)
		if readErr != nil {
			return Catalog{}, "", readErr
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != want {
			return Catalog{}, "", fmt.Errorf("bundled snapshot digest mismatch for %q", path)
		}
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
	return catalog, "sha256:" + hex.EncodeToString(sum[:]), nil
}
