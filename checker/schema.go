package checker

import (
	"encoding/json"
	"fmt"

	"github.com/dlclark/regexp2"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type offlineLoader struct{}

func (offlineLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema resource %q is unavailable in offline mode", url)
}

type ecmaRegexp regexp2.Regexp

func (regexp *ecmaRegexp) MatchString(value string) bool {
	matched, err := (*regexp2.Regexp)(regexp).MatchString(value)
	return err == nil && matched
}

func (regexp *ecmaRegexp) String() string {
	return (*regexp2.Regexp)(regexp).String()
}

func compileECMARegexp(expression string) (jsonschema.Regexp, error) {
	regexp, err := regexp2.Compile(expression, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	return (*ecmaRegexp)(regexp), nil
}

func validateJSON(schemaPath string, data []byte) error {
	schemaBytes, err := readSnapshot(schemaPath)
	if err != nil {
		return err
	}
	var schemaDocument any
	if err := json.Unmarshal(schemaBytes, &schemaDocument); err != nil {
		return fmt.Errorf("decode bundled schema %q: %w", schemaPath, err)
	}
	var instance any
	if err := json.Unmarshal(data, &instance); err != nil {
		return fmt.Errorf("decode JSON data model: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	compiler.UseRegexpEngine(compileECMARegexp)
	compiler.UseLoader(offlineLoader{})
	resource := "urn:5010-dev:bundled:" + schemaPath
	if err := compiler.AddResource(resource, schemaDocument); err != nil {
		return fmt.Errorf("add bundled schema %q: %w", schemaPath, err)
	}
	schema, err := compiler.Compile(resource)
	if err != nil {
		return fmt.Errorf("compile bundled schema %q: %w", schemaPath, err)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}
	return nil
}
