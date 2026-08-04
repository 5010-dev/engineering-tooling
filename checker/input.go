package checker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxInputBytes = 2 << 20

var errNotFound = errors.New("repository input not found")
var metadataComponentPathPattern = regexp.MustCompile(`^(?:\.|[a-z0-9][a-z0-9._-]*(?:/[a-z0-9][a-z0-9._-]*)*)$`)
var nativeProfiles = map[string]bool{
	"node-typescript":          true,
	"python":                   true,
	"go":                       true,
	"rust":                     true,
	"zig":                      true,
	"zig-toolchain":            true,
	"infrastructure-aws-cdk":   true,
	"infrastructure-terraform": true,
	"infrastructure-opentofu":  true,
	"infrastructure-pulumi":    true,
}

func readRepositoryFile(root, name string) ([]byte, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	relativeName := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(relativeName) || relativeName == ".." || strings.HasPrefix(relativeName, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes repository root: %q", name)
	}
	rootFS, err := os.OpenRoot(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("open repository root: %w", err)
	}
	defer func() {
		_ = rootFS.Close()
	}()

	info, err := rootFS.Lstat(relativeName)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%q is a symbolic link; checker inputs must be regular files", name)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", name)
	}
	if info.Size() > maxInputBytes {
		return nil, fmt.Errorf("%q exceeds the %d-byte input limit", name, maxInputBytes)
	}

	file, err := rootFS.Open(relativeName)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", name, err)
	}
	defer func() {
		_ = file.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", name, err)
	}
	if len(data) > maxInputBytes {
		return nil, fmt.Errorf("%q exceeds the %d-byte input limit", name, maxInputBytes)
	}
	return data, nil
}

func decodeYAML(data []byte) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("YAML input must contain exactly one document")
		}
		return nil, fmt.Errorf("parse trailing YAML: %w", err)
	}
	if len(document.Content) != 1 {
		return nil, errors.New("YAML input must contain exactly one root value")
	}
	value, err := yamlNodeToJSON(document.Content[0], "$")
	if err != nil {
		return nil, err
	}
	result, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("serialize YAML data model: %w", err)
	}
	return result, nil
}

func yamlNodeToJSON(node *yaml.Node, location string) (any, error) {
	if node.Alias != nil || node.Kind == yaml.AliasNode {
		return nil, fmt.Errorf("%s: YAML aliases are not supported", location)
	}
	if node.Tag != "" && !strings.HasPrefix(node.Tag, "tag:yaml.org,2002:") && !strings.HasPrefix(node.Tag, "!!") {
		return nil, fmt.Errorf("%s: YAML tag %q is not supported", location, node.Tag)
	}

	switch node.Kind {
	case yaml.MappingNode:
		result := make(map[string]any, len(node.Content)/2)
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			keyNode := node.Content[index]
			if keyNode.Kind != yaml.ScalarNode || keyNode.Tag != "!!str" {
				return nil, fmt.Errorf("%s: mapping keys must be strings", location)
			}
			key := keyNode.Value
			if _, ok := seen[key]; ok {
				return nil, fmt.Errorf("%s: duplicate mapping key %q", location, key)
			}
			seen[key] = struct{}{}
			value, err := yamlNodeToJSON(node.Content[index+1], location+"."+key)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, len(node.Content))
		for index, child := range node.Content {
			value, err := yamlNodeToJSON(child, fmt.Sprintf("%s[%d]", location, index))
			if err != nil {
				return nil, err
			}
			result[index] = value
		}
		return result, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str", "!!timestamp":
			return node.Value, nil
		case "!!bool":
			value, err := strconv.ParseBool(node.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: invalid boolean: %w", location, err)
			}
			return value, nil
		case "!!int":
			value, err := strconv.ParseInt(node.Value, 0, 64)
			if err != nil {
				return nil, fmt.Errorf("%s: invalid integer: %w", location, err)
			}
			return value, nil
		case "!!float":
			value, err := strconv.ParseFloat(node.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("%s: invalid number: %w", location, err)
			}
			return value, nil
		case "!!null":
			return nil, nil
		default:
			return nil, fmt.Errorf("%s: YAML scalar tag %q is not supported", location, node.Tag)
		}
	default:
		return nil, fmt.Errorf("%s: YAML node kind %d is not supported", location, node.Kind)
	}
}

func loadMetadata(root string) (Metadata, error) {
	data, err := readRepositoryFile(root, ".github/golden-path.yaml")
	if err != nil {
		return Metadata{}, err
	}
	jsonData, err := decodeYAML(data)
	if err != nil {
		return Metadata{}, err
	}
	if err := validateJSON("schemas/golden-path-metadata-v1.schema.json", jsonData); err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(jsonData, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	if extension, present := metadata.Extensions["dev.fiftyten.generator"]; present {
		data, marshalErr := json.Marshal(extension)
		if marshalErr != nil {
			return Metadata{}, fmt.Errorf("decode generator metadata extension: %w", marshalErr)
		}
		var generatorMetadata struct {
			Components []MetadataComponent `json:"components"`
		}
		if err := json.Unmarshal(data, &generatorMetadata); err != nil {
			return Metadata{}, fmt.Errorf("decode generator metadata extension: %w", err)
		}
		metadata.Components = generatorMetadata.Components
	}
	if err := validateComponentMetadata(metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func validateComponentMetadata(metadata Metadata) error {
	if len(metadata.Components) == 0 {
		return nil
	}
	paths := map[string]bool{}
	profiles, artifacts, capabilities := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, component := range metadata.Components {
		if !metadataComponentPathPattern.MatchString(component.Path) || len(component.Profiles) == 0 || len(component.ArtifactTypes) == 0 ||
			!uniqueNonEmpty(component.Profiles) || !uniqueNonEmpty(component.ArtifactTypes) || !uniqueNonEmpty(component.Capabilities) {
			return fmt.Errorf("metadata component declaration is invalid")
		}
		if paths[component.Path] {
			return fmt.Errorf("metadata component paths must be unique")
		}
		paths[component.Path] = true
		for _, profile := range component.Profiles {
			profiles[profile] = true
		}
		for _, artifact := range component.ArtifactTypes {
			artifacts[artifact] = true
		}
		for _, capability := range component.Capabilities {
			capabilities[capability] = true
		}
	}
	if len(metadata.Components) > 1 && paths["."] {
		return fmt.Errorf("metadata cannot mix a root component with nested components")
	}
	for left := range paths {
		for right := range paths {
			if left < right && (strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")) {
				return fmt.Errorf("metadata component paths overlap")
			}
		}
	}
	if strings.Join(sortedEnabledKeys(profiles), "\x00") != strings.Join(sortedCopy(metadata.Profiles), "\x00") ||
		strings.Join(sortedEnabledKeys(artifacts), "\x00") != strings.Join(sortedCopy(metadata.ArtifactTypes), "\x00") ||
		strings.Join(sortedEnabledKeys(capabilities), "\x00") != strings.Join(sortedCopy(metadata.Capabilities), "\x00") {
		return fmt.Errorf("metadata aggregate declarations must equal the union of component declarations")
	}
	return nil
}

func loadNativeRoots(root string, metadata Metadata) ([]MetadataNativeRoot, bool, error) {
	data, err := readRepositoryFile(root, ".github/golden-path-native-roots.yaml")
	if errors.Is(err, errNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	jsonData, err := decodeYAML(data)
	if err != nil {
		return nil, true, err
	}
	if err := validateJSON("schemas/golden-path-native-roots-v1.schema.json", jsonData); err != nil {
		return nil, true, err
	}
	var declaration NativeRootsFile
	if err := json.Unmarshal(jsonData, &declaration); err != nil {
		return nil, true, fmt.Errorf("decode native roots: %w", err)
	}
	if err := validateNativeRoots(metadata, declaration.Roots); err != nil {
		return nil, true, err
	}
	return declaration.Roots, true, nil
}

func validateNativeRoots(metadata Metadata, roots []MetadataNativeRoot) error {
	if len(roots) == 0 {
		return fmt.Errorf("native root declaration must contain at least one root")
	}
	metadataProfiles := enabledSet(metadata.Profiles)
	ids := map[string]bool{}
	coveredProfiles := map[string]bool{}

	for _, root := range roots {
		if root.ID == "" || ids[root.ID] {
			return fmt.Errorf("native root IDs must be non-empty and unique")
		}
		ids[root.ID] = true
		if !metadataComponentPathPattern.MatchString(root.Path) ||
			len(root.Profiles) == 0 || !uniqueNonEmpty(root.Profiles) {
			return fmt.Errorf("native root %q declaration is invalid", root.ID)
		}
		if !isSubset(root.Profiles, metadataProfiles) {
			return fmt.Errorf("native root %q profiles must be a subset of aggregate metadata", root.ID)
		}
		for _, profile := range root.Profiles {
			if !nativeProfiles[profile] {
				return fmt.Errorf("native root %q contains a non-native profile", root.ID)
			}
			coveredProfiles[profile] = true
		}
	}

	for profile := range metadataProfiles {
		if nativeProfiles[profile] && !coveredProfiles[profile] {
			return fmt.Errorf("native profile %q has no declared root", profile)
		}
	}
	for leftIndex, left := range roots {
		for _, right := range roots[leftIndex+1:] {
			if pathsOverlap(left.Path, right.Path) && intersects(left.Profiles, right.Profiles) {
				return fmt.Errorf("native roots %q and %q overlap for the same profile", left.ID, right.ID)
			}
		}
	}
	return nil
}

func enabledSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func isSubset(values []string, allowed map[string]bool) bool {
	for _, value := range values {
		if !allowed[value] {
			return false
		}
	}
	return true
}

func pathsOverlap(left, right string) bool {
	return left == right || left == "." || right == "." ||
		strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func uniqueNonEmpty(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func sortedEnabledKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func loadExceptions(root string) (ExceptionsFile, bool, error) {
	data, err := readRepositoryFile(root, ".github/golden-path-exceptions.yaml")
	if errors.Is(err, errNotFound) {
		return ExceptionsFile{SchemaVersion: "golden-path-exceptions/v1"}, false, nil
	}
	if err != nil {
		return ExceptionsFile{}, false, err
	}
	jsonData, err := decodeYAML(data)
	if err != nil {
		return ExceptionsFile{}, true, err
	}
	if err := validateJSON("schemas/golden-path-exceptions-v1.schema.json", jsonData); err != nil {
		return ExceptionsFile{}, true, err
	}
	var exceptions ExceptionsFile
	if err := json.Unmarshal(jsonData, &exceptions); err != nil {
		return ExceptionsFile{}, true, fmt.Errorf("decode exceptions: %w", err)
	}
	return exceptions, true, nil
}
