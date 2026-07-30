package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/5010-dev/engineering-tooling/checker"
)

type target struct {
	os           string
	architecture string
}

type archiveEntry struct {
	name string
	data []byte
	mode int64
}

type cycloneDXBOM struct {
	BOMFormat    string              `json:"bomFormat"`
	SpecVersion  string              `json:"specVersion"`
	Version      int                 `json:"version"`
	Metadata     cycloneDXMetadata   `json:"metadata"`
	Components   []cycloneComponent  `json:"components,omitempty"`
	Dependencies []cycloneDependency `json:"dependencies"`
}

type cycloneDXMetadata struct {
	Component cycloneComponent `json:"component"`
}

type cycloneComponent struct {
	Type       string            `json:"type"`
	BOMRef     string            `json:"bom-ref"`
	Name       string            `json:"name"`
	Version    string            `json:"version,omitempty"`
	PURL       string            `json:"purl,omitempty"`
	Hashes     []cycloneHash     `json:"hashes,omitempty"`
	Properties []cycloneProperty `json:"properties,omitempty"`
}

type cycloneHash struct {
	Algorithm string `json:"alg"`
	Content   string `json:"content"`
}

type cycloneProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type cycloneDependency struct {
	Ref       string   `json:"ref"`
	DependsOn []string `json:"dependsOn"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(parent context.Context, arguments []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("release-package", flag.ContinueOnError)
	flags.SetOutput(stderr)
	source := flags.String("source", ".", "clean release source root")
	dist := flags.String("dist", "dist", "release output directory")
	targetSelector := flags.String("target", "", "optional release target as os/architecture")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}

	sourceRoot, err := os.OpenRoot(*source)
	if err != nil {
		return fmt.Errorf("open source root: %w", err)
	}
	defer func() {
		_ = sourceRoot.Close()
	}()
	readme, err := sourceRoot.ReadFile("README.md")
	if err != nil {
		return fmt.Errorf("read README.md: %w", err)
	}
	security, err := sourceRoot.ReadFile("SECURITY.md")
	if err != nil {
		return fmt.Errorf("read SECURITY.md: %w", err)
	}

	if err := os.MkdirAll(*dist, 0o750); err != nil {
		return fmt.Errorf("create release directory: %w", err)
	}
	distRoot, err := os.OpenRoot(*dist)
	if err != nil {
		return fmt.Errorf("open release directory: %w", err)
	}
	defer func() {
		_ = distRoot.Close()
	}()

	temporary, err := os.MkdirTemp("", "golden-path-release-*")
	if err != nil {
		return fmt.Errorf("create release staging directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(temporary)
	}()
	temporaryRoot, err := os.OpenRoot(temporary)
	if err != nil {
		return fmt.Errorf("open release staging directory: %w", err)
	}
	defer func() {
		_ = temporaryRoot.Close()
	}()

	targets, err := selectedTargets(*targetSelector)
	if err != nil {
		return err
	}
	for _, releaseTarget := range targets {
		binaryName := "golden-path-" + releaseTarget.os + "-" + releaseTarget.architecture
		binaryPath := filepath.Join(temporary, binaryName)
		buildContext, cancel := context.WithTimeout(parent, 2*time.Minute)
		// #nosec G204 -- executable and arguments are fixed; only allowlisted GOOS/GOARCH values vary.
		command := exec.CommandContext(buildContext, "go",
			"build",
			"-mod=readonly",
			"-trimpath",
			"-o", binaryPath,
			"./cmd/golden-path",
		)
		command.Dir = *source
		command.Env = buildEnvironment(os.Environ(), releaseTarget)
		var buildOutput bytes.Buffer
		command.Stdout = &buildOutput
		command.Stderr = &buildOutput
		err := command.Run()
		cancel()
		if err != nil {
			return fmt.Errorf("build %s/%s: %w", releaseTarget.os, releaseTarget.architecture, err)
		}
		binary, err := temporaryRoot.ReadFile(binaryName)
		if err != nil {
			return fmt.Errorf("read %s/%s binary: %w", releaseTarget.os, releaseTarget.architecture, err)
		}

		archiveName := fmt.Sprintf(
			"golden-path_%s_%s_%s.tar.gz",
			checker.Version,
			releaseTarget.os,
			releaseTarget.architecture,
		)
		archive, err := deterministicArchive([]archiveEntry{
			{name: "golden-path/README.md", data: readme, mode: 0o644},
			{name: "golden-path/SECURITY.md", data: security, mode: 0o644},
			{name: "golden-path/golden-path", data: binary, mode: 0o755},
		})
		if err != nil {
			return fmt.Errorf("archive %s/%s: %w", releaseTarget.os, releaseTarget.architecture, err)
		}
		if err := writeRootFile(distRoot, archiveName, archive); err != nil {
			return err
		}
		sum := sha256.Sum256(archive)
		archiveDigest := hex.EncodeToString(sum[:])
		sbom, err := deterministicSBOM(binary, archiveName, archiveDigest, releaseTarget)
		if err != nil {
			return fmt.Errorf("generate SBOM for %s/%s: %w", releaseTarget.os, releaseTarget.architecture, err)
		}
		sbomName := fmt.Sprintf(
			"golden-path_%s_%s_%s.cdx.json",
			checker.Version,
			releaseTarget.os,
			releaseTarget.architecture,
		)
		if err := writeRootFile(distRoot, sbomName, sbom); err != nil {
			return err
		}
		sbomSum := sha256.Sum256(sbom)
		if _, err := fmt.Fprintf(stdout, "%s  %s\n", hex.EncodeToString(sum[:]), archiveName); err != nil {
			return fmt.Errorf("write package result: %w", err)
		}
		if _, err := fmt.Fprintf(stdout, "%s  %s\n", hex.EncodeToString(sbomSum[:]), sbomName); err != nil {
			return fmt.Errorf("write package result: %w", err)
		}
	}
	return nil
}

func selectedTargets(selector string) ([]target, error) {
	compatibility, err := checker.BundledCompatibility()
	if err != nil {
		return nil, fmt.Errorf("verify bundled compatibility manifest: %w", err)
	}
	available := make([]target, 0, len(compatibility.SupportedTargets))
	for _, supported := range compatibility.SupportedTargets {
		available = append(available, target{
			os:           supported.OS,
			architecture: supported.Architecture,
		})
	}
	if selector == "" {
		return available, nil
	}
	selectedOS, selectedArchitecture, found := strings.Cut(selector, "/")
	if !found {
		return nil, fmt.Errorf("target must use os/architecture")
	}
	for _, candidate := range available {
		if candidate.os == selectedOS && candidate.architecture == selectedArchitecture {
			return []target{candidate}, nil
		}
	}
	return nil, fmt.Errorf("unsupported release target %q", selector)
}

func deterministicSBOM(
	binary []byte,
	archiveName,
	archiveDigest string,
	releaseTarget target,
) ([]byte, error) {
	build, err := buildinfo.Read(bytes.NewReader(binary))
	if err != nil {
		return nil, fmt.Errorf("read Go build information: %w", err)
	}
	rootRef := "urn:sha256:" + archiveDigest
	components := make([]cycloneComponent, 0, len(build.Deps))
	dependencyRefs := make([]string, 0, len(build.Deps))
	seen := map[string]bool{}
	for _, dependency := range build.Deps {
		module := dependency
		if dependency.Replace != nil {
			module = dependency.Replace
		}
		identity := module.Path + "@" + module.Version
		if identity == "@" || seen[identity] {
			continue
		}
		seen[identity] = true
		identitySum := sha256.Sum256([]byte(identity))
		reference := "urn:sha256:" + hex.EncodeToString(identitySum[:])
		component := cycloneComponent{
			Type:    "library",
			BOMRef:  reference,
			Name:    module.Path,
			Version: module.Version,
			PURL:    goModulePURL(module.Path, module.Version),
		}
		if validGoModuleHash(module.Sum) {
			component.Properties = []cycloneProperty{{
				Name:  "5010-dev:go:module-h1",
				Value: module.Sum,
			}}
		}
		components = append(components, component)
		dependencyRefs = append(dependencyRefs, reference)
	}
	sort.Slice(components, func(left, right int) bool {
		return components[left].BOMRef < components[right].BOMRef
	})
	sort.Strings(dependencyRefs)

	document := cycloneDXBOM{
		BOMFormat:   "CycloneDX",
		SpecVersion: "1.6",
		Version:     1,
		Metadata: cycloneDXMetadata{Component: cycloneComponent{
			Type:    "application",
			BOMRef:  rootRef,
			Name:    archiveName,
			Version: checker.Version,
			Hashes:  []cycloneHash{{Algorithm: "SHA-256", Content: archiveDigest}},
			Properties: []cycloneProperty{
				{Name: "5010-dev:target:os", Value: releaseTarget.os},
				{Name: "5010-dev:target:architecture", Value: releaseTarget.architecture},
			},
		}},
		Components:   components,
		Dependencies: []cycloneDependency{{Ref: rootRef, DependsOn: dependencyRefs}},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode CycloneDX document: %w", err)
	}
	return append(data, '\n'), nil
}

func validGoModuleHash(value string) bool {
	if !strings.HasPrefix(value, "h1:") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "h1:"))
	return err == nil && len(raw) == sha256.Size
}

func goModulePURL(modulePath, version string) string {
	segments := strings.Split(modulePath, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return "pkg:golang/" + strings.Join(segments, "/") + "@" + url.PathEscape(version)
}

func buildEnvironment(environment []string, releaseTarget target) []string {
	replacements := map[string]string{
		"CGO_ENABLED": "0",
		"GOOS":        releaseTarget.os,
		"GOARCH":      releaseTarget.architecture,
		"GOTOOLCHAIN": "local",
	}
	result := make([]string, 0, len(environment)+len(replacements))
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := replacements[key]; replaced {
				continue
			}
		}
		result = append(result, entry)
	}
	for _, key := range []string{"CGO_ENABLED", "GOARCH", "GOOS", "GOTOOLCHAIN"} {
		result = append(result, key+"="+replacements[key])
	}
	return result
}

func deterministicArchive(entries []archiveEntry) ([]byte, error) {
	var result bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&result, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}
	gzipWriter.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{
			Name:       entry.name,
			Mode:       entry.mode,
			Size:       int64(len(entry.data)),
			ModTime:    time.Unix(0, 0).UTC(),
			AccessTime: time.Time{},
			ChangeTime: time.Time{},
			Uid:        0,
			Gid:        0,
			Format:     tar.FormatUSTAR,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			_ = gzipWriter.Close()
			return nil, fmt.Errorf("write tar header: %w", err)
		}
		if _, err := tarWriter.Write(entry.data); err != nil {
			_ = gzipWriter.Close()
			return nil, fmt.Errorf("write tar entry: %w", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		_ = gzipWriter.Close()
		return nil, fmt.Errorf("close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}
	return result.Bytes(), nil
}

func writeRootFile(root *os.Root, name string, data []byte) error {
	temporary := "." + name + ".tmp"
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create release asset %q: %w", name, err)
	}
	remove := true
	defer func() {
		if remove {
			_ = root.Remove(temporary)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write release asset %q: %w", name, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync release asset %q: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close release asset %q: %w", name, err)
	}
	if err := root.Rename(temporary, name); err != nil {
		return fmt.Errorf("publish release asset %q: %w", name, err)
	}
	remove = false
	return nil
}
