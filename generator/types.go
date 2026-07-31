// Package generator materializes and upgrades versioned Golden Path assets.
package generator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/5010-dev/engineering-tooling/checker"
	bundlefs "github.com/5010-dev/engineering-tooling/templates"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

const (
	RequestSchema = "golden-path-generator-request/v1"
	AssetsSchema  = "golden-path-generated-assets/v1"
	PlanSchema    = "golden-path-materialization-plan/v1"
)

var (
	identifierPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	componentPathPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(?:/[a-z0-9][a-z0-9._-]*)*$`)
	fullCommitPattern    = regexp.MustCompile(`^[a-f0-9]{40}$`)
	exactVersionPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

type Request struct {
	SchemaVersion string      `json:"schemaVersion" yaml:"schemaVersion"`
	Layout        string      `json:"layout" yaml:"layout"`
	ProjectName   string      `json:"projectName" yaml:"projectName"`
	ProjectSlug   string      `json:"projectSlug" yaml:"projectSlug"`
	Components    []Component `json:"components" yaml:"components"`
}

type Component struct {
	Name          string   `json:"name" yaml:"name"`
	Path          string   `json:"path" yaml:"path"`
	Profiles      []string `json:"profiles" yaml:"profiles"`
	ArtifactTypes []string `json:"artifactTypes" yaml:"artifactTypes"`
	ModulePath    string   `json:"modulePath,omitempty" yaml:"modulePath,omitempty"`
}

type Bundle struct {
	SchemaVersion        string            `json:"schemaVersion"`
	StandardVersion      string            `json:"standardVersion"`
	ContractVersion      string            `json:"contractVersion"`
	AssetBundleVersion   string            `json:"assetBundleVersion"`
	MiseMinimumVersion   string            `json:"miseMinimumVersion"`
	PNPMArchiveSHA256    string            `json:"pnpmArchiveSHA256"`
	PNPMExecutableSHA256 string            `json:"pnpmExecutableSHA256"`
	Profiles             []string          `json:"profiles"`
	Tools                map[string]string `json:"tools"`
}

type ReleaseManifest struct {
	SchemaVersion   string         `json:"schemaVersion"`
	ReleaseVersion  string         `json:"releaseVersion"`
	StandardVersion string         `json:"standardVersion"`
	ContractVersion string         `json:"contractVersion"`
	Source          ReleaseSource  `json:"source"`
	Assets          []ReleaseAsset `json:"assets"`
}

type ReleaseSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Tag        string `json:"tag"`
}

type ReleaseAsset struct {
	Name             string `json:"name"`
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	SHA256           string `json:"sha256"`
	ExecutableSHA256 string `json:"executableSHA256"`
}

type File struct {
	Path    string      `json:"path"`
	Mode    fs.FileMode `json:"-"`
	Content []byte      `json:"-"`
	Source  string      `json:"source"`
}

type FileChange struct {
	Path           string `json:"path"`
	Status         string `json:"status"`
	Source         string `json:"source"`
	DesiredMode    string `json:"desiredMode,omitempty"`
	CurrentMode    string `json:"currentMode,omitempty"`
	PreviousMode   string `json:"previousGeneratedMode,omitempty"`
	DesiredSHA256  string `json:"desiredSHA256,omitempty"`
	CurrentSHA256  string `json:"currentSHA256,omitempty"`
	PreviousSHA256 string `json:"previousGeneratedSHA256,omitempty"`
}

type Plan struct {
	SchemaVersion      string       `json:"schemaVersion"`
	Operation          string       `json:"operation"`
	StandardVersion    string       `json:"standardVersion"`
	AssetBundleVersion string       `json:"assetBundleVersion"`
	ReleaseVersion     string       `json:"releaseVersion"`
	RequestSHA256      string       `json:"requestSHA256"`
	Changes            []FileChange `json:"changes"`
	ConflictCount      int          `json:"conflictCount"`
}

type GeneratedAsset struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
	Source string `json:"source"`
}

type AssetManifest struct {
	SchemaVersion      string           `json:"schemaVersion"`
	StandardVersion    string           `json:"standardVersion"`
	AssetBundleVersion string           `json:"assetBundleVersion"`
	ReleaseVersion     string           `json:"releaseVersion"`
	Source             ReleaseSource    `json:"source"`
	RequestSHA256      string           `json:"requestSHA256"`
	Files              []GeneratedAsset `json:"files"`
}

func DecodeRequest(reader io.Reader) (Request, error) {
	var request Request
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&request); err != nil {
		return Request{}, fmt.Errorf("decode generator request: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Request{}, fmt.Errorf("generator request must contain exactly one document")
	}
	if err := validateRequest(&request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func DecodeReleaseManifest(reader io.Reader) (ReleaseManifest, error) {
	var manifest ReleaseManifest
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReleaseManifest{}, fmt.Errorf("release manifest must contain exactly one JSON document")
	}
	return manifest, nil
}

func LoadBundle() (Bundle, error) {
	data, err := bundlefs.Files.ReadFile("bundle.json")
	if err != nil {
		return Bundle{}, fmt.Errorf("read embedded template bundle: %w", err)
	}
	var bundle Bundle
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return Bundle{}, fmt.Errorf("decode embedded template bundle: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Bundle{}, fmt.Errorf("embedded template bundle must contain exactly one JSON document")
	}
	if bundle.AssetBundleVersion != checker.Version {
		return Bundle{}, fmt.Errorf("embedded template bundle version %q does not match executable version %q", bundle.AssetBundleVersion, checker.Version)
	}
	if bundle.SchemaVersion != "golden-path-template-bundle/v1" || bundle.StandardVersion != checker.StandardVersion || bundle.ContractVersion != checker.ContractVersion {
		return Bundle{}, fmt.Errorf("embedded template bundle identity is incompatible with the executable")
	}
	if bundle.MiseMinimumVersion == "" || len(bundle.Profiles) == 0 || len(bundle.Tools) == 0 {
		return Bundle{}, fmt.Errorf("embedded template bundle is incomplete")
	}
	if !fullDigest(bundle.PNPMArchiveSHA256) || !fullDigest(bundle.PNPMExecutableSHA256) {
		return Bundle{}, fmt.Errorf("embedded template bundle does not bind the pnpm artifact and executable digests")
	}
	profileCatalog := stringSet(bundle.Profiles)
	for _, profile := range []string{
		"documentation", "go", "node-typescript", "python", "rust", "zig", "zig-toolchain",
		"infrastructure-aws-cdk", "infrastructure-terraform", "infrastructure-opentofu", "infrastructure-pulumi",
	} {
		if !profileCatalog[profile] {
			return Bundle{}, fmt.Errorf("embedded template bundle does not declare required profile %q", profile)
		}
	}
	for _, tool := range []string{
		"actionlint", "aws-cdk", "aws-cdk-lib", "constructs", "eslint", "eslint-js", "github-cli", "go", "goimports",
		"golangci-lint", "just", "mypy", "node", "node-types", "opentofu", "pnpm", "prettier", "pulumi",
		"pulumi-sdk", "pytest", "python", "ruff", "rust", "terraform", "typescript", "typescript-eslint",
		"uv", "uv-build", "vitest", "zig",
	} {
		if !exactVersionPattern.MatchString(bundle.Tools[tool]) {
			return Bundle{}, fmt.Errorf("embedded template bundle does not pin required tool %q", tool)
		}
	}
	return bundle, nil
}

func validateRequest(request *Request) error {
	if request.SchemaVersion != RequestSchema {
		return fmt.Errorf("request schemaVersion must be %q", RequestSchema)
	}
	allowedLayouts := map[string]bool{"single": true, "monorepo": true, "polyglot": true, "documentation": true}
	if !allowedLayouts[request.Layout] {
		return fmt.Errorf("unsupported layout %q", request.Layout)
	}
	if request.ProjectName != strings.TrimSpace(request.ProjectName) {
		return fmt.Errorf("projectName must not have leading or trailing whitespace")
	}
	printable := true
	for _, character := range request.ProjectName {
		if !unicode.IsGraphic(character) || unicode.Is(unicode.Zl, character) || unicode.Is(unicode.Zp, character) {
			printable = false
			break
		}
	}
	projectNameLength := utf8.RuneCountInString(request.ProjectName)
	if projectNameLength < 2 || projectNameLength > 100 || !printable {
		return fmt.Errorf("projectName must contain 2-100 printable characters on one line")
	}
	if !identifierPattern.MatchString(request.ProjectSlug) {
		return fmt.Errorf("projectSlug must be a lowercase kebab-case identifier")
	}
	if len(request.Components) == 0 {
		return fmt.Errorf("at least one component is required")
	}
	if (request.Layout == "single" || request.Layout == "documentation") && (len(request.Components) != 1 || request.Components[0].Path != ".") {
		return fmt.Errorf("%s layout requires exactly one root component", request.Layout)
	}
	profileSet := map[string]bool{
		"documentation": true, "go": true, "node-typescript": true, "python": true,
		"rust": true, "zig": true, "zig-toolchain": true,
		"infrastructure-aws-cdk": true, "infrastructure-terraform": true,
		"infrastructure-opentofu": true, "infrastructure-pulumi": true,
	}
	artifactSet := map[string]bool{
		"application": true, "service": true, "library": true, "cli": true,
		"package": true, "binary": true, "container": true, "infrastructure": true,
		"tooling": true, "documentation": true,
	}
	seenNames := map[string]bool{}
	seenPaths := map[string]bool{}
	for index := range request.Components {
		component := &request.Components[index]
		if !identifierPattern.MatchString(component.Name) || len(component.Name) > 32 || seenNames[component.Name] {
			return fmt.Errorf("component name %q must be a unique lowercase kebab-case identifier of at most 32 characters", component.Name)
		}
		seenNames[component.Name] = true
		cleanPath, err := cleanRelativePath(component.Path)
		if err != nil || seenPaths[cleanPath] {
			return fmt.Errorf("component %q has an invalid or duplicate path", component.Name)
		}
		component.Path = cleanPath
		if cleanPath != "." && !componentPathPattern.MatchString(cleanPath) {
			return fmt.Errorf("component %q path must use lowercase repository-safe segments", component.Name)
		}
		seenPaths[cleanPath] = true
		if len(component.Profiles) == 0 || len(component.ArtifactTypes) == 0 {
			return fmt.Errorf("component %q must declare profiles and artifactTypes", component.Name)
		}
		profiles := sortedUnique(component.Profiles)
		artifacts := sortedUnique(component.ArtifactTypes)
		if len(profiles) != len(component.Profiles) || len(artifacts) != len(component.ArtifactTypes) {
			return fmt.Errorf("component %q profiles and artifactTypes must be unique and non-empty", component.Name)
		}
		component.Profiles = profiles
		component.ArtifactTypes = artifacts
		for _, profile := range component.Profiles {
			if !profileSet[profile] {
				return fmt.Errorf("component %q has unsupported profile %q", component.Name, profile)
			}
		}
		for _, artifact := range component.ArtifactTypes {
			if !artifactSet[artifact] {
				return fmt.Errorf("component %q has unsupported artifact type %q", component.Name, artifact)
			}
		}
		profileSelection := stringSet(component.Profiles)
		if (profileSelection["go"] || profileSelection["rust"]) && !hasSourceArtifact(*component) {
			return fmt.Errorf("component %q with the go or rust profile must declare a source-bearing artifact type", component.Name)
		}
		if profileSelection["zig"] && profileSelection["zig-toolchain"] {
			return fmt.Errorf("component %q cannot combine zig and zig-toolchain", component.Name)
		}
		languageCount := 0
		for _, profile := range []string{"documentation", "go", "node-typescript", "python", "rust", "zig", "zig-toolchain"} {
			if profileSelection[profile] {
				languageCount++
			}
		}
		if languageCount > 1 {
			return fmt.Errorf("component %q must keep independent language roots as separate components", component.Name)
		}
		iacCount := 0
		for _, profile := range []string{"infrastructure-aws-cdk", "infrastructure-terraform", "infrastructure-opentofu", "infrastructure-pulumi"} {
			if profileSelection[profile] {
				iacCount++
			}
		}
		if iacCount > 1 {
			return fmt.Errorf("component %q must select one infrastructure engine", component.Name)
		}
		if (profileSelection["infrastructure-aws-cdk"] || profileSelection["infrastructure-pulumi"]) && !profileSelection["node-typescript"] {
			return fmt.Errorf("component %q must combine the selected code-first IaC profile with node-typescript", component.Name)
		}
		if profileSelection["go"] {
			if component.ModulePath == "" {
				component.ModulePath = path.Join("github.com/5010-dev", request.ProjectSlug)
				if component.Path != "." {
					component.ModulePath = path.Join(component.ModulePath, component.Path)
				}
			}
			if err := module.CheckPath(component.ModulePath); err != nil {
				return fmt.Errorf("component %q has invalid Go modulePath", component.Name)
			}
		} else if component.ModulePath != "" {
			return fmt.Errorf("component %q sets modulePath without the go profile", component.Name)
		}
	}
	if len(request.Components) > 1 {
		for _, component := range request.Components {
			if component.Path == "." {
				return fmt.Errorf("multi-component layouts cannot mix a root component with nested components")
			}
		}
	}
	for left := range request.Components {
		for right := left + 1; right < len(request.Components); right++ {
			leftPath := request.Components[left].Path
			rightPath := request.Components[right].Path
			if strings.HasPrefix(leftPath, rightPath+"/") || strings.HasPrefix(rightPath, leftPath+"/") {
				return fmt.Errorf("component paths %q and %q overlap", leftPath, rightPath)
			}
		}
	}
	if request.Layout == "documentation" {
		component := request.Components[0]
		if len(component.Profiles) != 1 || component.Profiles[0] != "documentation" || len(component.ArtifactTypes) != 1 || component.ArtifactTypes[0] != "documentation" {
			return fmt.Errorf("documentation layout requires the documentation profile and artifact type only")
		}
	}
	sort.Slice(request.Components, func(left, right int) bool {
		return request.Components[left].Path < request.Components[right].Path
	})
	return nil
}

func ValidateRelease(bundle Bundle, release ReleaseManifest) error {
	if release.SchemaVersion != "golden-path-release-manifest/v1" || release.ReleaseVersion != bundle.AssetBundleVersion || release.StandardVersion != bundle.StandardVersion || release.ContractVersion != bundle.ContractVersion {
		return fmt.Errorf("release manifest is incompatible with embedded template bundle")
	}
	if release.Source.Repository != "https://github.com/5010-dev/engineering-tooling" || !fullCommitPattern.MatchString(release.Source.Commit) || release.Source.Tag != "v"+release.ReleaseVersion {
		return fmt.Errorf("release manifest source identity is invalid")
	}
	expected := map[string]bool{"darwin/amd64": true, "darwin/arm64": true, "linux/amd64": true, "linux/arm64": true}
	if len(release.Assets) != len(expected) {
		return fmt.Errorf("release manifest must bind exactly four supported archives")
	}
	for _, asset := range release.Assets {
		key := asset.OS + "/" + asset.Architecture
		if !expected[key] || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(asset.SHA256) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(asset.ExecutableSHA256) {
			continue
		}
		name := fmt.Sprintf("golden-path_%s_%s_%s.tar.gz", release.ReleaseVersion, asset.OS, asset.Architecture)
		if asset.Name == name {
			delete(expected, key)
		}
	}
	if len(expected) != 0 {
		return fmt.Errorf("release manifest must bind all four supported archives")
	}
	return nil
}

func cleanRelativePath(value string) (string, error) {
	if value == "" || strings.Contains(value, "\\") {
		return "", fmt.Errorf("empty or platform-specific path")
	}
	cleaned := path.Clean(value)
	if cleaned == "/" || strings.HasPrefix(cleaned, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != value {
		return "", fmt.Errorf("path must be a clean repository-relative path")
	}
	return cleaned, nil
}

func sortedUnique(values []string) []string {
	copyOfValues := append([]string(nil), values...)
	sort.Strings(copyOfValues)
	result := copyOfValues[:0]
	for _, value := range copyOfValues {
		if value == "" || len(result) > 0 && result[len(result)-1] == value {
			continue
		}
		result = append(result, value)
	}
	return result
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
