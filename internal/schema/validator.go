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

//go:embed schema/snapshot.schema.json
var canonicalSnapshotSchema []byte

var (
	modelSchemaOnce    sync.Once
	modelSchemaObj     *jsonschema.Schema
	modelSchemaErr     error
	snapshotSchemaOnce sync.Once
	snapshotSchemaObj  *jsonschema.Schema
	snapshotSchemaErr  error
)

func EmbeddedSchema() []byte {
	out := make([]byte, len(canonicalSchema))
	copy(out, canonicalSchema)
	return out
}

func EmbeddedSnapshotSchema() []byte {
	out := make([]byte, len(canonicalSnapshotSchema))
	copy(out, canonicalSnapshotSchema)
	return out
}

func EmbeddedSchemaDigest() string {
	sum := sha256.Sum256(canonicalSchema)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func EmbeddedSnapshotSchemaDigest() string {
	sum := sha256.Sum256(canonicalSnapshotSchema)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidateStrict(ref SchemaRef) error {
	return validateStrictAgainst(ref, ExpectedRef())
}

func ValidateSnapshotStrict(ref SchemaRef) error {
	return validateStrictAgainst(ref, ExpectedSnapshotRef())
}

func ValidateJSON(raw []byte) error {
	return validateDocument(raw, ExpectedRef(), compiledModelSchema)
}

func ValidateSnapshotJSON(raw []byte) error {
	doc, err := decodeJSON(raw)
	if err != nil {
		return fmt.Errorf("decode snapshot json: %w", err)
	}
	compiled, err := compiledSnapshotSchema()
	if err != nil {
		return fmt.Errorf("compile snapshot schema: %w", err)
	}
	if err := compiled.Validate(doc); err != nil {
		return fmt.Errorf("jsonschema validation failed: %w", err)
	}
	ref, err := extractSchemaRef(doc)
	if err != nil {
		return fmt.Errorf("extract metadata.schema: %w", err)
	}
	if err := ValidateSnapshotStrict(ref); err != nil {
		return fmt.Errorf("strict metadata.schema validation failed: %w", err)
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return fmt.Errorf("snapshot root is not an object")
	}
	rawModel, err := json.Marshal(root["model"])
	if err != nil {
		return fmt.Errorf("encode nested model: %w", err)
	}
	if err := ValidateJSON(rawModel); err != nil {
		return fmt.Errorf("nested model validation failed: %w", err)
	}
	return nil
}

func ValidateArtifactJSON(raw []byte) error {
	doc, err := decodeJSON(raw)
	if err != nil {
		return fmt.Errorf("decode artifact json: %w", err)
	}
	ref, err := extractSchemaRef(doc)
	if err != nil {
		return fmt.Errorf("extract metadata.schema: %w", err)
	}
	switch ref.Name {
	case ExpectedSchemaName:
		return ValidateJSON(raw)
	case ExpectedSnapshotSchemaName:
		return ValidateSnapshotJSON(raw)
	default:
		return fmt.Errorf("unsupported artifact schema name: %s", ref.Name)
	}
}

func validateDocument(raw []byte, expected SchemaRef, compiler func() (*jsonschema.Schema, error)) error {
	doc, err := decodeJSON(raw)
	if err != nil {
		return fmt.Errorf("decode model json: %w", err)
	}
	compiled, err := compiler()
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
	if err := validateStrictAgainst(ref, expected); err != nil {
		return fmt.Errorf("strict metadata.schema validation failed: %w", err)
	}
	return nil
}

func validateStrictAgainst(ref, expected SchemaRef) error {
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
	if ref.Name != expected.Name {
		return fmt.Errorf("schema name mismatch: got %q want %q", ref.Name, expected.Name)
	}
	if ref.Version != expected.Version {
		return fmt.Errorf("schema version mismatch: got %q want %q", ref.Version, expected.Version)
	}
	if ref.URI != expected.URI {
		return fmt.Errorf("schema uri mismatch: got %q want %q", ref.URI, expected.URI)
	}
	if ref.Digest != expected.Digest {
		return fmt.Errorf("schema digest mismatch: got %q want %q", ref.Digest, expected.Digest)
	}
	return nil
}

func compiledModelSchema() (*jsonschema.Schema, error) {
	modelSchemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(ExpectedSchemaURI, bytes.NewReader(canonicalSchema)); err != nil {
			modelSchemaErr = err
			return
		}
		modelSchemaObj, modelSchemaErr = compiler.Compile(ExpectedSchemaURI)
	})
	return modelSchemaObj, modelSchemaErr
}

func compiledSnapshotSchema() (*jsonschema.Schema, error) {
	snapshotSchemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(ExpectedSnapshotSchemaURI, bytes.NewReader(canonicalSnapshotSchema)); err != nil {
			snapshotSchemaErr = err
			return
		}
		snapshotSchemaObj, snapshotSchemaErr = compiler.Compile(ExpectedSnapshotSchemaURI)
	})
	return snapshotSchemaObj, snapshotSchemaErr
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

func ExtractSchemaRef(raw []byte) (SchemaRef, error) {
	doc, err := decodeJSON(raw)
	if err != nil {
		return SchemaRef{}, err
	}
	return extractSchemaRef(doc)
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
