package release_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

type cutoffManifest struct {
	SchemaVersion      string            `json:"schemaVersion"`
	CheckedAt          string            `json:"checkedAt"`
	StandardVersion    string            `json:"standardVersion"`
	ReleaseVersion     string            `json:"releaseVersion"`
	MiseMinimumVersion string            `json:"miseMinimumVersion"`
	IntegrityRecords   []integrityRecord `json:"integrityRecords"`
	ToolSelections     []toolSelection   `json:"toolSelections"`
	RuntimeSupport     []runtimeSupport  `json:"runtimeSupport"`
	WorkflowActions    []workflowAction  `json:"workflowActions"`
}

type sourceIntegrityManifest struct {
	SchemaVersion    string            `json:"schemaVersion"`
	CheckedAt        string            `json:"checkedAt"`
	StandardVersion  string            `json:"standardVersion"`
	ReleaseVersion   string            `json:"releaseVersion"`
	IntegrityRecords []integrityRecord `json:"integrityRecords"`
}

type integrityRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type toolSelection struct {
	ID                string   `json:"id"`
	Version           string   `json:"version"`
	IntegrityEvidence []string `json:"integrityEvidence"`
}

type runtimeSupport struct {
	Profile       string `json:"profile"`
	Tool          string `json:"tool"`
	Version       string `json:"version"`
	Disposition   string `json:"disposition"`
	SupportEndsAt string `json:"supportEndsAt"`
}

type workflowAction struct {
	Repository string `json:"repository"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
}

type workflowDocument struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Steps []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Name string         `yaml:"name"`
	With map[string]any `yaml:"with"`
	Run  string         `yaml:"run"`
}

type templateBundle struct {
	StandardVersion    string            `json:"standardVersion"`
	AssetBundleVersion string            `json:"assetBundleVersion"`
	MiseMinimumVersion string            `json:"miseMinimumVersion"`
	Tools              map[string]string `json:"tools"`
}

type compatibilityManifest struct {
	CheckerVersion    string   `json:"checkerVersion"`
	Lifecycle         string   `json:"lifecycle"`
	Enforcement       []string `json:"enforcement"`
	RuntimeSelections []struct {
		Profile  string `json:"profile"`
		Tool     string `json:"tool"`
		Versions []struct {
			Version       string `json:"version"`
			Disposition   string `json:"disposition"`
			SupportEndsAt string `json:"supportEndsAt"`
		} `json:"versions"`
	} `json:"runtimeSelections"`
}

func TestStableToolingCutoffIsCompleteAndConsistent(t *testing.T) {
	root, err := os.OpenRoot("..")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()

	cutoffData := readFile(t, root, "release/tooling-cutoff-2026-08-04.json")
	validateJSON(t, readFile(t, root, "release/golden-path-tooling-cutoff-v1.schema.json"), cutoffData)

	var cutoff cutoffManifest
	decodeJSON(t, cutoffData, &cutoff)
	if cutoff.SchemaVersion != "golden-path-tooling-cutoff/v1" {
		t.Fatalf("cutoff schema = %q", cutoff.SchemaVersion)
	}
	checkedAt, err := time.Parse(time.RFC3339, cutoff.CheckedAt)
	if err != nil || checkedAt.Location() != time.UTC {
		t.Fatalf("cutoff checkedAt must be canonical UTC: %q", cutoff.CheckedAt)
	}

	if cutoff.StandardVersion != "2026.08" || cutoff.ReleaseVersion != "1.2.4" {
		t.Fatalf("historical cutoff identity changed: standard=%q release=%q", cutoff.StandardVersion, cutoff.ReleaseVersion)
	}
	integrity := make(map[string]string, len(cutoff.IntegrityRecords))
	digestPattern := regexp.MustCompile(`^[a-f0-9]{64}$`)
	for _, record := range cutoff.IntegrityRecords {
		if _, duplicate := integrity[record.Path]; duplicate {
			t.Fatalf("duplicate integrity record %q", record.Path)
		}
		if !digestPattern.MatchString(record.SHA256) {
			t.Fatalf("historical integrity record %q has invalid digest %q", record.Path, record.SHA256)
		}
		integrity[record.Path] = record.SHA256
	}
	selected := make(map[string]string, len(cutoff.ToolSelections))
	referencedIntegrity := make(map[string]bool)
	for _, tool := range cutoff.ToolSelections {
		if _, duplicate := selected[tool.ID]; duplicate {
			t.Fatalf("duplicate cutoff tool %q", tool.ID)
		}
		selected[tool.ID] = tool.Version
		for _, evidence := range tool.IntegrityEvidence {
			if integrity[evidence] == "" {
				t.Errorf("tool %q references unbound integrity evidence %q", tool.ID, evidence)
			}
			referencedIntegrity[evidence] = true
		}
	}
	if fmt.Sprint(sortedKeys(referencedIntegrity)) != fmt.Sprint(sortedKeys(integrity)) {
		t.Fatalf("cutoff contains unreferenced or unbound integrity records\nreferenced: %v\nrecords: %v", sortedKeys(referencedIntegrity), sortedKeys(integrity))
	}
	var bundle templateBundle
	decodeJSON(t, readFile(t, root, "templates/bundle.json"), &bundle)
	if cutoff.MiseMinimumVersion != bundle.MiseMinimumVersion {
		t.Fatalf("retained cutoff mise minimum %q differs from bundle %q", cutoff.MiseMinimumVersion, bundle.MiseMinimumVersion)
	}
	if fmt.Sprint(sortedPairs(selected)) != fmt.Sprint(sortedPairs(bundle.Tools)) {
		t.Fatalf("cutoff tools do not match the bundle\ncutoff: %v\nbundle: %v", sortedPairs(selected), sortedPairs(bundle.Tools))
	}

	var compatibility compatibilityManifest
	decodeJSON(t, readFile(t, root, "compatibility/manifest.json"), &compatibility)
	if compatibility.Lifecycle != "stable" || fmt.Sprint(compatibility.Enforcement) != "[report-only]" {
		t.Fatalf("stable compatibility identity is inconsistent: %+v", compatibility)
	}
	wantRuntime := make(map[string]bool)
	for _, selection := range compatibility.RuntimeSelections {
		for _, version := range selection.Versions {
			wantRuntime[runtimeKey(selection.Profile, selection.Tool, version.Version, version.Disposition, version.SupportEndsAt)] = true
		}
	}
	gotRuntime := make(map[string]bool)
	for _, selection := range cutoff.RuntimeSupport {
		key := runtimeKey(selection.Profile, selection.Tool, selection.Version, selection.Disposition, selection.SupportEndsAt)
		if gotRuntime[key] {
			t.Fatalf("duplicate runtime cutoff %q", key)
		}
		gotRuntime[key] = true
	}
	if fmt.Sprint(sortedKeys(gotRuntime)) != fmt.Sprint(sortedKeys(wantRuntime)) {
		t.Fatalf("runtime cutoff does not match compatibility manifest\ncutoff: %v\ncompatibility: %v", sortedKeys(gotRuntime), sortedKeys(wantRuntime))
	}

	assertWorkflowActionPins(t, root, cutoff.WorkflowActions)
}

func TestReleaseSourceIntegrityBindsCurrentBytes(t *testing.T) {
	root, err := os.OpenRoot("..")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()

	data := readFile(t, root, "release/source-integrity-2026-08-07.json")
	validateJSON(t, readFile(t, root, "release/golden-path-source-integrity-v1.schema.json"), data)
	var sourceIntegrity sourceIntegrityManifest
	decodeJSON(t, data, &sourceIntegrity)
	if sourceIntegrity.SchemaVersion != "golden-path-source-integrity/v1" {
		t.Fatalf("source integrity schema = %q", sourceIntegrity.SchemaVersion)
	}
	checkedAt, err := time.Parse(time.RFC3339, sourceIntegrity.CheckedAt)
	if err != nil || checkedAt.Location() != time.UTC {
		t.Fatalf("source integrity checkedAt must be canonical UTC: %q", sourceIntegrity.CheckedAt)
	}
	var bundle templateBundle
	decodeJSON(t, readFile(t, root, "templates/bundle.json"), &bundle)
	if sourceIntegrity.StandardVersion != bundle.StandardVersion || sourceIntegrity.ReleaseVersion != bundle.AssetBundleVersion {
		t.Fatalf("source integrity identity does not match template bundle: integrity=%+v bundle=%+v", sourceIntegrity, bundle)
	}
	seen := make(map[string]bool, len(sourceIntegrity.IntegrityRecords))
	for _, record := range sourceIntegrity.IntegrityRecords {
		if seen[record.Path] {
			t.Fatalf("duplicate source integrity record %q", record.Path)
		}
		seen[record.Path] = true
		actual := fmt.Sprintf("%x", sha256.Sum256(readFile(t, root, record.Path)))
		if actual != record.SHA256 {
			t.Fatalf("source integrity %q = %s, want current digest %s", record.Path, record.SHA256, actual)
		}
	}
	if len(seen) != 11 {
		t.Fatalf("source integrity record count = %d, want 11", len(seen))
	}
}

func TestReleaseWorkflowPublishesTheFlatDownloadedBundle(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowDocument
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	upload := namedStep(t, workflow.Jobs["assemble"], "Upload release bundle")
	if path, ok := upload.With["path"].(string); !ok || path != "dist" {
		t.Fatalf("release bundle upload path = %#v, want flat dist root", upload.With["path"])
	}
	download := namedStep(t, workflow.Jobs["publish"], "Download release bundle")
	if path, ok := download.With["path"].(string); !ok || path != "bundle" {
		t.Fatalf("release bundle download path = %#v, want bundle", download.With["path"])
	}
	attest := namedStep(t, workflow.Jobs["publish"], "Attest release subjects")
	if subjects, ok := attest.With["subject-path"].(string); !ok || subjects != "bundle/*" {
		t.Fatalf("attestation subjects = %#v, want bundle/*", attest.With["subject-path"])
	}
	publish := namedStep(t, workflow.Jobs["publish"], "Publish immutable release")
	for _, required := range []string{
		`gh release create "$GITHUB_REF_NAME" bundle/*`,
		`--notes-file bundle/RELEASE_NOTES.md`,
	} {
		if !strings.Contains(publish.Run, required) {
			t.Errorf("publish step does not contain %q", required)
		}
	}
	if strings.Contains(publish.Run, "bundle/dist/") {
		t.Fatal("publish step assumes the removed dist directory prefix")
	}
	assemble := namedStep(t, workflow.Jobs["assemble"], "Assemble release evidence")
	if !strings.Contains(assemble.Run, "--require-clean-checkout") {
		t.Fatal("release manifest publication does not require a clean exact source checkout")
	}
}

func namedStep(t *testing.T, job workflowJob, name string) workflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("workflow step %q is missing", name)
	return workflowStep{}
}

func assertWorkflowActionPins(t *testing.T, root *os.Root, actions []workflowAction) {
	t.Helper()
	want := make(map[string]workflowAction, len(actions))
	for _, action := range actions {
		if _, duplicate := want[action.Repository]; duplicate {
			t.Fatalf("duplicate workflow action %q", action.Repository)
		}
		want[action.Repository] = action
	}
	seen := make(map[string]bool, len(actions))
	entries, err := fs.ReadDir(root.FS(), ".github/workflows")
	if err != nil {
		t.Fatal(err)
	}
	fullCommit := regexp.MustCompile(`^[a-f0-9]{40}$`)
	exactVersion := regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data := readFile(t, root, ".github/workflows/"+entry.Name())
		pins, parseErr := workflowActionPins(data)
		if parseErr != nil {
			t.Errorf("workflow %q action references are invalid: %v", entry.Name(), parseErr)
			continue
		}
		for _, pin := range pins {
			repository, commit, version := pin.Repository, pin.Commit, pin.Version
			if !fullCommit.MatchString(commit) || !exactVersion.MatchString(version) {
				t.Errorf("workflow action %q is not pinned by a full commit with an exact version comment", repository)
				continue
			}
			expected, ok := want[repository]
			if !ok {
				t.Errorf("workflow uses unrecorded external action %q", repository)
				continue
			}
			if expected.Commit != commit || expected.Version != version {
				t.Errorf("workflow action %q is %s %s, want %s %s", repository, version, commit, expected.Version, expected.Commit)
			}
			seen[repository] = true
		}
	}
	if fmt.Sprint(sortedKeys(seen)) != fmt.Sprint(sortedKeys(want)) {
		t.Fatalf("cutoff workflow action set does not match executable workflows\nseen: %v\ncutoff: %v", sortedKeys(seen), sortedKeys(want))
	}
}

func workflowActionPins(data []byte) ([]workflowAction, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow root must be a mapping")
	}
	jobs, err := yamlMappingValue(document.Content[0], "jobs")
	if err != nil {
		return nil, err
	}
	if jobs == nil {
		return nil, nil
	}
	jobs, err = dereferenceYAMLNode(jobs)
	if err != nil {
		return nil, err
	}
	if jobs.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow jobs must be a mapping")
	}
	var pins []workflowAction
	for index := 1; index < len(jobs.Content); index += 2 {
		job, err := dereferenceYAMLNode(jobs.Content[index])
		if err != nil {
			return nil, err
		}
		if job.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("workflow job must be a mapping")
		}
		uses, err := yamlMappingValue(job, "uses")
		if err != nil {
			return nil, err
		}
		if uses != nil {
			pin, local, err := workflowActionPin(uses)
			if err != nil {
				return nil, err
			}
			if !local {
				pins = append(pins, pin)
			}
		}
		steps, err := yamlMappingValue(job, "steps")
		if err != nil {
			return nil, err
		}
		if steps == nil {
			continue
		}
		steps, err = dereferenceYAMLNode(steps)
		if err != nil {
			return nil, err
		}
		if steps.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("workflow steps must be a sequence")
		}
		for _, stepValue := range steps.Content {
			step, err := dereferenceYAMLNode(stepValue)
			if err != nil {
				return nil, err
			}
			if step.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("workflow step must be a mapping")
			}
			uses, err := yamlMappingValue(step, "uses")
			if err != nil {
				return nil, err
			}
			if uses == nil {
				continue
			}
			pin, local, err := workflowActionPin(uses)
			if err != nil {
				return nil, err
			}
			if !local {
				pins = append(pins, pin)
			}
		}
	}
	return pins, nil
}

func yamlMappingValue(node *yaml.Node, key string) (*yaml.Node, error) {
	mapping, err := dereferenceYAMLNode(node)
	if err != nil {
		return nil, err
	}
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("YAML value must be a mapping")
	}
	var result *yaml.Node
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		name, err := yamlString(mapping.Content[index], "YAML mapping key")
		if err != nil {
			return nil, err
		}
		if name != key {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("YAML mapping contains duplicate key %q", key)
		}
		result = mapping.Content[index+1]
	}
	return result, nil
}

func dereferenceYAMLNode(node *yaml.Node) (*yaml.Node, error) {
	for depth := 0; node != nil && node.Kind == yaml.AliasNode; depth++ {
		if depth >= 32 || node.Alias == nil {
			return nil, fmt.Errorf("YAML alias depth exceeds limit")
		}
		node = node.Alias
	}
	if node == nil {
		return nil, fmt.Errorf("YAML node is missing")
	}
	return node, nil
}

func yamlString(node *yaml.Node, context string) (string, error) {
	scalar, err := dereferenceYAMLNode(node)
	if err != nil {
		return "", err
	}
	if scalar.Kind != yaml.ScalarNode || scalar.Tag != "!!str" || strings.TrimSpace(scalar.Value) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", context)
	}
	return strings.TrimSpace(scalar.Value), nil
}

func workflowActionPin(node *yaml.Node) (workflowAction, bool, error) {
	scalar, err := dereferenceYAMLNode(node)
	if err != nil {
		return workflowAction{}, false, err
	}
	reference, err := yamlString(scalar, "workflow uses")
	if err != nil {
		return workflowAction{}, false, err
	}
	if strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "$/") {
		return workflowAction{}, true, nil
	}
	at := strings.LastIndex(reference, "@")
	if at < 0 {
		return workflowAction{}, false, fmt.Errorf("workflow action %q has no revision", reference)
	}
	parts := strings.Split(reference[:at], "/")
	if len(parts) < 2 {
		return workflowAction{}, false, fmt.Errorf("workflow action %q has no owner and repository", reference)
	}
	version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(scalar.LineComment), "#"))
	return workflowAction{Repository: parts[0] + "/" + parts[1], Commit: reference[at+1:], Version: version}, false, nil
}

func TestWorkflowActionPinsUseParsedYAMLStructure(t *testing.T) {
	pins, err := workflowActionPins([]byte(`name: test
on: push
jobs:
  call:
    uses: octo/workflows/.github/workflows/quality.yml@0123456789abcdef0123456789abcdef01234567 # v1.2.3
  local:
    uses: $/.github/workflows/local.yml
  build:
    runs-on: ubuntu-24.04
    steps:
      - &checkout
        uses: actions/checkout@89abcdef0123456789abcdef0123456789abcdef # v7.0.1
      - *checkout
      - run: |
          uses: ignored/block-scalar@v1
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []workflowAction{
		{Repository: "octo/workflows", Commit: "0123456789abcdef0123456789abcdef01234567", Version: "v1.2.3"},
		{Repository: "actions/checkout", Commit: "89abcdef0123456789abcdef0123456789abcdef", Version: "v7.0.1"},
		{Repository: "actions/checkout", Commit: "89abcdef0123456789abcdef0123456789abcdef", Version: "v7.0.1"},
	}
	if fmt.Sprint(pins) != fmt.Sprint(want) {
		t.Fatalf("workflow action pins = %+v, want %+v", pins, want)
	}
}

func validateJSON(t *testing.T, schemaData, documentData []byte) {
	t.Helper()
	var schemaDocument any
	decodeJSON(t, schemaData, &schemaDocument)
	var document any
	decodeJSON(t, documentData, &document)
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:test:schema", schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("urn:test:schema")
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root *os.Root, name string) []byte {
	t.Helper()
	data, err := root.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeJSON(t *testing.T, data []byte, value any) {
	t.Helper()
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}

func runtimeKey(profile, tool, version, disposition, supportEndsAt string) string {
	return profile + "\x00" + tool + "\x00" + version + "\x00" + disposition + "\x00" + supportEndsAt
}

func sortedPairs(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

func sortedKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
