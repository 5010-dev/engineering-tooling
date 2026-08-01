package release_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
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

	cutoffData := readFile(t, root, "release/tooling-cutoff-2026-08-01.json")
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

	var bundle templateBundle
	decodeJSON(t, readFile(t, root, "templates/bundle.json"), &bundle)
	if cutoff.StandardVersion != bundle.StandardVersion || cutoff.ReleaseVersion != bundle.AssetBundleVersion || cutoff.MiseMinimumVersion != bundle.MiseMinimumVersion {
		t.Fatalf("cutoff identity does not match template bundle: cutoff=%+v bundle=%+v", cutoff, bundle)
	}
	integrity := make(map[string]string, len(cutoff.IntegrityRecords))
	for _, record := range cutoff.IntegrityRecords {
		if _, duplicate := integrity[record.Path]; duplicate {
			t.Fatalf("duplicate integrity record %q", record.Path)
		}
		data := readFile(t, root, record.Path)
		actual := fmt.Sprintf("%x", sha256.Sum256(data))
		if actual != record.SHA256 {
			t.Fatalf("integrity record %q = %s, want %s", record.Path, record.SHA256, actual)
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
	if fmt.Sprint(sortedPairs(selected)) != fmt.Sprint(sortedPairs(bundle.Tools)) {
		t.Fatalf("cutoff tools do not match the bundle\ncutoff: %v\nbundle: %v", sortedPairs(selected), sortedPairs(bundle.Tools))
	}

	var compatibility compatibilityManifest
	decodeJSON(t, readFile(t, root, "compatibility/manifest.json"), &compatibility)
	if compatibility.CheckerVersion != cutoff.ReleaseVersion || compatibility.Lifecycle != "stable" || fmt.Sprint(compatibility.Enforcement) != "[report-only]" {
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
	pattern := regexp.MustCompile(`(?m)^\s*uses:\s+["']?([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)(?:/[A-Za-z0-9_./-]+)?@([^"'\s#]+)["']?\s*(?:#\s*(v[0-9]+\.[0-9]+\.[0-9]+))?\s*$`)
	fullCommit := regexp.MustCompile(`^[a-f0-9]{40}$`)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data := readFile(t, root, ".github/workflows/"+entry.Name())
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			repository, commit, version := match[1], match[2], match[3]
			if !fullCommit.MatchString(commit) || version == "" {
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
