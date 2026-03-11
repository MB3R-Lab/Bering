package release

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	ProductName                    = "bering"
	ContractsManifestSchemaName    = "io.mb3r.bering.contracts-manifest"
	ContractsManifestSchemaVersion = "1.0.0"
	ContractsManifestSchemaURI     = "urn:io.mb3r.bering:contracts-manifest:1.0.0"
	ReleaseManifestSchemaName      = "io.mb3r.bering.release-manifest"
	ReleaseManifestSchemaVersion   = "1.0.0"
	ReleaseManifestSchemaURI       = "urn:io.mb3r.bering:release-manifest:1.0.0"
	ReleaseChecksumsName           = "release-checksums.txt"
	ReleaseManifestName            = "release-manifest.json"
	ReleaseNotesName               = "release-notes.md"
	ContractsManifestName          = "contracts-manifest.json"
)

func ContractsManifestSchema() SchemaDescriptor {
	return SchemaDescriptor{
		Name:    ContractsManifestSchemaName,
		Version: ContractsManifestSchemaVersion,
		URI:     ContractsManifestSchemaURI,
	}
}

func ReleaseManifestSchema() SchemaDescriptor {
	return SchemaDescriptor{
		Name:    ReleaseManifestSchemaName,
		Version: ReleaseManifestSchemaVersion,
		URI:     ReleaseManifestSchemaURI,
	}
}

func normalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func digestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DigestFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return digestBytes(raw), nil
}

func writeJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o644)
}

func relativeTo(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return normalizePath(path)
	}
	return normalizePath(rel)
}

func parseBuildDate(value string) (time.Time, error) {
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("build date must be RFC3339: %w", err)
	}
	return ts.UTC(), nil
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writeTarGz(path string, timestamp time.Time, files map[string]string) error {
	keys := make([]string, 0, len(files))
	for name := range files {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	for _, name := range keys {
		src := files[name]
		info, err := os.Stat(src)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("pack source %s is not a regular file", src)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = normalizePath(name)
		header.ModTime = timestamp
		header.AccessTime = timestamp
		header.ChangeTime = timestamp
		header.Mode = int64(fs.FileMode(0o644))
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(src)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, file); err != nil {
			file.Close()
			return err
		}
		file.Close()
	}

	return nil
}
