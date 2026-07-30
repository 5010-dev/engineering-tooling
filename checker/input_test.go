package checker

import (
	"strings"
	"testing"
)

func TestDecodeYAMLRejectsDuplicateKeysAndTags(t *testing.T) {
	tests := []string{
		"a: 1\na: 2\n",
		"a: !custom value\n",
		"---\na: 1\n---\na: 2\n",
	}
	for _, input := range tests {
		if _, err := decodeYAML([]byte(input)); err == nil {
			t.Fatalf("decodeYAML(%q) succeeded, want error", input)
		}
	}
}

func TestDecodeYAMLUsesJSONCompatibleModel(t *testing.T) {
	data, err := decodeYAML([]byte("date: 2026-07-31\nenabled: true\ncount: 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, fragment := range []string{`"date":"2026-07-31"`, `"enabled":true`, `"count":3`} {
		if !strings.Contains(got, fragment) {
			t.Errorf("decoded JSON %q does not contain %q", got, fragment)
		}
	}
}
