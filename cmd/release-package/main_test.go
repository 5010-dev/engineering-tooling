package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"
)

func TestDeterministicArchive(t *testing.T) {
	entries := []archiveEntry{
		{name: "golden-path/README.md", data: []byte("readme\n"), mode: 0o644},
		{name: "golden-path/SECURITY.md", data: []byte("security\n"), mode: 0o644},
		{name: "golden-path/golden-path", data: []byte("binary"), mode: 0o755},
	}
	first, err := deterministicArchive(entries)
	if err != nil {
		t.Fatal(err)
	}
	second, err := deterministicArchive(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("identical archive inputs produced different bytes")
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			t.Error(err)
		}
	}()
	tarReader := tar.NewReader(gzipReader)
	for index, expected := range entries {
		header, err := tarReader.Next()
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != expected.name || header.Mode != expected.mode || !header.ModTime.Equal(time.Unix(0, 0)) {
			t.Fatalf("entry %d metadata mismatch: %+v", index, header)
		}
		data, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, expected.data) {
			t.Fatalf("entry %d data mismatch", index)
		}
	}
	if _, err := tarReader.Next(); err != io.EOF {
		t.Fatalf("trailing tar entry or error: %v", err)
	}
}

func TestDeterministicSBOMBindsArchiveDigest(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- os.Executable returns the current test binary path.
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	archive := []byte("release-archive")
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	first, err := deterministicSBOM(
		binary,
		"golden-path_0.1.0_linux_amd64.tar.gz",
		digest,
		target{os: "linux", architecture: "amd64"},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := deterministicSBOM(
		binary,
		"golden-path_0.1.0_linux_amd64.tar.gz",
		digest,
		target{os: "linux", architecture: "amd64"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("identical SBOM inputs produced different bytes")
	}
	var document cycloneDXBOM
	if err := json.Unmarshal(first, &document); err != nil {
		t.Fatal(err)
	}
	if document.BOMFormat != "CycloneDX" ||
		document.SpecVersion != "1.6" ||
		len(document.Metadata.Component.Hashes) != 1 ||
		document.Metadata.Component.Hashes[0].Content != digest {
		t.Fatalf("SBOM does not bind archive digest: %+v", document.Metadata.Component)
	}
}

func TestBuildEnvironmentReplacesTargetSelectors(t *testing.T) {
	result := buildEnvironment(
		[]string{"PATH=/bin", "GOOS=windows", "GOARCH=386", "CGO_ENABLED=1"},
		target{os: "linux", architecture: "arm64"},
	)
	joined := bytes.Join(func() [][]byte {
		values := make([][]byte, len(result))
		for index := range result {
			values[index] = []byte(result[index])
		}
		return values
	}(), []byte{0})
	for _, expected := range [][]byte{
		[]byte("GOOS=linux"),
		[]byte("GOARCH=arm64"),
		[]byte("CGO_ENABLED=0"),
		[]byte("GOTOOLCHAIN=local"),
	} {
		if !bytes.Contains(joined, expected) {
			t.Errorf("environment does not contain %q", expected)
		}
	}
	for _, forbidden := range [][]byte{[]byte("GOOS=windows"), []byte("GOARCH=386"), []byte("CGO_ENABLED=1")} {
		if bytes.Contains(joined, forbidden) {
			t.Errorf("environment contains stale selector %q", forbidden)
		}
	}
}
