package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

var releaseTargets = []target{
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
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

	for _, releaseTarget := range releaseTargets {
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
		if _, err := fmt.Fprintf(stdout, "%s  %s\n", hex.EncodeToString(sum[:]), archiveName); err != nil {
			return fmt.Errorf("write package result: %w", err)
		}
	}
	return nil
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
