package schema

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema/model.schema.json
var canonicalSchema []byte

var (
	schemaOnce sync.Once
	schemaObj  *jsonschema.Schema
	schemaErr  error
)

func EmbeddedSchema() []byte {
	out := make([]byte, len(canonicalSchema))
	copy(out, canonicalSchema)
	return out
}

func EmbeddedSchemaDigest() string {
	sum := sha256.Sum256(canonicalSchema)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidateStrict(ref SchemaRef) error {
	if strings.TrimSpace(ref.Name) == "" {
		return fmt.Errorf("metadata.schema.name cannot be empty")
	}
	if strings.TrimSpace(ref.Version) == "" {
		return fmt.Errorf("metadata.schema.version cannot be empty")
	}
	if strings.TrimSpace(ref.URI) == "" {
		return fmt.Errorf("metadata.schema.uri cannot be empty")
	}
	if strings.TrimSpace(ref.Digest) == "" {
		return fmt.Errorf("metadata.schema.digest cannot be empty")
	}

	if ref.Name != ExpectedSchemaName {
		return fmt.Errorf("schema name mismatch: got %q want %q", ref.Name, ExpectedSchemaName)
	}
	if ref.Version != ExpectedSchemaVersion {
		return fmt.Errorf("schema version mismatch: got %q want %q", ref.Version, ExpectedSchemaVersion)
	}
	if ref.URI != ExpectedSchemaURI {
		return fmt.Errorf("schema uri mismatch: got %q want %q", ref.URI, ExpectedSchemaURI)
	}
	if ref.Digest != ExpectedSchemaDigest {
		return fmt.Errorf("schema digest mismatch: got %q want %q", ref.Digest, ExpectedSchemaDigest)
	}
	return nil
}

func ValidateJSON(raw []byte) error {
	doc, err := decodeJSON(raw)
	if err != nil {
		return fmt.Errorf("decode model json: %w", err)
	}

	compiled, err := compiledSchema()
	if err != nil {
		return fmt.Errorf("compile canonical schema: %w", err)
	}
	if err := compiled.Validate(doc); err != nil {
		return fmt.Errorf("jsonschema validation failed: %w", err)
	}

	ref, err := extractSchemaRef(doc)
	if err != nil {
		return fmt.Errorf("extract metadata.schema: %w", err)
	}
	if err := ValidateStrict(ref); err != nil {
		return fmt.Errorf("strict metadata.schema validation failed: %w", err)
	}
	return nil
}

func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(ExpectedSchemaURI, bytes.NewReader(canonicalSchema)); err != nil {
			schemaErr = err
			return
		}
		schemaObj, schemaErr = compiler.Compile(ExpectedSchemaURI)
	})
	return schemaObj, schemaErr
}

func decodeJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing tokens")
	}
	return doc, nil
}

func extractSchemaRef(doc any) (SchemaRef, error) {
	root, ok := doc.(map[string]any)
	if !ok {
		return SchemaRef{}, fmt.Errorf("root is not an object")
	}

	metadata, ok := root["metadata"].(map[string]any)
	if !ok {
		return SchemaRef{}, fmt.Errorf("metadata object missing")
	}

	rawSchema, ok := metadata["schema"].(map[string]any)
	if !ok {
		return SchemaRef{}, fmt.Errorf("metadata.schema object missing")
	}

	ref := SchemaRef{
		Name:    toString(rawSchema["name"]),
		Version: toString(rawSchema["version"]),
		URI:     toString(rawSchema["uri"]),
		Digest:  toString(rawSchema["digest"]),
	}
	return ref, nil
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}
