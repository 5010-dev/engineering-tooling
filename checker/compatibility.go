package checker

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	bundledcompatibility "github.com/5010-dev/engineering-tooling/compatibility"
)

func loadCompatibility() (CompatibilityManifest, error) {
	var manifest CompatibilityManifest
	if err := json.Unmarshal(bundledcompatibility.Manifest, &manifest); err != nil {
		return CompatibilityManifest{}, fmt.Errorf("decode bundled compatibility manifest: %w", err)
	}
	if manifest.SchemaVersion != "golden-path-checker-compatibility/v1" ||
		manifest.CheckerVersion != Version ||
		manifest.Lifecycle != "prerelease" ||
		!slices.Equal(manifest.Enforcement, []string{"report-only"}) {
		return CompatibilityManifest{}, fmt.Errorf("bundled checker compatibility identity mismatch")
	}
	if manifest.ExceptionExpiryWarningDays < 1 || manifest.ExceptionExpiryWarningDays > 365 {
		return CompatibilityManifest{}, fmt.Errorf("bundled exception expiry warning window is invalid")
	}
	if len(manifest.Standards) != 1 {
		return CompatibilityManifest{}, fmt.Errorf("bundled compatibility manifest must select one standard")
	}
	standard := manifest.Standards[0]
	if standard.StandardVersion != StandardVersion ||
		standard.ContractVersion != ContractVersion ||
		standard.Snapshot != "standards/"+snapshotRoot+"/manifest.json" ||
		standard.SourceCommit != SnapshotSourceCommit ||
		standard.CatalogDigest != CatalogDigest ||
		standard.SnapshotAggregateDigest != SnapshotAggregateDigest {
		return CompatibilityManifest{}, fmt.Errorf("bundled standard compatibility identity mismatch")
	}

	profiles := make(map[string]struct{}, len(manifest.RuntimeSelections))
	expectedTools := map[string]string{
		"node-typescript": "node",
		"python":          "python",
		"go":              "go",
		"rust":            "rust",
		"zig":             "zig",
		"zig-toolchain":   "zig",
	}
	validDispositions := map[string]bool{
		"preferred":          true,
		"supported":          true,
		"compatibility-only": true,
		"blocked":            true,
	}
	for _, selection := range manifest.RuntimeSelections {
		if expectedTools[selection.Profile] != selection.Tool {
			return CompatibilityManifest{}, fmt.Errorf("runtime selection %q has an invalid tool", selection.Profile)
		}
		if _, duplicate := profiles[selection.Profile]; duplicate {
			return CompatibilityManifest{}, fmt.Errorf("duplicate runtime selection for profile %q", selection.Profile)
		}
		profiles[selection.Profile] = struct{}{}
		if len(selection.Versions) == 0 {
			return CompatibilityManifest{}, fmt.Errorf("runtime selection %q has no exact versions", selection.Profile)
		}
		versions := make(map[string]struct{}, len(selection.Versions))
		for _, version := range selection.Versions {
			if !exactStableVersion(version.Version) {
				return CompatibilityManifest{}, fmt.Errorf("runtime selection %q has a non-exact version", selection.Profile)
			}
			if _, duplicate := versions[version.Version]; duplicate {
				return CompatibilityManifest{}, fmt.Errorf("runtime selection %q repeats version %q", selection.Profile, version.Version)
			}
			versions[version.Version] = struct{}{}
			if !validDispositions[version.Disposition] {
				return CompatibilityManifest{}, fmt.Errorf("runtime selection %q has an invalid disposition", selection.Profile)
			}
			if version.SupportEndsAt != "" {
				if _, err := time.Parse("2006-01-02", version.SupportEndsAt); err != nil {
					return CompatibilityManifest{}, fmt.Errorf("runtime selection %q has an invalid support deadline", selection.Profile)
				}
			}
		}
	}
	targets := make(map[string]struct{}, len(manifest.SupportedTargets))
	for _, target := range manifest.SupportedTargets {
		key := target.OS + "/" + target.Architecture
		if (target.OS != "darwin" && target.OS != "linux") ||
			(target.Architecture != "amd64" && target.Architecture != "arm64") {
			return CompatibilityManifest{}, fmt.Errorf("unsupported release target %q", key)
		}
		if _, duplicate := targets[key]; duplicate {
			return CompatibilityManifest{}, fmt.Errorf("duplicate release target %q", key)
		}
		targets[key] = struct{}{}
	}
	if len(targets) == 0 {
		return CompatibilityManifest{}, fmt.Errorf("compatibility manifest has no supported targets")
	}
	return manifest, nil
}

// BundledCompatibility returns the verified release compatibility manifest.
func BundledCompatibility() (CompatibilityManifest, error) {
	return loadCompatibility()
}

func exactStableVersion(value string) bool {
	return exactPatchVersion(value) && !strings.ContainsAny(value, "-+")
}
