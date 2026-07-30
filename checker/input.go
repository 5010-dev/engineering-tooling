package checker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxInputBytes = 2 << 20

var errNotFound = errors.New("repository input not found")

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
	return metadata, nil
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
