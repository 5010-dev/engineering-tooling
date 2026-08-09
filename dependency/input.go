package dependency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/5010-dev/engineering-tooling/standards"
	"github.com/dlclark/regexp2"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const maxRepositoryInput = 2 << 20

var errInputNotFound = errors.New("repository input not found")

type offlineSchemaLoader struct{}

func (offlineSchemaLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema resource %q is unavailable", url)
}

type ecmaRegexp regexp2.Regexp

func (regexp *ecmaRegexp) MatchString(value string) bool {
	matched, err := (*regexp2.Regexp)(regexp).MatchString(value)
	return err == nil && matched
}

func (regexp *ecmaRegexp) String() string { return (*regexp2.Regexp)(regexp).String() }

func compileECMA(expression string) (jsonschema.Regexp, error) {
	compiled, err := regexp2.Compile(expression, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	return (*ecmaRegexp)(compiled), nil
}

func readRegular(root, name string) ([]byte, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	relative := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes repository root: %q", name)
	}
	rootFS, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("open repository root: %w", err)
	}
	defer func() { _ = rootFS.Close() }()
	info, err := rootFS.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errInputNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxRepositoryInput {
		return nil, fmt.Errorf("%q must be a bounded regular file", name)
	}
	file, err := rootFS.Open(relative)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", name, err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxRepositoryInput+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", name, err)
	}
	if len(data) > maxRepositoryInput {
		return nil, fmt.Errorf("%q exceeds the input limit", name)
	}
	return data, nil
}

func decodeYAML(data []byte, destination any) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(destination); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("YAML input must contain exactly one document")
	}
	encoded, err := json.Marshal(destination)
	if err != nil {
		return nil, fmt.Errorf("encode YAML data model: %w", err)
	}
	return encoded, nil
}

func validateSchema(name string, data []byte) error {
	schemaBytes, err := standards.Snapshots.ReadFile("snapshots/2026.08.5/schemas/" + name)
	if err != nil {
		return fmt.Errorf("read bundled schema: %w", err)
	}
	var schemaDocument any
	if err := json.Unmarshal(schemaBytes, &schemaDocument); err != nil {
		return fmt.Errorf("decode bundled schema: %w", err)
	}
	var instance any
	if err := json.Unmarshal(data, &instance); err != nil {
		return fmt.Errorf("decode input data model: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseRegexpEngine(compileECMA)
	compiler.UseLoader(offlineSchemaLoader{})
	resource := "urn:5010-dev:dependency-schema:" + name
	if err := compiler.AddResource(resource, schemaDocument); err != nil {
		return fmt.Errorf("add bundled schema: %w", err)
	}
	schema, err := compiler.Compile(resource)
	if err != nil {
		return fmt.Errorf("compile bundled schema: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}
	return nil
}

func loadPolicy(root string) (Policy, []byte, error) {
	data, err := readRegular(root, ".github/golden-path-dependency-policy.yaml")
	if err != nil {
		return Policy{}, nil, err
	}
	var policy Policy
	jsonData, err := decodeYAML(data, &policy)
	if err != nil {
		return Policy{}, nil, err
	}
	if err := validateSchema("golden-path-dependency-policy-v1.schema.json", jsonData); err != nil {
		return Policy{}, nil, err
	}
	if policy.Adapter == "" {
		policy.Adapter = "dependabot"
	}
	return policy, data, nil
}

type metadataInput struct {
	SchemaVersion      string   `json:"schemaVersion" yaml:"schemaVersion"`
	ContractVersion    string   `json:"contractVersion" yaml:"contractVersion"`
	StandardVersion    string   `json:"standardVersion" yaml:"standardVersion"`
	AssetBundleVersion string   `json:"assetBundleVersion" yaml:"assetBundleVersion"`
	Applicability      any      `json:"applicability,omitempty" yaml:"applicability,omitempty"`
	Capabilities       []string `json:"capabilities" yaml:"capabilities"`
	Profiles           []string `json:"profiles" yaml:"profiles"`
	ArtifactTypes      []string `json:"artifactTypes" yaml:"artifactTypes"`
	Targets            []any    `json:"targets,omitempty" yaml:"targets,omitempty"`
	Extensions         struct {
		Generator struct {
			Components []struct {
				Path     string   `json:"path" yaml:"path"`
				Profiles []string `json:"profiles" yaml:"profiles"`
			} `json:"components" yaml:"components"`
		} `json:"dev.fiftyten.generator" yaml:"dev.fiftyten.generator"`
	} `json:"extensions" yaml:"extensions"`
}

func loadMetadataInput(root string) (metadataInput, error) {
	data, err := readRegular(root, ".github/golden-path.yaml")
	if err != nil {
		return metadataInput{}, err
	}
	var metadata metadataInput
	if json.Unmarshal(data, &metadata) == nil {
		return metadata, nil
	}
	if _, err := decodeYAML(data, &metadata); err != nil {
		return metadataInput{}, err
	}
	return metadata, nil
}

func loadNativeRootsInput(root string) ([]NativeRoot, bool, error) {
	data, err := readRegular(root, ".github/golden-path-native-roots.yaml")
	if errors.Is(err, errInputNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var document struct {
		SchemaVersion string       `json:"schemaVersion" yaml:"schemaVersion"`
		Roots         []NativeRoot `json:"roots" yaml:"roots"`
	}
	jsonData, err := decodeYAML(data, &document)
	if err != nil {
		return nil, true, err
	}
	if err := validateSchema("golden-path-native-roots-v1.schema.json", jsonData); err != nil {
		return nil, true, fmt.Errorf("native-root schema validation failed: %w", err)
	}
	sort.Slice(document.Roots, func(left, right int) bool { return document.Roots[left].ID < document.Roots[right].ID })
	return document.Roots, true, nil
}

type releaseUnit struct {
	ID       string   `json:"id"`
	Profiles []string `json:"profiles"`
}

func loadReleaseUnits(root, name string) (map[string]releaseUnit, error) {
	data, err := readRegular(root, name)
	if err != nil {
		return nil, err
	}
	var document struct {
		ReleaseUnits []releaseUnit `json:"releaseUnits"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		// Release-unit authorities are intentionally extensible. Decode only the
		// explicit ID/profile projection when extra repository-owned fields exist.
		if err := json.Unmarshal(data, &document); err != nil {
			return nil, fmt.Errorf("decode release units: %w", err)
		}
	}
	result := make(map[string]releaseUnit, len(document.ReleaseUnits))
	for _, unit := range document.ReleaseUnits {
		if unit.ID == "" || result[unit.ID].ID != "" {
			return nil, fmt.Errorf("release-unit IDs must be non-empty and unique")
		}
		result[unit.ID] = unit
	}
	return result, nil
}

func sha256Digest(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
