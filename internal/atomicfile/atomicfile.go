package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// WriteFile writes raw to path through a same-directory temporary file.
// On POSIX systems the final rename replaces the target atomically. On Windows,
// where replacing an existing file via os.Rename can fail, it falls back to
// removing the existing target before the rename; the target is still never
// overwritten in place.
func WriteFile(path string, raw []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := temp.Name()
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temp.Chmod(perm); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		if runtime.GOOS != "windows" {
			return fmt.Errorf("replace target file: %w", err)
		}
		if _, statErr := os.Stat(path); statErr != nil {
			return fmt.Errorf("replace target file: %w", err)
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("remove existing target file: %w", removeErr)
		}
		if renameErr := os.Rename(tempPath, path); renameErr != nil {
			return fmt.Errorf("replace target file after removing existing file: %w", renameErr)
		}
	}
	keepTemp = false
	return nil
}
