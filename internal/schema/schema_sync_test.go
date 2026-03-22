package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAPISchemasStayInSyncWithEmbeddedSchemas(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	pkgDir := filepath.Dir(thisFile)
	for _, contract := range PublishedContracts() {
		internalPath := filepath.Join(pkgDir, filepath.FromSlash(contract.EmbeddedPath))
		apiPath := filepath.Join(pkgDir, "..", "..", filepath.FromSlash(contract.APIPath))

		internalRaw, err := os.ReadFile(internalPath)
		if err != nil {
			t.Fatalf("read internal schema %s: %v", internalPath, err)
		}
		apiRaw, err := os.ReadFile(apiPath)
		if err != nil {
			t.Fatalf("read api schema %s: %v", apiPath, err)
		}

		var internalObj any
		if err := json.Unmarshal(internalRaw, &internalObj); err != nil {
			t.Fatalf("decode internal schema json %s: %v", internalPath, err)
		}
		var apiObj any
		if err := json.Unmarshal(apiRaw, &apiObj); err != nil {
			t.Fatalf("decode api schema json %s: %v", apiPath, err)
		}

		internalNorm, err := json.Marshal(internalObj)
		if err != nil {
			t.Fatalf("normalize internal schema json %s: %v", internalPath, err)
		}
		apiNorm, err := json.Marshal(apiObj)
		if err != nil {
			t.Fatalf("normalize api schema json %s: %v", apiPath, err)
		}

		if string(internalNorm) != string(apiNorm) {
			t.Fatalf("schema mismatch: %s and %s must stay semantically identical", internalPath, apiPath)
		}
	}
}
