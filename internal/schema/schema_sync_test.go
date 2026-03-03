package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAPISchemaStaysInSyncWithEmbeddedSchema(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	pkgDir := filepath.Dir(thisFile)
	internalPath := filepath.Join(pkgDir, "schema", "model.schema.json")
	apiPath := filepath.Join(pkgDir, "..", "..", "api", "schema", "model.schema.json")

	internalRaw, err := os.ReadFile(internalPath)
	if err != nil {
		t.Fatalf("read internal schema: %v", err)
	}
	apiRaw, err := os.ReadFile(apiPath)
	if err != nil {
		t.Fatalf("read api schema: %v", err)
	}

	var internalObj any
	if err := json.Unmarshal(internalRaw, &internalObj); err != nil {
		t.Fatalf("decode internal schema json: %v", err)
	}
	var apiObj any
	if err := json.Unmarshal(apiRaw, &apiObj); err != nil {
		t.Fatalf("decode api schema json: %v", err)
	}

	internalNorm, err := json.Marshal(internalObj)
	if err != nil {
		t.Fatalf("normalize internal schema json: %v", err)
	}
	apiNorm, err := json.Marshal(apiObj)
	if err != nil {
		t.Fatalf("normalize api schema json: %v", err)
	}

	if string(internalNorm) != string(apiNorm) {
		t.Fatalf("schema mismatch: %s and %s must stay semantically identical", internalPath, apiPath)
	}
}
